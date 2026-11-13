package presenter

import (
	"time"

	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// User is the HTTP representation of the guest profile.
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
	CreatedAt   string `json:"created_at"`
}

func UserToHTTP(in models.User) User {
	return User{
		ID:          in.ID,
		DisplayName: in.DisplayName,
		Timezone:    in.Timezone,
		CreatedAt:   in.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// CreatedUser extends the profile with the one-time CSRF token. It is only
// returned by POST /users, where a session (and therefore a CSRF secret) is
// actually issued.
type CreatedUser struct {
	User
	CSRFToken string `json:"csrf_token"`
}

func CreatedUserToHTTP(in usecase.CreateUserResult) CreatedUser {
	return CreatedUser{
		User:      UserToHTTP(in.User),
		CSRFToken: in.CSRFToken,
	}
}
