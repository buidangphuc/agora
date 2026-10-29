package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

// failingRecorder wraps the in-memory repo and fails RecordLogin.
type failingRecorder struct {
	*storetest.InMemorySessionRepository
}

func (failingRecorder) RecordLogin(context.Context, store.LoginEvent) (store.LoginEvent, error) {
	return store.LoginEvent{}, errors.New("db down")
}

func newLoginSvc(t *testing.T, sess store.SessionRepository) (*identity.AuthService, string) {
	t.Helper()
	svc := newSvc(t, storetest.NewInMemoryUserRepository(), sess)
	res, err := svc.Register(context.Background(), "alice", "password123", "buyer")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return svc, res.UserID
}

func TestLoginRecordsEvents(t *testing.T) {
	sess := storetest.NewInMemorySessionRepository()
	svc, uid := newLoginSvc(t, sess)
	ctx := identity.WithClient(context.Background(), identity.ClientInfo{IP: "9.9.9.9", UserAgent: "ua"})

	history := func() []store.LoginEvent {
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
	if _, err := svc.Login(ctx, "alice", "wrong-password"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	if h := history(); len(h) != 2 || h[0].Success {
		t.Fatalf("failure event not recorded newest-first: %+v", h)
	}
	// Unknown username: identical error, nothing recorded.
	if _, err := svc.Login(ctx, "ghost", "x"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	if n := len(history()); n != 2 {
		t.Fatalf("unknown user must record nothing, got %d events", n)
	}
}

func TestLoginSucceedsWhenRecordingFails(t *testing.T) {
	svc, _ := newLoginSvc(t, failingRecorder{storetest.NewInMemorySessionRepository()})
	if _, err := svc.Login(context.Background(), "alice", "password123"); err != nil {
		t.Fatalf("recording failure must not fail login: %v", err)
	}
	if _, err := svc.Login(context.Background(), "alice", "bad"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}
