// Package service holds team-identity's business logic: registration, login,
// and token issuance. It is transport-agnostic; the handler maps its results to
// the proto AuthService.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
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

// ClientInfo is the caller's device/IP as forwarded by the gateway
// (x-client-user-agent / x-client-ip). Audit-only: never used for authorization.
type ClientInfo struct {
	IP        string
	UserAgent string
}

type clientCtxKey struct{}

// WithClient attaches ClientInfo to ctx for Register/Login to record on the session.
func WithClient(ctx context.Context, c ClientInfo) context.Context {
	return context.WithValue(ctx, clientCtxKey{}, c)
}

type AuthService struct {
	repo     repository.UserRepository
	sessions repository.SessionRepository // nil: no session is recorded
	signer   *token.Signer
	ttl      time.Duration
}

// WithSessions makes Register/Login record a session row and stamp its id into
// the token as `sid`.
func (s *AuthService) WithSessions(r repository.SessionRepository) *AuthService {
	s.sessions = r
	return s
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
	return s.issue(ctx, created)
}

// recordTimeout bounds each best-effort login-history write.
const recordTimeout = 2 * time.Second

// recordLogin appends a login event (with the forwarded client IP/UA). It is
// best-effort: a failure is logged server-side and never fails or alters the login.
func (s *AuthService) recordLogin(ctx context.Context, userID string, success bool) {
	if s.sessions == nil {
		return
	}
	c, _ := ctx.Value(clientCtxKey{}).(ClientInfo)
	ctx, cancel := context.WithTimeout(ctx, recordTimeout)
	defer cancel()
	if _, err := s.sessions.RecordLogin(ctx, repository.LoginEvent{
		UserID: userID, IP: clip(c.IP, 64), UserAgent: clip(c.UserAgent, 256), Success: success,
	}); err != nil {
		slog.ErrorContext(ctx, "login recording failed",
			slog.String("op", "record login"), slog.String("user_id", userID), slog.Any("error", err))
	}
}

// Login verifies credentials and issues a token. A wrong password for an existing
// user records a failure event and a success records a success event (best-effort);
// an unknown username records nothing (login_history.user_id is an FK) and returns
// the identical error.
func (s *AuthService) Login(ctx context.Context, username, password string) (AuthResult, error) {
	u, err := s.repo.GetByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		s.recordLogin(ctx, u.ID, false)
		return AuthResult{}, ErrInvalidCredentials
	}
	res, err := s.issue(ctx, u)
	if err != nil {
		return AuthResult{}, err
	}
	s.recordLogin(ctx, u.ID, true)
	return res, nil
}

func (s *AuthService) issue(ctx context.Context, u repository.User) (AuthResult, error) {
	scopes := authz.ScopesForRoles(u.Roles)
	sid := ""
	if s.sessions != nil {
		c, _ := ctx.Value(clientCtxKey{}).(ClientInfo)
		sess, err := s.sessions.CreateSession(ctx, repository.Session{UserID: u.ID, Device: clip(c.UserAgent, 256), IP: clip(c.IP, 64)})
		if err != nil {
			return AuthResult{}, err
		}
		sid = sess.ID
	}
	signed, err := s.signer.SignWithSession(u.ID, u.Username, "user", scopes, sid, s.ttl)
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

// ResetIssue is a freshly issued password-reset token plus the facts safe to log.
type ResetIssue struct {
	Token     string
	UserID    string
	ExpiresAt time.Time
}

// RequestPasswordReset creates a temporary reset token for user identified by username.
func (s *AuthService) RequestPasswordReset(ctx context.Context, username string) (string, time.Time, error) {
	iss, err := s.IssuePasswordReset(ctx, username)
	if err != nil {
		return "", time.Time{}, err
	}
	return iss.Token, iss.ExpiresAt, nil
}

// IssuePasswordReset is RequestPasswordReset that also reports the user id, so the
// transport layer can audit-log the issuance without the raw token.
func (s *AuthService) IssuePasswordReset(ctx context.Context, username string) (ResetIssue, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return ResetIssue{}, ErrInvalidInput
	}
	u, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		return ResetIssue{}, err
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
		return ResetIssue{}, err
	}
	return ResetIssue{Token: rawToken, UserID: u.ID, ExpiresAt: expiresAt}, nil
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

// TokenFingerprint is a short, non-reversible identifier for a reset token (the
// first 12 hex characters of its stored SHA-256 hash), safe to log.
func TokenFingerprint(raw string) string { return hashToken(raw)[:12] }

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// EnsureAdmin creates the named admin account if it does not exist. Callers
// supply the credentials (SEED_ADMIN_*); an existing account is left untouched.
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

func clip(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}
