package identity_test

// Task 1.3 (identity-seller-onboarding): AuthService.BecomeSeller.
//
// Service API assumed by these tests (the implementer must provide it):
//   - (*AuthService).WithSellerTerms(versions []string) *AuthService  // first entry = current version
//   - (*AuthService).BecomeSeller(ctx, sid, termsVersion string) (AuthResult, error)
//     principal comes from auth.FromContext(ctx); sid is the caller's session id
//   - the user repository passed to NewAuthService implements
//     GrantSeller(ctx, userID, version string, now time.Time, enqueue store.SellerEnqueueFn, audit func(context.Context) error) (store.SellerGrantResult, error)
//     with the semantics of PostgresUserRepository.GrantSeller (store.ErrNotActive for a non-ACTIVE
//     user, idempotent for a seller, enqueue and audit called only on a first grant, an audit error rolls back)
//   - errors: ErrInvalidInput (bad terms version), store.ErrNotActive or ErrAccountLocked
//     (non-ACTIVE), store.ErrSessionNotFound (revoked or unknown session)
//   - the access token is signed on the caller's sid; AuthResult.RefreshToken stays empty; no
//     session refresh rotation; a first grant goes through the audit gate (auth.become_seller).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

const bsCurrent = "V1"

type grantRecord struct {
	version string
	at      time.Time
}

type sellerUsers struct {
	*storetest.InMemoryUserRepository
	mu     sync.Mutex
	roles  map[string][]string
	terms  map[string]grantRecord
	outbox []store.OutboxRow
	grants int
}

func newSellerUsers() *sellerUsers {
	return &sellerUsers{
		InMemoryUserRepository: storetest.NewInMemoryUserRepository(),
		roles:                  map[string][]string{},
		terms:                  map[string]grantRecord{},
	}
}

func (f *sellerUsers) GetByID(ctx context.Context, id string) (store.User, error) {
	u, err := f.InMemoryUserRepository.GetByID(ctx, id)
	if err != nil {
		return u, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.roles[id]; ok {
		u.Roles = append([]string(nil), r...)
	}
	return u, nil
}

func (f *sellerUsers) GrantSeller(ctx context.Context, userID, version string, now time.Time, enqueue store.SellerEnqueueFn, audit func(context.Context) error) (store.SellerGrantResult, error) {
	u, err := f.GetByID(ctx, userID)
	if err != nil {
		return store.SellerGrantResult{}, err
	}
	if u.Status != store.StatusActive {
		return store.SellerGrantResult{}, store.ErrNotActive
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grants++
	for _, r := range u.Roles {
		if r == "seller" {
			return store.SellerGrantResult{User: u}, nil
		}
	}
	row, err := enqueue(store.SellerGrant{UserID: userID, Role: "seller", GrantedAt: now})
	if err != nil {
		return store.SellerGrantResult{}, err
	}
	if audit != nil {
		if err := audit(ctx); err != nil {
			return store.SellerGrantResult{}, err
		}
	}
	f.outbox = append(f.outbox, row)
	u.Roles = append(u.Roles, "seller")
	f.roles[userID] = u.Roles
	f.terms[userID] = grantRecord{version: version, at: now}
	return store.SellerGrantResult{User: u, Granted: true}, nil
}

func (f *sellerUsers) rolesOf(t *testing.T, id string) []string {
	t.Helper()
	u, err := f.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return u.Roles
}

func (f *sellerUsers) outboxLen() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.outbox) }

type countingAudit struct {
	mu     sync.Mutex
	n      int
	down   bool
	events []auditx.Event
}

func (c *countingAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	if c.down {
		return errors.New("audit unavailable")
	}
	c.events = append(c.events, ev)
	return nil
}

func (c *countingAudit) setDown(down bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.down = down
}

func (c *countingAudit) written(action string) []auditx.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []auditx.Event
	for _, ev := range c.events {
		if ev.Action == action {
			out = append(out, ev)
		}
	}
	return out
}

