package dto

import "github.com/google/uuid"

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         UserResponse `json:"user"`
}

type UserResponse struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	IsAppleUser bool      `json:"is_apple_user"`
}

type DeleteAccountRequest struct {
	Password string `json:"password"`
}

type AppleSignInRequest struct {
	IdentityToken string `json:"identity_token"`
	AuthCode      string `json:"authorization_code"`
	FullName      string `json:"full_name,omitempty"`
	Email         string `json:"email,omitempty"`
}

type ErrorResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	DB        string `json:"db"`
	AppCount  int    `json:"app_count"`
}
