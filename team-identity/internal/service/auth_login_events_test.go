package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
)

// failingRecorder wraps the in-memory repo and fails RecordLogin.
type failingRecorder struct {
	*repository.InMemorySessionRepository
}

func (failingRecorder) RecordLogin(context.Context, repository.LoginEvent) (repository.LoginEvent, error) {
	return repository.LoginEvent{}, errors.New("db down")
}

func newLoginSvc(t *testing.T, sess repository.SessionRepository) (*service.AuthService, string) {
	t.Helper()
	svc := service.NewAuthService(repository.NewInMemoryUserRepository(), testSigner(t), time.Hour).WithSessions(sess)
	res, err := svc.Register(context.Background(), "alice", "password123", "buyer")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return svc, res.UserID
}

func TestLoginRecordsEvents(t *testing.T) {
	sess := repository.NewInMemorySessionRepository()
	svc, uid := newLoginSvc(t, sess)
	ctx := service.WithClient(context.Background(), service.ClientInfo{IP: "9.9.9.9", UserAgent: "ua"})

	history := func() []repository.LoginEvent {
		items, _, err := sess.ListLoginHistory(context.Background(), uid, 50, 0)
		if err != nil {
			t.Fatalf("history: %v", err)
		}
		return items
	}
	if n := len(history()); n != 0 {
		t.Fatalf("Register must not record a login event, got %d", n)
	}
	if _, err := svc.Login(ctx, "alice", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if h := history(); len(h) != 1 || !h[0].Success || h[0].IP != "9.9.9.9" || h[0].UserAgent != "ua" {
		t.Fatalf("success event wrong: %+v", h)
	}
	if _, err := svc.Login(ctx, "alice", "wrong-password"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	if h := history(); len(h) != 2 || h[0].Success {
		t.Fatalf("failure event not recorded newest-first: %+v", h)
	}
	// Unknown username: identical error, nothing recorded.
	if _, err := svc.Login(ctx, "ghost", "x"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	if n := len(history()); n != 2 {
		t.Fatalf("unknown user must record nothing, got %d events", n)
	}
}

func TestLoginSucceedsWhenRecordingFails(t *testing.T) {
	svc, _ := newLoginSvc(t, failingRecorder{repository.NewInMemorySessionRepository()})
	if _, err := svc.Login(context.Background(), "alice", "password123"); err != nil {
		t.Fatalf("recording failure must not fail login: %v", err)
	}
	if _, err := svc.Login(context.Background(), "alice", "bad"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}
