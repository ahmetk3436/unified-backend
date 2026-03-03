package services

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

const (
	appleTokenURL  = "https://appleid.apple.com/auth/token"
	appleRevokeURL = "https://appleid.apple.com/auth/revoke"
)

// RevokeAppleTokens exchanges the authorization code for tokens, then revokes them.
// This is required by Apple Guideline 5.1.1 when deleting user accounts.
// If credentials are not configured, this is a no-op (logs a warning).
func RevokeAppleTokens(cfg *config.Config, bundleID, authorizationCode string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("apple revocation goroutine panicked", "panic", r)
		}
	}()

	if cfg.AppleTeamID == "" || cfg.AppleKeyID == "" || cfg.ApplePrivateKey == "" {
		slog.Warn("apple token revocation skipped: credentials not configured")
		return
	}
	if authorizationCode == "" {
		slog.Warn("apple token revocation skipped: no authorization code provided")
		return
	}

	clientSecret, err := generateAppleClientSecret(cfg, bundleID)
	if err != nil {
		slog.Error("apple revocation: failed to generate client secret", "error", err)
		return
	}

	// Step 1: Exchange authorization code for refresh token
	refreshToken, err := exchangeAppleCode(bundleID, clientSecret, authorizationCode)
	if err != nil {
		slog.Error("apple revocation: failed to exchange auth code", "error", err)
		return
	}

	// Step 2: Revoke the refresh token
	if err := revokeAppleToken(bundleID, clientSecret, refreshToken); err != nil {
		slog.Error("apple revocation: failed to revoke token", "error", err)
		return
	}

	slog.Info("apple token revoked successfully", "bundle_id", bundleID)
}

func generateAppleClientSecret(cfg *config.Config, bundleID string) (string, error) {
	// Parse the .p8 private key (may be passed with literal \n or real newlines)
	keyPEM := strings.ReplaceAll(cfg.ApplePrivateKey, "\\n", "\n")
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block from APPLE_PRIVATE_KEY")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": cfg.AppleTeamID,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"aud": "https://appleid.apple.com",
		"sub": bundleID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = cfg.AppleKeyID

	return token.SignedString(privateKey)
}

func exchangeAppleCode(bundleID, clientSecret, authorizationCode string) (string, error) {
	data := url.Values{
		"client_id":     {bundleID},
		"client_secret": {clientSecret},
		"code":          {authorizationCode},
		"grant_type":    {"authorization_code"},
	}

	resp, err := http.PostForm(appleTokenURL, data)
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token exchange response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse refresh_token from JSON response
	// Simple extraction without full JSON parsing
	bodyStr := string(body)
	idx := strings.Index(bodyStr, `"refresh_token"`)
	if idx == -1 {
		return "", fmt.Errorf("no refresh_token in response: %s", bodyStr)
	}

	// Find the value after "refresh_token":"
	rest := bodyStr[idx+len(`"refresh_token"`):]
	// Skip possible whitespace and colon
	rest = strings.TrimLeft(rest, " :")
	rest = strings.TrimLeft(rest, `"`)
	endIdx := strings.Index(rest, `"`)
	if endIdx == -1 {
		return "", fmt.Errorf("malformed refresh_token in response")
	}

	return rest[:endIdx], nil
}

func revokeAppleToken(bundleID, clientSecret, refreshToken string) error {
	data := url.Values{
		"client_id":       {bundleID},
		"client_secret":   {clientSecret},
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
	}

	resp, err := http.PostForm(appleRevokeURL, data)
	if err != nil {
		return fmt.Errorf("revoke request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("revoke returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
