// Package repository is team-identity's persistence boundary: users +
// credentials. Passwords are stored as bcrypt hashes; the port is swappable
// (Postgres in prod, in-memory for tests).
package repository

import (
	"context"
	"errors"
	"sync"
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

// InMemoryUserRepository is a fake store for tests.
type InMemoryUserRepository struct {
	mu     sync.RWMutex
	byName map[string]User
	byID   map[string]User
	tokens map[string]PasswordResetToken
}

func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		byName: make(map[string]User),
		byID:   make(map[string]User),
		tokens: make(map[string]PasswordResetToken),
	}
}

func (r *InMemoryUserRepository) Create(_ context.Context, u User) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[u.Username]; ok {
		return User{}, ErrConflict
	}
	r.byName[u.Username] = u
	r.byID[u.ID] = u
	return u, nil
}

func (r *InMemoryUserRepository) GetByUsername(_ context.Context, username string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byName[username]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (r *InMemoryUserRepository) GetByID(_ context.Context, id string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (r *InMemoryUserRepository) UpdatePassword(_ context.Context, userID, newPasswordHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return ErrNotFound
	}
	u.PasswordHash = newPasswordHash
	r.byID[userID] = u
	r.byName[u.Username] = u
	return nil
}

func (r *InMemoryUserRepository) CreateResetToken(_ context.Context, token PasswordResetToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token.TokenHash] = token
	return nil
}

func (r *InMemoryUserRepository) GetResetToken(_ context.Context, tokenHash string) (PasswordResetToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	token, ok := r.tokens[tokenHash]
	if !ok {
		return PasswordResetToken{}, ErrTokenNotFound
	}
	return token, nil
}

func (r *InMemoryUserRepository) MarkResetTokenUsed(_ context.Context, tokenHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenHash]
	if !ok {
		return ErrTokenNotFound
	}
	token.Used = true
	r.tokens[tokenHash] = token
	return nil
}

var _ UserRepository = (*InMemoryUserRepository)(nil)