type bsEnv struct {
	svc   *identity.AuthService
	users *sellerUsers
	sess  *storetest.InMemorySessionRepository
	audit *countingAudit
	uid   string
	sid   string
	first identity.AuthResult
}

func newBSEnv(t *testing.T, role string) *bsEnv {
	t.Helper()
	e := &bsEnv{users: newSellerUsers(), sess: storetest.NewInMemorySessionRepository(), audit: &countingAudit{}}
	e.svc = newSvcAudit(t, e.users, e.sess, e.audit).
		WithClock(func() time.Time { return t0 }).
		WithSellerTerms([]string{bsCurrent, "V0"})
	res, err := e.svc.Register(context.Background(), "alice", "password123", role)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	e.uid, e.first = res.UserID, res
	e.sid, _ = jwtClaims(t, res.Token)
	if e.sid == "" {
		t.Fatalf("registered token carries no sid")
	}
	e.audit.mu.Lock()
	e.audit.n = 0
	e.audit.events = nil
	e.audit.mu.Unlock()
	return e
}

func (e *bsEnv) setStatus(t *testing.T, status string) {
	t.Helper()
	noRow := func(store.Session) (store.OutboxRow, error) { return store.OutboxRow{}, nil }
	chg := func(store.StatusChange) (store.OutboxRow, error) { return store.OutboxRow{}, nil }
	if _, err := e.users.SetStatus(context.Background(), e.uid, status, "test", time.Now().Add(time.Hour), t0, noRow, chg); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
}

func (e *bsEnv) call(sid, version string) (identity.AuthResult, error) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: e.uid, Type: auth.TypeUser})
	return e.svc.BecomeSeller(ctx, sid, version)
}

func hasString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestBecomeSellerGrantsRoleAndToken(t *testing.T) {
	e := newBSEnv(t, "buyer")
	if hasString(e.first.Scopes, "listing.write") {
		t.Fatalf("buyer must not start with listing.write")
	}
	res, err := e.call(e.sid, bsCurrent)
	if err != nil {
		t.Fatalf("BecomeSeller: %v", err)
	}
	sid, scopes := jwtClaims(t, res.Token)
	if sid != e.sid {
		t.Fatalf("sid = %q, want caller sid %q", sid, e.sid)
	}
	if !hasString(scopes, "listing.write") || !hasString(res.Scopes, "listing.write") {
		t.Fatalf("scopes = %v / %v, want listing.write", scopes, res.Scopes)
	}
	roles := e.users.rolesOf(t, e.uid)
	if !hasString(roles, "buyer") || !hasString(roles, "seller") {
		t.Fatalf("roles = %v, want buyer and seller", roles)
	}
	if res.UserID != e.uid {
		t.Fatalf("UserID = %q, want %q", res.UserID, e.uid)
	}
	if e.users.outboxLen() != 1 {
		t.Fatalf("outbox rows = %d, want 1", e.users.outboxLen())
	}
}

func TestBecomeSellerKeepsRefreshToken(t *testing.T) {
	e := newBSEnv(t, "buyer")
	res, err := e.call(e.sid, bsCurrent)
	if err != nil {
		t.Fatalf("BecomeSeller: %v", err)
	}
	if res.RefreshToken != "" || !res.RefreshExpiresAt.IsZero() {
		t.Fatalf("BecomeSeller must not return a refresh token, got %q / %v", res.RefreshToken, res.RefreshExpiresAt)
	}
	rr, err := e.svc.RefreshSession(context.Background(), e.first.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshSession with the login refresh token: %v", err)
	}
	sid, scopes := jwtClaims(t, rr.Token)
	if sid != e.sid {
		t.Fatalf("refreshed sid = %q, want %q", sid, e.sid)
	}
	if !hasString(scopes, "listing.write") {
		t.Fatalf("refreshed scopes = %v, want listing.write", scopes)
	}
}

