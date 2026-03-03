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

// dummyHash is a precomputed bcrypt hash used to equalize login timing
// when the requested email does not exist, preventing account enumeration
// via response-time analysis (unknown email ~1ms vs wrong password ~150ms).
var dummyHash []byte

func init() {
	var err error
	dummyHash, err = bcrypt.GenerateFromPassword([]byte("dummy-timing-equalization-v1"), 12)
	if err != nil {
		panic("failed to generate dummy bcrypt hash: " + err.Error())
	}
}

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
	// bcrypt silently truncates passwords at 72 bytes (Okta-class vulnerability).
	// Reject passwords that would be silently truncated so two different passwords
	// cannot hash to the same bcrypt digest.
	if len(password) > 72 {
		return errors.New("password must not exceed 72 characters")
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
	if len(req.Email) > 254 {
		return nil, errors.New("email too long")
	}
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	// Basic email format validation — normalize to lowercase to prevent duplicate accounts
	// from the same address with mixed case (e.g. User@domain.com vs user@domain.com).
	email := strings.ToLower(strings.TrimSpace(req.Email))
	atIdx := strings.Index(email, "@")
	if atIdx < 1 || atIdx >= len(email)-1 || !strings.Contains(email[atIdx+1:], ".") {
		return nil, errors.New("invalid email format")
	}
	req.Email = email

	var existing models.User
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return nil, ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
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
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	var user models.User
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("email = ?", req.Email).First(&user).Error; err != nil {
		// Always run bcrypt even when user not found to equalize response time.
		// Without this, an attacker can distinguish "no such email" (~1ms) from
		// "wrong password" (~150ms) by measuring response latency.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
		slog.Warn("login failed: user not found", "app_id", appID)
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
		if err := s.db.Model(&models.RefreshToken{}).
			Scopes(tenant.ForTenant(appID)).
			Where("user_id = ? AND revoked = false", stored.UserID).
			Update("revoked", true).Error; err != nil {
			slog.Error("failed to revoke all tokens after reuse detection", "user_id", stored.UserID, "error", err)
		}
		return nil, ErrInvalidToken
	}

	if time.Now().After(stored.ExpiresAt) {
		if err := s.db.Model(&stored).Update("revoked", true).Error; err != nil {
			slog.Error("failed to revoke expired refresh token", "token_id", stored.ID, "error", err)
		}
		return nil, ErrInvalidToken
	}

	// Atomically revoke the token only if it is still unrevoked.
	// Without the WHERE clause, two concurrent refresh requests for the same token
	// can both pass the stored.Revoked guard above and both issue new tokens (TOCTOU race).
	// PostgreSQL's row-level locking ensures only one UPDATE wins; the other gets RowsAffected=0.
	result := s.db.Model(&models.RefreshToken{}).
		Where("id = ? AND revoked = false", stored.ID).
		Update("revoked", true)
	if result.Error != nil {
		slog.Error("failed to revoke refresh token during rotation", "token_id", stored.ID, "error", result.Error)
		return nil, fmt.Errorf("failed to rotate refresh token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Another concurrent request already revoked this token — treat as reuse.
		slog.Warn("refresh token rotation race detected", "token_id", stored.ID, "app_id", appID)
		return nil, ErrInvalidToken
	}

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

	// Apple token revocation (Guideline 5.1.1) — fire-and-forget, don't block deletion.
	// Wrapped with a 30s timeout to prevent goroutine leaks if Apple APIs are slow.
	if user.AuthProvider == "apple" && authorizationCode != "" && bundleID != "" {
		go func(cfg *config.Config, bid, code string) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in apple token revocation goroutine", "recover", r)
				}
			}()
			done := make(chan struct{}, 1)
			go func() {
				RevokeAppleTokens(cfg, bid, code)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				slog.Warn("apple token revocation timed out")
			}
		}(s.cfg, bundleID, authorizationCode)
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

	claims, err := s.appleJWKS.VerifyToken(req.IdentityToken, bundleID, req.Nonce)
	if err != nil {
		slog.Error("apple token verification failed", "error", err, "app_id", appID)
		return nil, fmt.Errorf("failed to verify Apple identity token: %w", err)
	}

	appleUserID := claims.Sub

	// tokenEmail is the email from the cryptographically-verified Apple JWT.
	// It is only present on the FIRST sign-in; subsequent sign-ins return empty.
	tokenEmail := strings.ToLower(strings.TrimSpace(claims.Email))

	// accountEmail is used solely for new-account creation, never for DB lookups.
	// req.Email is client-supplied and MUST NOT be used to find existing accounts —
	// doing so allows an attacker to merge their Apple ID into a victim's account
	// by passing the victim's email when their own token email is empty.
	accountEmail := tokenEmail
	if accountEmail == "" {
		accountEmail = strings.ToLower(strings.TrimSpace(req.Email))
	}
	if accountEmail == "" {
		accountEmail = appleUserID + "@privaterelay.appleid.com"
	}

	var user models.User
	var lookupErr error
	if tokenEmail != "" {
		// Email is token-verified — safe to match by email (handles the case where
		// a user first registered via email/password, then signs in with Apple using
		// the same address on their first Apple sign-in).
		lookupErr = s.db.Scopes(tenant.ForTenant(appID)).
			Where("apple_user_id = ? OR email = ?", appleUserID, tokenEmail).First(&user).Error
	} else {
		// No email in the token — match ONLY by apple_user_id.
		// Never use req.Email here: an attacker could set it to any address to
		// hijack an existing account.
		lookupErr = s.db.Scopes(tenant.ForTenant(appID)).
			Where("apple_user_id = ?", appleUserID).First(&user).Error
	}

	if lookupErr != nil {
		user = models.User{
			ID:           uuid.New(),
			AppID:        appID,
			Email:        accountEmail,
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
				return nil, fmt.Errorf("failed to update user with Apple ID: %w", err)
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
