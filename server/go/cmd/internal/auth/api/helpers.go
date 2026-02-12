package authapi

import (
	"arc/cmd/identity"
	"arc/cmd/internal/auth/session"
)

func toUserResponse(u identity.User) userResponse {
	return userResponse{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Bio:         u.Bio,
		CreatedAt:   u.CreatedAt,
	}
}

func toSessionResponse(issued session.Issued) sessionResponse {
	return sessionResponse{
		SessionID:        issued.SessionID,
		AccessToken:      issued.AccessToken,
		AccessExpiresAt:  issued.AccessExp,
		RefreshToken:     issued.RefreshToken,
		RefreshExpiresAt: issued.RefreshExp,
	}
}