func TestBecomeSellerNonActiveDenied(t *testing.T) {
	e := newBSEnv(t, "buyer")
	e.setStatus(t, store.StatusRestricted)
	res, err := e.call(e.sid, bsCurrent)
	if !isNotActive(err) {
		t.Fatalf("err = %v, want not-active refusal", err)
	}
	if res.Token != "" {
		t.Fatalf("a refused call must carry no token")
	}
	if hasString(e.users.rolesOf(t, e.uid), "seller") {
		t.Fatalf("seller role must not be granted")
	}
	if e.users.outboxLen() != 0 {
		t.Fatalf("outbox rows = %d, want 0", e.users.outboxLen())
	}
}

func isNotActive(err error) bool {
	return errors.Is(err, store.ErrNotActive) || errors.Is(err, identity.ErrAccountLocked)
}

func TestBecomeSellerExistingSellerNonActiveDenied(t *testing.T) {
	for _, st := range []string{store.StatusRestricted, store.StatusSuspended, store.StatusBanned} {
		t.Run(st, func(t *testing.T) {
			e := newBSEnv(t, "seller")
			if !hasString(e.users.rolesOf(t, e.uid), "seller") {
				t.Fatalf("fixture user must already be a seller")
			}
			e.setStatus(t, st)
			res, err := e.call(e.sid, bsCurrent)
			if !isNotActive(err) {
				t.Fatalf("err = %v, want not-active refusal", err)
			}
			if res.Token != "" {
				t.Fatalf("a refused call must carry no token")
			}
			if e.users.outboxLen() != 0 {
				t.Fatalf("outbox rows = %d, want 0", e.users.outboxLen())
			}
		})
	}
}

func TestBecomeSellerRevokedSession(t *testing.T) {
	e := newBSEnv(t, "buyer")
	if err := e.sess.RevokeSession(context.Background(), e.sid, e.uid, nil); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	res, err := e.call(e.sid, bsCurrent)
	if !errors.Is(err, store.ErrSessionNotFound) {
		t.Fatalf("err = %v, want store.ErrSessionNotFound", err)
	}
	if res.Token != "" {
		t.Fatalf("a refused call must carry no token")
	}
	if hasString(e.users.rolesOf(t, e.uid), "seller") {
		t.Fatalf("seller role must not be granted")
	}
	if e.users.outboxLen() != 0 || e.users.grants != 0 {
		t.Fatalf("outbox = %d grants = %d, want none", e.users.outboxLen(), e.users.grants)
	}
}

const becomeSellerAction = "auth.become_seller"

// Scenario: Seller onboarding: a first grant writes one audit entry
func TestBecomeSellerWritesAuditEntry(t *testing.T) {
	e := newBSEnv(t, "buyer")
	res, err := e.call(e.sid, bsCurrent)
	if err != nil {
		t.Fatalf("BecomeSeller: %v", err)
	}
	got := e.audit.written(becomeSellerAction)
	if len(got) != 1 {
		t.Fatalf("%s entries = %d, want 1", becomeSellerAction, len(got))
	}
	ev := got[0]
	if ev.ActorID != e.uid || ev.TargetID != e.uid || ev.TargetType != "user" {
		t.Fatalf("entry actor=%q target=%s/%q, want the user %q", ev.ActorID, ev.TargetType, ev.TargetID, e.uid)
	}
	want := map[string]string{"terms_version": bsCurrent, "result": "granted"}
	if len(ev.Metadata) != len(want) {
		t.Fatalf("metadata = %v, want exactly %v", ev.Metadata, want)
	}
	for k, v := range want {
		if ev.Metadata[k] != v {
			t.Fatalf("metadata[%s] = %q, want %q", k, ev.Metadata[k], v)
		}
	}
	for k, v := range ev.Metadata {
		if v == e.sid || v == res.Token || v == e.first.Token || v == e.first.RefreshToken {
			t.Fatalf("metadata %s carries a secret or the session id", k)
		}
	}
}

