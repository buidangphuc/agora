// Package repository is team-identity's persistence boundary: users +
// credentials. Passwords are stored as bcrypt hashes; the port is swappable
// (Postgres in prod, in-memory for tests).
package store

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when a user lookup misses.
	ErrNotFound = errors.New("user not found")
	// ErrConflict is returned when a username already exists.
	ErrConflict = errors.New("username already exists")
	// ErrTokenNotFound is returned when a reset token lookup misses.
	ErrTokenNotFound = errors.New("reset token not found")
)

// User is the persistence-layer account.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	Roles        []string

	// Enforcement state. Zero times map to NULL.
	Status          string
	StatusReason    string
	StatusUntil     time.Time
	StatusChangedAt time.Time
	CreatedAt       time.Time
}

// Account enforcement statuses.
const (
	StatusActive     = "active"
	StatusRestricted = "restricted"
	StatusSuspended  = "suspended"
	StatusBanned     = "banned"
)

// ErrAccountLocked is returned by CreateSession for a suspended or banned user.
var ErrAccountLocked = errors.New("account locked")

// StatusChange describes one committed status transition.
type StatusChange struct {
	UserID         string
	PreviousStatus string
	Status         string
	Reason         string
	Until          time.Time
	ChangedAt      time.Time
}

// StatusEnqueueFn builds the UserStatusChanged outbox row inside the write transaction.
type StatusEnqueueFn func(StatusChange) (OutboxRow, error)

// StatusResult is the outcome of SetStatus.
type StatusResult struct {
	User            User
	SessionsRevoked int
}

const (
	defaultListLimit = 20
	maxListLimit     = 100
)

func clampListLimit(n int) int {
	if n <= 0 {
		return defaultListLimit
	}
	if n > maxListLimit {
		return maxListLimit
	}
	return n
}

// PasswordResetToken represents a temporary token used for password reset.
type PasswordResetToken struct {
	TokenHash string
	UserID    string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

// UserRepository is the port the service depends on.
type UserRepository interface {
	Create(ctx context.Context, u User) (User, error)
	GetByUsername(ctx context.Context, username string) (User, error)
	GetByID(ctx context.Context, id string) (User, error)
	UpdatePassword(ctx context.Context, userID, newPasswordHash string) error
	CreateResetToken(ctx context.Context, token PasswordResetToken) error
	GetResetToken(ctx context.Context, tokenHash string) (PasswordResetToken, error)
	MarkResetTokenUsed(ctx context.Context, tokenHash string) error
}

// ProfileReader is the narrow read port behind GetPublicProfiles. Unknown ids
// are simply absent from the result.
type ProfileReader interface {
	GetByIDs(ctx context.Context, ids []string) ([]User, error)
}
