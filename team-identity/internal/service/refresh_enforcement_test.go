package service_test

// Task 7 of identity-user-enforcement: RefreshSession status gate and scopes.
// Uses only existing API: ErrAccountLocked, ErrInvalidRefreshToken, SetStatus on the
// in-memory user twin (not linked to the session twin, so a direct set revokes nothing).

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
)

type okAudit struct{}

func (okAudit) WriteUserSetStatus(context.Context, string, string, string, string, string, *time.Time) error {
	return nil
}

// setDirect writes a status straight into the user store: no session revoke, no outbox row.
func (e *refreshEnv) setDirect(t *testing.T, status string, until time.Time) {
	t.Helper()
	row := func() (repository.OutboxRow, error) {
		return repository.OutboxRow{EventID: uuid.NewString(), AggregateType: "User", AggregateID: e.uid, EventType: "fixture", Payload: []byte("x")}, nil
	}
	_, err := e.users.SetStatus(context.Background(), e.uid, status, "test_reason", until, e.clock.Now(),
		func(repository.Session) (repository.OutboxRow, error) { return row() },
		func(repository.StatusChange) (repository.OutboxRow, error) { return row() })
	if err != nil {
		t.Fatalf("set status: %v", err)
	}
}

func (e *refreshEnv) wantLocked(t *testing.T, tok string) {
	t.Helper()
	res, err := e.refresh(tok)
	if !errors.Is(err, service.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if res.Token != "" || res.RefreshToken != "" {
		t.Fatalf("refusal must issue no tokens: %+v", res)
	}
}

// scenario: Login enforcement: Surviving session of a banned user is refused and revoked
func TestRefreshBannedPermissionDeniedRevokes(t *testing.T) {
	e := newRefreshEnv(t)
	e.setDirect(t, repository.StatusBanned, time.Time{})
	e.wantLocked(t, e.first.RefreshToken)
	e.wantRevokedOnce(t)
}

// scenario: Login enforcement: Expired suspension is not lifted by refresh
func TestRefreshSuspendedNotLifted(t *testing.T) {
	e := newRefreshEnv(t)
	e.setDirect(t, repository.StatusSuspended, e.clock.Now().Add(-time.Minute))
	e.wantLocked(t, e.first.RefreshToken)
	u, err := e.users.GetByID(context.Background(), e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != repository.StatusSuspended {
		t.Fatalf("refresh must never lift a suspension, status = %q", u.Status)
	}
	e.wantRevokedOnce(t)
}

// scenario: Login enforcement: Unknown token is still generic
func TestRefreshUnknownTokenBeforeStatusCheck(t *testing.T) {
	e := newRefreshEnv(t)
	e.setDirect(t, repository.StatusBanned, time.Time{})
	res, err := e.refresh("unknown-token-value-that-matches-nothing")
	if !errors.Is(err, service.ErrInvalidRefreshToken) || errors.Is(err, service.ErrAccountLocked) {
		t.Fatalf("want generic ErrInvalidRefreshToken, got %v", err)
	}
	if res.Token != "" {
		t.Fatalf("no token expected: %+v", res)
	}
	e.wantNotRevoked(t)
}

// scenario: Login enforcement: Restricted user refreshes without listing.write
func TestRefreshRestrictedDropsListingWrite(t *testing.T) {
	e := newSellerRefreshEnv(t)
	if !hasScope(e.mustScopes(t, e.first.Token), "listing.write") {
		t.Fatalf("precondition: active seller must hold listing.write")
	}
	e.setDirect(t, repository.StatusRestricted, time.Time{})
	res := e.mustRefresh(t, e.first.RefreshToken)
	if hasScope(res.Scopes, "listing.write") {
		t.Fatalf("restricted result scopes must not contain listing.write: %v", res.Scopes)
	}
	_, claimScopes := jwtClaims(t, res.Token)
	if hasScope(claimScopes, "listing.write") || !hasScope(claimScopes, "listing.read") {
		t.Fatalf("token scopes must drop only listing.write: %v", claimScopes)
	}
}

// scenario: Login enforcement: Reinstated user regains listing.write at next refresh
func TestRefreshAfterReinstatementRestoresScope(t *testing.T) {
	e := newSellerRefreshEnv(t)
	e.setDirect(t, repository.StatusRestricted, time.Time{})
	res := e.mustRefresh(t, e.first.RefreshToken)
	if hasScope(res.Scopes, "listing.write") {
		t.Fatalf("restricted must lack listing.write: %v", res.Scopes)
	}
	e.setDirect(t, repository.StatusActive, time.Time{})
	res2 := e.mustRefresh(t, res.RefreshToken)
	_, claimScopes := jwtClaims(t, res2.Token)
	if !hasScope(res2.Scopes, "listing.write") || !hasScope(claimScopes, "listing.write") {
		t.Fatalf("reinstated seller must regain listing.write: %v / %v", res2.Scopes, claimScopes)
	}
}

// scenario: User enforcement: Refresh tokens of revoked sessions are dead
func TestBannedUserRefreshTokenDead(t *testing.T) {
	e := newRefreshEnv(t)
	e.users.AttachSessions(e.sess.InMemorySessionRepository)
	e.svc.WithAudit(okAudit{})
	if _, err := e.svc.SetUserStatus(context.Background(), "admin-actor", e.uid, repository.StatusBanned, "abuse", "", nil); err != nil {
		t.Fatalf("ban: %v", err)
	}
	before := e.sessionRow(t, e.uid)
	outboxBefore := len(e.sess.OutboxRows())
	e.wantInvalid(t, e.first.RefreshToken)
	if after := e.sessionRow(t, e.uid); after != before {
		t.Fatalf("session row must not change: %+v -> %+v", before, after)
	}
	if n := len(e.sess.OutboxRows()); n != outboxBefore {
		t.Fatalf("outbox must not grow: %d -> %d", outboxBefore, n)
	}
}

func newSellerRefreshEnv(t *testing.T) *refreshEnv {
	t.Helper()
	e := &refreshEnv{
		users: &fakeUsers{InMemoryUserRepository: repository.NewInMemoryUserRepository()},
		sess:  &spySessions{InMemorySessionRepository: repository.NewInMemorySessionRepository()},
		clock: &fakeClock{t: t0},
	}
	e.svc = service.NewAuthService(e.users, testSigner(t), time.Hour).WithSessions(e.sess).WithClock(e.clock.Now)
	res, err := e.svc.Register(context.Background(), "sally", "password123", "seller")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	e.uid, e.first = res.UserID, res
	return e
}

func (e *refreshEnv) mustScopes(t *testing.T, tok string) []string {
	t.Helper()
	_, s := jwtClaims(t, tok)
	return s
}
