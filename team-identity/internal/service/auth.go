// Package service holds team-identity's business logic: registration, login,
// and token issuance. It is transport-agnostic; the handler maps its results to
// the proto AuthService.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/buidangphuc/team-identity/internal/authz"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/token"
)

var (
	// ErrInvalidInput is a bad register/login/password request.
	ErrInvalidInput = errors.New("invalid input")
	// ErrInvalidCredentials is a wrong username/password on login or password change.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidToken is an invalid reset token.
	ErrInvalidToken = errors.New("invalid reset token")
	// ErrTokenExpired is an expired reset token.
	ErrTokenExpired = errors.New("reset token expired")
	// ErrTokenAlreadyUsed is an already used reset token.
	ErrTokenAlreadyUsed = errors.New("reset token already used")
)

// AuthResult is the domain outcome of register/login: a signed token + the
// resolved identity the handler turns into a Principal.
type AuthResult struct {
	Token    string
	UserID   string
	Username string
	Type     string // always "user" here
	Scopes   []string
}

type AuthService struct {
	repo   repository.UserRepository
	signer *token.Signer
	ttl    time.Duration
}

func NewAuthService(repo repository.UserRepository, signer *token.Signer, ttl time.Duration) *AuthService {
	return &AuthService{repo: repo, signer: signer, ttl: ttl}
}

// Register creates a user (role buyer/seller; admin is seeded only) and issues a
// token. ErrConflict from the repo bubbles up (username taken).
func (s *AuthService) Register(ctx context.Context, username, password, role string) (AuthResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 4 {
		return AuthResult{}, ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, err
	}
	u := repository.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(hash),
		Roles:        []string{authz.NormalizeRole(role)},
	}
	created, err := s.repo.Create(ctx, u)
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(created)
}

// Login verifies credentials and issues a token.
func (s *AuthService) Login(ctx context.Context, username, password string) (AuthResult, error) {
	u, err := s.repo.GetByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	return s.issue(u)
}

func (s *AuthService) issue(u repository.User) (AuthResult, error) {
	scopes := authz.ScopesForRoles(u.Roles)
	signed, err := s.signer.Sign(u.ID, u.Username, "user", scopes, s.ttl)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Token: signed, UserID: u.ID, Username: u.Username, Type: "user", Scopes: scopes}, nil
}

// ChangePassword verifies old password and updates user's password with new hash.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || len(newPassword) < 4 || oldPassword == "" {
		return ErrInvalidInput
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(ctx, userID, string(newHash))
}

// RequestPasswordReset creates a temporary reset token for user identified by username.
func (s *AuthService) RequestPasswordReset(ctx context.Context, username string) (string, time.Time, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", time.Time{}, ErrInvalidInput
	}
	u, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		return "", time.Time{}, err
	}

	rawToken := uuid.NewString()
	tokenHash := hashToken(rawToken)
	expiresAt := time.Now().Add(15 * time.Minute)

	err = s.repo.CreateResetToken(ctx, repository.PasswordResetToken{
		TokenHash: tokenHash,
		UserID:    u.ID,
		ExpiresAt: expiresAt,
		Used:      false,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return rawToken, expiresAt, nil
}

// ResetPassword verifies the reset token and updates the user's password.
func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" || len(newPassword) < 4 {
		return ErrInvalidInput
	}

	tokenHash := hashToken(rawToken)
	t, err := s.repo.GetResetToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrTokenNotFound) {
			return ErrInvalidToken
		}
		return err
	}

	if t.Used {
		return ErrTokenAlreadyUsed
	}
	if time.Now().After(t.ExpiresAt) {
		return ErrTokenExpired
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.repo.UpdatePassword(ctx, t.UserID, string(newHash)); err != nil {
		return err
	}

	return s.repo.MarkResetTokenUsed(ctx, tokenHash)
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// EnsureAdmin seeds a default admin account if none exists (dev convenience).
func (s *AuthService) EnsureAdmin(ctx context.Context, username, password string) error {
	if _, err := s.repo.GetByUsername(ctx, username); err == nil {
		return nil // already exists
	} else if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.repo.Create(ctx, repository.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(hash),
		Roles:        []string{authz.RoleAdmin},
	})
	if errors.Is(err, repository.ErrConflict) {
		return nil
	}
	return err
}
