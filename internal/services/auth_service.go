package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired refresh token")
	ErrUserNotFound       = errors.New("user not found")
)

type AuthService struct {
	db        *gorm.DB
	cfg       *config.Config
	appleJWKS *AppleJWKSClient
}

func NewAuthService(db *gorm.DB, cfg *config.Config) *AuthService {
	return &AuthService{
		db:        db,
		cfg:       cfg,
		appleJWKS: NewAppleJWKSClient(),
	}
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters, contain at least one uppercase letter and one digit")
	}
	hasUpper := false
	hasDigit := false
	for _, ch := range password {
		if ch >= 'A' && ch <= 'Z' {
			hasUpper = true
		}
		if ch >= '0' && ch <= '9' {
			hasDigit = true
		}
	}
	if !hasUpper || !hasDigit {
		return errors.New("password must be at least 8 characters, contain at least one uppercase letter and one digit")
	}
	return nil
}

func (s *AuthService) Register(appID string, req *dto.RegisterRequest) (*dto.AuthResponse, error) {
	if len(req.Email) == 0 {
		return nil, errors.New("email is required")
	}
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	// Basic email format validation
	email := strings.TrimSpace(req.Email)
	atIdx := strings.Index(email, "@")
	if atIdx < 1 || atIdx >= len(email)-1 || !strings.Contains(email[atIdx+1:], ".") {
		return nil, errors.New("invalid email format")
	}
	req.Email = email

	var existing models.User
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return nil, ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := models.User{
		ID:           uuid.New(),
		AppID:        appID,
		Email:        req.Email,
		Password:     string(hash),
		AuthProvider: "email",
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return s.generateTokenPair(appID, &user)
}

func (s *AuthService) Login(appID string, req *dto.LoginRequest) (*dto.AuthResponse, error) {
	var user models.User
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("email = ?", req.Email).First(&user).Error; err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.generateTokenPair(appID, &user)
}

func (s *AuthService) Refresh(appID string, req *dto.RefreshRequest) (*dto.AuthResponse, error) {
	tokenHash := hashToken(req.RefreshToken)

	var stored models.RefreshToken
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("token_hash = ?", tokenHash).First(&stored).Error; err != nil {
		return nil, ErrInvalidToken
	}

	// Token reuse detection: if the token was already revoked, this is a replay attack.
	// Revoke ALL tokens for this user as a safety measure.
	if stored.Revoked {
		slog.Warn("refresh token reuse detected, revoking all tokens for user",
			"user_id", stored.UserID, "app_id", appID)
		s.db.Model(&models.RefreshToken{}).
			Scopes(tenant.ForTenant(appID)).
			Where("user_id = ? AND revoked = false", stored.UserID).
			Update("revoked", true)
		return nil, ErrInvalidToken
	}

	if time.Now().After(stored.ExpiresAt) {
		s.db.Model(&stored).Update("revoked", true)
		return nil, ErrInvalidToken
	}

	// Revoke the current token (rotation)
	s.db.Model(&stored).Update("revoked", true)

	var user models.User
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&user, "id = ?", stored.UserID).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return s.generateTokenPair(appID, &user)
}

func (s *AuthService) Logout(appID string, req *dto.LogoutRequest) error {
	tokenHash := hashToken(req.RefreshToken)
	return s.db.Model(&models.RefreshToken{}).
		Scopes(tenant.ForTenant(appID)).
		Where("token_hash = ?", tokenHash).
		Update("revoked", true).Error
}

