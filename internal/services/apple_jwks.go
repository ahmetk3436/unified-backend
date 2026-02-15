package services

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type AppleJWKS struct {
	Keys []AppleJWK `json:"keys"`
}

type AppleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type AppleJWKSCache struct {
	keys      map[string]*rsa.PublicKey
	expiresAt time.Time
	mu        sync.RWMutex
}

type AppleJWKSClient struct {
	cache      *AppleJWKSCache
	httpClient *http.Client
	jwksURL    string
}

type AppleJWTHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type AppleJWTClaims struct {
	Iss           string      `json:"iss"`
	Sub           string      `json:"sub"`
	Aud           string      `json:"aud"`
	Iat           int64       `json:"iat"`
	Exp           int64       `json:"exp"`
	Email         string      `json:"email"`
	EmailVerified interface{} `json:"email_verified"`
}

func NewAppleJWKSClient() *AppleJWKSClient {
	return &AppleJWKSClient{
		cache: &AppleJWKSCache{
			keys: make(map[string]*rsa.PublicKey),
		},
		httpClient: &http.Client{Timeout: 10 * time.Second},
		jwksURL:    "https://appleid.apple.com/auth/keys",
	}
}

func (c *AppleJWKSClient) fetchKeys() error {
	resp, err := c.httpClient.Get(c.jwksURL)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks AppleJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()

	c.cache.keys = make(map[string]*rsa.PublicKey)
	for _, jwk := range jwks.Keys {
		pubKey, err := parseRSAPublicKey(jwk.N, jwk.E)
		if err != nil {
			continue
		}
		c.cache.keys[jwk.Kid] = pubKey
	}
	c.cache.expiresAt = time.Now().Add(24 * time.Hour)
	return nil
}

func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	var e int
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}, nil
}

func (c *AppleJWKSClient) GetPublicKey(kid string) (*rsa.PublicKey, error) {
	c.cache.mu.RLock()
	if key, ok := c.cache.keys[kid]; ok && time.Now().Before(c.cache.expiresAt) {
		c.cache.mu.RUnlock()
		return key, nil
	}
	c.cache.mu.RUnlock()

	if err := c.fetchKeys(); err != nil {
		return nil, err
	}

	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()
	if key, ok := c.cache.keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("public key with kid %s not found", kid)
}

func (c *AppleJWKSClient) VerifyToken(identityToken, bundleID string) (*AppleJWTClaims, error) {
	parts := strings.Split(identityToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode header: %w", err)
	}

	var header AppleJWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Alg)
	}

	pubKey, err := c.GetPublicKey(header.Kid)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode claims: %w", err)
	}

	var claims AppleJWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	if claims.Iss != "https://appleid.apple.com" {
		return nil, fmt.Errorf("invalid issuer: %s", claims.Iss)
	}
	if claims.Aud != bundleID {
		return nil, fmt.Errorf("invalid audience: %s (expected %s)", claims.Aud, bundleID)
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	signingInput := parts[0] + "." + parts[1]
	signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("failed to decode signature: %w", err)
	}

	hashed := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], signatureBytes); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	return &claims, nil
}
