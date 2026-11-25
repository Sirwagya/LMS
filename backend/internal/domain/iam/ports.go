package iam

import (
	"context"
	"encoding/json"
	"time"
)

type Role string

const (
	RoleStudent Role = "student"
	RoleTeacher Role = "teacher"
	RoleAdmin   Role = "admin"
)

type User struct {
	ID           string          `json:"id"`
	Email        string          `json:"email"`
	Name         string          `json:"name"`
	Role         Role            `json:"role"`
	PasswordHash string          `json:"-"`
	Profile      json.RawMessage `json:"profile"`
	Version      int64           `json:"version,omitempty"`
	CreatedAt    time.Time       `json:"-"`
	UpdatedAt    time.Time       `json:"-"`
}

type PublicUser struct {
	ID      string          `json:"id"`
	Email   string          `json:"email"`
	Name    string          `json:"name"`
	Role    Role            `json:"role"`
	Profile json.RawMessage `json:"profile"`
}

func (u User) Public() PublicUser {
	profile := u.Profile
	if len(profile) == 0 {
		profile = json.RawMessage(`{}`)
	}
	return PublicUser{
		ID:      u.ID,
		Email:   u.Email,
		Name:    u.Name,
		Role:    u.Role,
		Profile: profile,
	}
}

type UserReader interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	FindByRole(ctx context.Context, id string, role Role) (*User, error)
	ListAll(ctx context.Context) ([]User, error)
}

type UserWriter interface {
	Create(ctx context.Context, user *User) error
	UpdateProfile(ctx context.Context, userID, name string, profile json.RawMessage, expectedVersion *int64) (int64, error)
	UpdatePasswordHash(ctx context.Context, userID, newHash string) error
}

type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(plain, encoded string) (valid bool, needsRehash bool, err error)
}

type SessionStore interface {
	CreateSession(ctx context.Context, userID string, ttl time.Duration) (token string, err error)
	LookupSession(ctx context.Context, token string) (userID string, err error)
	RevokeAllForUser(ctx context.Context, userID string) error
}