func (s *AuthService) DeleteAccount(appID string, userID uuid.UUID, password string, authorizationCode string, bundleID string) error {
	var user models.User
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&user, "id = ?", userID).Error; err != nil {
		return ErrUserNotFound
	}

	if user.AuthProvider != "apple" {
		if password == "" {
			return errors.New("password is required")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
			return ErrInvalidCredentials
		}
	}

	// Apple token revocation (Guideline 5.1.1) — fire-and-forget, don't block deletion
	if user.AuthProvider == "apple" && authorizationCode != "" && bundleID != "" {
		go RevokeAppleTokens(s.cfg, bundleID, authorizationCode)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND app_id = ?", userID, appID).Delete(&models.RefreshToken{}).Error; err != nil {
			return fmt.Errorf("delete refresh tokens: %w", err)
		}
		if err := tx.Where("user_id = ? AND app_id = ?", userID, appID).Delete(&models.Subscription{}).Error; err != nil {
			return fmt.Errorf("delete subscriptions: %w", err)
		}
		if err := tx.Where("reporter_id = ? AND app_id = ?", userID, appID).Delete(&models.Report{}).Error; err != nil {
			return fmt.Errorf("delete reports: %w", err)
		}
		if err := tx.Where("(blocker_id = ? OR blocked_id = ?) AND app_id = ?", userID, userID, appID).Delete(&models.Block{}).Error; err != nil {
			return fmt.Errorf("delete blocks: %w", err)
		}
		return tx.Delete(&user).Error
	})
}

func (s *AuthService) AppleSignIn(appID string, bundleID string, req *dto.AppleSignInRequest) (*dto.AuthResponse, error) {
	if req.IdentityToken == "" {
		return nil, errors.New("identity token is required")
	}

	claims, err := s.appleJWKS.VerifyToken(req.IdentityToken, bundleID)
	if err != nil {
		slog.Error("apple token verification failed", "error", err, "app_id", appID)
		return nil, fmt.Errorf("failed to verify Apple identity token: %w", err)
	}

	appleUserID := claims.Sub
	email := claims.Email
	if email == "" {
		email = req.Email
	}
	if email == "" {
		email = appleUserID + "@privaterelay.appleid.com"
	}

	var user models.User
	err = s.db.Scopes(tenant.ForTenant(appID)).
		Where("apple_user_id = ? OR email = ?", appleUserID, email).First(&user).Error

	if err != nil {
		displayName := req.FullName
		if displayName == "" {
			displayName = strings.Split(email, "@")[0]
		}

		user = models.User{
			ID:           uuid.New(),
			AppID:        appID,
			Email:        email,
			Password:     "",
			AppleUserID:  &appleUserID,
			AuthProvider: "apple",
		}
		if err := s.db.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to create Apple user: %w", err)
		}
	} else {
		if user.AppleUserID == nil {
			if err := s.db.Model(&user).Updates(map[string]interface{}{
				"apple_user_id": appleUserID,
				"auth_provider": "apple",
			}).Error; err != nil {
				slog.Error("failed to update user with Apple ID", "error", err, "user_id", user.ID, "app_id", appID)
			}
			user.AppleUserID = &appleUserID
			user.AuthProvider = "apple"
		}
	}

	return s.generateTokenPair(appID, &user)
}

func (s *AuthService) generateTokenPair(appID string, user *models.User) (*dto.AuthResponse, error) {
	accessToken, err := s.generateAccessToken(appID, user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(appID, user)
	if err != nil {
		return nil, err
	}

	return &dto.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: dto.UserResponse{
			ID:          user.ID,
			Email:       user.Email,
			IsAppleUser: user.AuthProvider == "apple",
		},
	}, nil
}

func (s *AuthService) generateAccessToken(appID string, user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":           user.ID.String(),
		"email":         user.Email,
		"app_id":        appID,
		"is_apple_user": user.AuthProvider == "apple",
		"iat":           time.Now().Unix(),
		"exp":           time.Now().Add(s.cfg.JWTAccessExpiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) generateRefreshToken(appID string, user *models.User) (string, error) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	rawToken := base64.URLEncoding.EncodeToString(rawBytes)
	tokenHash := hashToken(rawToken)

	record := models.RefreshToken{
		ID:        uuid.New(),
		AppID:     appID,
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshExpiry),
	}

	if err := s.db.Create(&record).Error; err != nil {
		return "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	return rawToken, nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}