// Scenario: Seller onboarding: audit unavailable refuses the grant
func TestBecomeSellerAuditDownRefusesGrant(t *testing.T) {
	e := newBSEnv(t, "buyer")
	e.audit.setDown(true)
	res, err := e.call(e.sid, bsCurrent)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v, want Unavailable", err)
	}
	if res.Token != "" {
		t.Fatalf("a refused call must carry no token")
	}
	if roles := e.users.rolesOf(t, e.uid); hasString(roles, "seller") || !hasString(roles, "buyer") {
		t.Fatalf("roles = %v, want buyer only", roles)
	}
	if _, ok := e.users.terms[e.uid]; ok {
		t.Fatalf("terms recorded although the audit write failed")
	}
	if e.users.outboxLen() != 0 {
		t.Fatalf("outbox rows = %d, want 0", e.users.outboxLen())
	}
	if n := len(e.audit.written(becomeSellerAction)); n != 0 {
		t.Fatalf("%s entries = %d, want 0", becomeSellerAction, n)
	}
}

// Scenario: Seller onboarding: a repeat call by a seller does not audit again
func TestBecomeSellerRepeatDoesNotAudit(t *testing.T) {
	e := newBSEnv(t, "buyer")
	if _, err := e.call(e.sid, bsCurrent); err != nil {
		t.Fatalf("first call: %v", err)
	}
	for _, down := range []bool{false, true} {
		e.audit.setDown(down)
		res, err := e.call(e.sid, bsCurrent)
		if err != nil {
			t.Fatalf("repeat call (audit down=%v): %v", down, err)
		}
		if sid, scopes := jwtClaims(t, res.Token); sid != e.sid || !hasString(scopes, "listing.write") {
			t.Fatalf("repeat token sid=%q scopes=%v", sid, scopes)
		}
		if n := len(e.audit.written(becomeSellerAction)); n != 1 {
			t.Fatalf("%s entries = %d after a repeat (audit down=%v), want 1", becomeSellerAction, n, down)
		}
	}
	if e.users.outboxLen() != 1 {
		t.Fatalf("outbox rows = %d, want 1", e.users.outboxLen())
	}
}

func TestBecomeSellerIdempotentForSeller(t *testing.T) {
	e := newBSEnv(t, "buyer")
	if _, err := e.call(e.sid, bsCurrent); err != nil {
		t.Fatalf("first call: %v", err)
	}
	firstTerms := e.users.terms[e.uid]
	res, err := e.svc.WithClock(func() time.Time { return t0.Add(time.Hour) }).
		BecomeSeller(auth.WithPrincipal(context.Background(), auth.Principal{ID: e.uid, Type: auth.TypeUser}), e.sid, bsCurrent)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	sid, scopes := jwtClaims(t, res.Token)
	if sid != e.sid || !hasString(scopes, "listing.write") {
		t.Fatalf("second token sid=%q scopes=%v", sid, scopes)
	}
	if got := e.users.terms[e.uid]; got != firstTerms {
		t.Fatalf("terms record changed: %v -> %v", firstTerms, got)
	}
	if e.users.outboxLen() != 1 {
		t.Fatalf("outbox rows = %d, want exactly 1", e.users.outboxLen())
	}
}

func TestBecomeSellerUnknownTermsVersion(t *testing.T) {
	for name, v := range map[string]string{"unknown": "V9", "empty": "", "older listed version": "V0"} {
		t.Run(name, func(t *testing.T) {
			e := newBSEnv(t, "buyer")
			res, err := e.call(e.sid, v)
			if !errors.Is(err, identity.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
			if res.Token != "" {
				t.Fatalf("a refused call must carry no token")
			}
			if hasString(e.users.rolesOf(t, e.uid), "seller") || e.users.outboxLen() != 0 {
				t.Fatalf("no role and no outbox row may be written")
			}
		})
	}
}

func TestBecomeSellerTermsVersionExactMatch(t *testing.T) {
	for name, v := range map[string]string{"leading space": " V1", "trailing space": "V1 ", "lower case": "v1", "newline": "V1\n"} {
		t.Run(name, func(t *testing.T) {
			e := newBSEnv(t, "buyer")
			_, err := e.call(e.sid, v)
			if !errors.Is(err, identity.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
			if hasString(e.users.rolesOf(t, e.uid), "seller") || e.users.outboxLen() != 0 {
				t.Fatalf("no role and no outbox row may be written")
			}
		})
	}
}
