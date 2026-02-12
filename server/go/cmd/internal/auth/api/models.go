package authapi

import "time"

type loginRequest struct {
	Username   *string `json:"username"`
	Email      *string `json:"email"`
	Password   string  `json:"password"`
	RememberMe bool    `json:"remember_me"`
	Platform   string  `json:"platform"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	RememberMe   bool   `json:"remember_me"`
	Platform     string `json:"platform"`
}

type inviteCreateRequest struct {
	ExpiresInSeconds int64 `json:"expires_in_seconds"`
}

type inviteConsumeRequest struct {
	InviteToken string  `json:"invite_token"`
	Username    *string `json:"username"`
	Email       *string `json:"email"`
	Password    string  `json:"password"`
	RememberMe  bool    `json:"remember_me"`
	Platform    string  `json:"platform"`
}

type userResponse struct {
	ID          string    `json:"id"`
	Username    *string   `json:"username"`
	Email       *string   `json:"email"`
	DisplayName *string   `json:"display_name"`
	Bio         *string   `json:"bio"`
	CreatedAt   time.Time `json:"created_at"`
}

type sessionResponse struct {
	SessionID        string    `json:"session_id"`
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type loginResponse struct {
	User    userResponse    `json:"user"`
	Session sessionResponse `json:"session"`
}

type refreshResponse struct {
	Session sessionResponse `json:"session"`
}

type meResponse struct {
	User userResponse `json:"user"`
}

type inviteCreateResponse struct {
	InviteID    string    `json:"invite_id"`
	InviteToken string    `json:"invite_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type inviteConsumeResponse struct {
	User     userResponse    `json:"user"`
	Session  sessionResponse `json:"session"`
	InviteID string          `json:"invite_id"`
}
