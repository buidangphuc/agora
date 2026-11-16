package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

// Scenario: identity.account-deletion-login-to-an-erased-account-fails
func TestLoginToDeletedAccountFailsLikeUnknownUsername(t *testing.T) {
	e := newRefreshEnv(t)
	e.setDirect(t, store.StatusDeleted, time.Time{})

	res, err := e.svc.Login(context.Background(), "alice", "password123")
	if !errors.Is(err, identity.ErrInvalidCredentials) || errors.Is(err, identity.ErrAccountLocked) {
		t.Fatalf("err = %v, want ErrInvalidCredentials exactly like an unknown username", err)
	}
	if res.Token != "" || res.RefreshToken != "" {
		t.Fatalf("no tokens may be issued: %+v", res)
	}
	_, unknownErr := e.svc.Login(context.Background(), "no-such-user", "password123")
	if err == nil || unknownErr == nil || err.Error() != unknownErr.Error() {
		t.Fatalf("deleted error %v differs from unknown-user error %v", err, unknownErr)
	}
}

// Scenario: identity.account-deletion-login-to-an-erased-account-fails (Refresh)
func TestRefreshDeletedAccountFailsLikeUnknownToken(t *testing.T) {
	e := newRefreshEnv(t)
	e.setDirect(t, store.StatusDeleted, time.Time{})

	res, err := e.refresh(e.first.RefreshToken)
	if !errors.Is(err, identity.ErrInvalidRefreshToken) || errors.Is(err, identity.ErrAccountLocked) {
		t.Fatalf("err = %v, want ErrInvalidRefreshToken", err)
	}
	if res.Token != "" || res.RefreshToken != "" {
		t.Fatalf("no tokens may be issued: %+v", res)
	}
}

// Scenario: identity.account-deletion-login-to-an-erased-account-fails (handoff exchange)
func TestRedeemHandoffDeletedAccountIsRefused(t *testing.T) {
	users := storetest.NewInMemoryUserRepository()
	if _, err := users.Create(context.Background(), store.User{ID: hoUser, Username: "minter", Roles: []string{"buyer"}, Status: store.StatusActive}); err != nil {
		t.Fatal(err)
	}
	repo := &hoRepo{}
	svc := newSvcAudit(t, users, storetest.NewInMemorySessionRepository(), &hoAudit{}).
		WithClock(func() time.Time { return hoNow }).WithHandoff(repo)
	m, err := svc.Mint(userCtx(), hoSID, identityv1.HandoffApp_HANDOFF_APP_SELLER)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	setUserStatusDirect(t, users, hoUser, store.StatusDeleted)

	res, err := svc.Redeem(context.Background(), m.Code, identityv1.HandoffApp_HANDOFF_APP_SELLER)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("err = %v, want Unauthenticated", err)
	}
	if res.Token != "" {
		t.Fatalf("no token may be issued: %+v", res)
	}
}

func setUserStatusDirect(t *testing.T, users *storetest.InMemoryUserRepository, id, st string) {
	t.Helper()
	row := func() (store.OutboxRow, error) {
		return store.OutboxRow{EventID: "fx-" + id, AggregateType: "User", AggregateID: id, EventType: "fixture", Payload: []byte("x")}, nil
	}
	if _, err := users.SetStatus(context.Background(), id, st, "", time.Time{}, hoNow,
		func(store.Session) (store.OutboxRow, error) { return row() },
		func(store.StatusChange) (store.OutboxRow, error) { return row() }); err != nil {
		t.Fatal(err)
	}
}

type noObligations struct{}

func (noObligations) UserObligations(context.Context, string) (upstream.UserObligations, error) {
	return upstream.UserObligations{}, nil
}

func (noObligations) PayoutObligations(context.Context, string) (upstream.PayoutObligations, error) {
	return upstream.PayoutObligations{}, nil
}

type pendingRepo struct {
	*storetest.InMemoryUserRepository
	eraseAfter time.Time
}

func (p *pendingRepo) ScheduleDeletion(_ context.Context, _, _ string, now time.Time, w time.Duration) (store.DeletionSchedule, error) {
	p.eraseAfter = now.Add(w)
	return store.DeletionSchedule{RequestedAt: now, EraseAfter: p.eraseAfter, Created: true}, nil
}
func (p *pendingRepo) CancelDeletion(context.Context, string) error { return nil }
func (p *pendingRepo) DeletionEraseAfter(context.Context, string) (time.Time, error) {
	return p.eraseAfter, nil
}

// Scenario: identity.account-deletion-pending-deletion-does-not-block-sign-in
func TestPendingDeletionDoesNotBlockSignIn(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users := &pendingRepo{InMemoryUserRepository: storetest.NewInMemoryUserRepository()}
	if _, err := users.Create(context.Background(), store.User{ID: "u-pend", Username: "pending", PasswordHash: string(hash), Roles: []string{"buyer"}, Status: store.StatusActive}); err != nil {
		t.Fatal(err)
	}
	del := identity.NewAccountDeletionService(identity.AccountDeletionDeps{Users: users, Obligations: noObligations{}, Window: 48 * time.Hour, Now: func() time.Time { return hoNow }})
	if _, err := del.Request(context.Background(), "u-pend", "sid", "password123"); err != nil {
		t.Fatalf("request: %v", err)
	}
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	res, err := svc.Login(context.Background(), "pending", "password123")
	if err != nil || res.Token == "" {
		t.Fatalf("login with pending deletion = %v, %v; want success", res, err)
	}
	if res.UserID != "u-pend" {
		t.Fatalf("wrong user: %+v", res)
	}
}
