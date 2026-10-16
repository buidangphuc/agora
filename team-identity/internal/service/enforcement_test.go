package service_test

// Task 6 of identity-user-enforcement: SetUserStatus and the login status gate.
//
// Contract pinned by these tests (the implementer adds exactly this; the
// enforcement_teststub.go declarations are replaced):
//
//	var ErrAccountLocked, ErrAuditUnavailable, ErrStatusPrecondition error
//	type AuditWriter interface { WriteUserSetStatus(ctx, actorID, userID, status, reasonCode, note string, until *time.Time) error }
//	(*AuthService).WithAudit(AuditWriter) *AuthService          // nil interface or typed-nil *upstream.AuditClient => ErrAuditUnavailable
//	(*AuthService).SetUserStatus(ctx, actorID, userID, status, reasonCode, note string, until *time.Time) (repository.StatusResult, error)
//	    status is a repository.Status* string. Errors (errors.Is): ErrInvalidInput (validation),
//	    repository.ErrNotFound, ErrStatusPrecondition (self / admin target), ErrAuditUnavailable;
//	    any other error = transaction failure (audit already written).
//	Login: refusal => errors.Is(err, ErrAccountLocked); failed lift => any error that is neither
//	    ErrAccountLocked nor ErrInvalidCredentials, no session.
//	The service reaches SetStatus / LiftExpiredSuspension through an interface assertion on the
//	user repository (so wrappers embedding the in-memory repo are honoured), and builds outbox rows
//	with events.SessionRevokedEventType / events.UserStatusChangedEventType (lift reason code
//	"suspension_expired", previous status SUSPENDED, status ACTIVE).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

var enfNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

type auditCall struct {
	actor, user, status, reason, note string
	until                             *time.Time
}

type fakeAudit struct {
	mu    sync.Mutex
	calls []auditCall
	err   error
	log   *[]string
	onHit func()
}

func (f *fakeAudit) WriteUserSetStatus(_ context.Context, actor, user, status, reason, note string, until *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, auditCall{actor, user, status, reason, note, until})
	if f.log != nil {
		*f.log = append(*f.log, "audit")
	}
	if f.onHit != nil {
		f.onHit()
	}
	return f.err
}

func (f *fakeAudit) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.calls) }

// txRepo wraps the in-memory user repo to observe order, inject failures and
// return a stale read once.
type txRepo struct {
	*repository.InMemoryUserRepository
	log      *[]string
	setErr   error
	liftErr  error
	stale    *repository.User
	liftHits int
}

func (r *txRepo) GetByUsername(ctx context.Context, name string) (repository.User, error) {
	if r.stale != nil && r.stale.Username == name {
		u := *r.stale
		r.stale = nil
		return u, nil
	}
	return r.InMemoryUserRepository.GetByUsername(ctx, name)
}

func (r *txRepo) SetStatus(ctx context.Context, userID, status, reason string, until, now time.Time, revoke repository.EnqueueFn, changed repository.StatusEnqueueFn) (repository.StatusResult, error) {
	*r.log = append(*r.log, "tx")
	if r.setErr != nil {
		return repository.StatusResult{}, r.setErr
	}
	return r.InMemoryUserRepository.SetStatus(ctx, userID, status, reason, until, now, revoke, changed)
}

func (r *txRepo) LiftExpiredSuspension(ctx context.Context, userID string, now time.Time, changed repository.StatusEnqueueFn) (bool, error) {
	r.liftHits++
	if r.liftErr != nil {
		return false, r.liftErr
	}
	return r.InMemoryUserRepository.LiftExpiredSuspension(ctx, userID, now, changed)
}

type enfEnv struct {
	t     *testing.T
	ctx   context.Context
	users *repository.InMemoryUserRepository
	wrap  *txRepo
	sess  *repository.InMemorySessionRepository
	audit *fakeAudit
	log   []string
	svc   *service.AuthService
}

func newEnfEnv(t *testing.T) *enfEnv {
	t.Helper()
	e := &enfEnv{t: t, ctx: context.Background(), users: repository.NewInMemoryUserRepository(), sess: repository.NewInMemorySessionRepository()}
	e.audit = &fakeAudit{log: &e.log}
	e.wrap = &txRepo{InMemoryUserRepository: e.users, log: &e.log}
	e.svc = service.NewAuthService(e.wrap, testSigner(t), time.Hour).WithClock(func() time.Time { return enfNow })
	return e
}

// start links the twins and enables sessions and audit. Seed users, sessions and
// fixture statuses BEFORE calling it.
func (e *enfEnv) start() {
	e.users.AttachSessions(e.sess)
	e.svc.WithSessions(e.sess).WithAudit(e.audit)
}

func (e *enfEnv) user(name, role string) string {
	e.t.Helper()
	if _, err := e.svc.Register(e.ctx, name, "password123", role); err != nil {
		e.t.Fatalf("register: %v", err)
	}
	u, err := e.users.GetByUsername(e.ctx, name)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.ID
}

func (e *enfEnv) admin(name string) string {
	e.t.Helper()
	if err := e.svc.EnsureAdmin(e.ctx, name, "password123"); err != nil {
		e.t.Fatal(err)
	}
	u, err := e.users.GetByUsername(e.ctx, name)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.ID
}

// seedSessions inserts n live sessions directly (before start()).
func (e *enfEnv) seedSessions(uid string, n int) {
	e.t.Helper()
	for i := 0; i < n; i++ {
		if _, err := e.sess.CreateSession(e.ctx, repository.Session{UserID: uid}); err != nil {
			e.t.Fatal(err)
		}
	}
}

// fixture sets a status directly in the store (before start(): no revokes, no outbox).
func (e *enfEnv) fixture(uid, status, reason string, until time.Time) {
	e.t.Helper()
	row := func() (repository.OutboxRow, error) {
		return repository.OutboxRow{EventID: uuid.NewString(), AggregateType: "User", AggregateID: uid, EventType: "fixture", Payload: []byte("x")}, nil
	}
	_, err := e.users.SetStatus(e.ctx, uid, status, reason, until, enfNow,
		func(repository.Session) (repository.OutboxRow, error) { return row() },
		func(repository.StatusChange) (repository.OutboxRow, error) { return row() })
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *enfEnv) stored(uid string) repository.User {
	e.t.Helper()
	u, err := e.users.GetByID(e.ctx, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *enfEnv) sessionCounts(uid string) (live, total int) {
	e.t.Helper()
	ss, err := e.sess.ListSessions(e.ctx, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, s := range ss {
		if !s.Revoked {
			live++
		}
	}
	return live, len(ss)
}

func (e *enfEnv) rows(eventType string) []repository.OutboxRow {
	var out []repository.OutboxRow
	for _, r := range e.sess.OutboxRows() {
		if r.EventType == eventType {
			out = append(out, r)
		}
	}
	return out
}

func (e *enfEnv) statusEvents() []*identityv1.UserStatusChanged {
	e.t.Helper()
	var out []*identityv1.UserStatusChanged
	for _, r := range e.rows(events.UserStatusChangedEventType) {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(r.Payload, &env); err != nil {
			e.t.Fatal(err)
		}
		var ev identityv1.UserStatusChanged
		if err := proto.Unmarshal(env.Payload, &ev); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func (e *enfEnv) failedLogins(uid string) int {
	items, _, err := e.sess.ListLoginHistory(e.ctx, uid, 50, 0)
	if err != nil {
		e.t.Fatal(err)
	}
	n := 0
	for _, it := range items {
		if !it.Success {
			n++
		}
	}
	return n
}

func hasScope(scopes []string, s string) bool {
	for _, x := range scopes {
		if x == s {
			return true
		}
	}
	return false
}

func tp(t time.Time) *time.Time { return &t }

// ---------------------------------------------------------------- SetUserStatus

func TestSetUserStatusBanAuditsAndRevokes(t *testing.T) { // User enforcement: Ban writes one audit event and revokes sessions
	e := newEnfEnv(t)
	uid, admin := e.user("buyer1", "buyer"), e.admin("root")
	e.seedSessions(uid, 3)
	e.start()
	res, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusBanned, "scam", "a note", nil)
	if err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if res.SessionsRevoked != 3 {
		t.Fatalf("sessions revoked = %d, want 3", res.SessionsRevoked)
	}
	if u := e.stored(uid); u.Status != repository.StatusBanned || u.StatusReason != "scam" {
		t.Fatalf("stored user = %+v", u)
	}
	if live, total := e.sessionCounts(uid); live != 0 || total != 3 {
		t.Fatalf("live=%d total=%d", live, total)
	}
	if e.audit.count() != 1 {
		t.Fatalf("audit calls = %d, want 1", e.audit.count())
	}
	c := e.audit.calls[0]
	if c.actor != admin || c.user != uid || c.status != "banned" || c.reason != "scam" || c.note != "a note" || c.until != nil {
		t.Fatalf("audit call = %+v", c)
	}
	if n := len(e.rows(events.SessionRevokedEventType)); n != 3 {
		t.Fatalf("SessionRevoked rows = %d, want 3", n)
	}
	if evs := e.statusEvents(); len(evs) != 1 || evs[0].Status != identityv1.UserStatus_USER_STATUS_BANNED || evs[0].PreviousStatus != identityv1.UserStatus_USER_STATUS_ACTIVE {
		t.Fatalf("status events = %v", evs)
	}
}

func TestSetUserStatusAuditPrecedesChange(t *testing.T) { // User enforcement: Audit is written before the change
	e := newEnfEnv(t)
	uid, admin := e.user("buyer1", "buyer"), e.admin("root")
	e.start()
	var statusAtAudit string
	e.audit.onHit = func() { statusAtAudit = e.stored(uid).Status }
	if _, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusBanned, "scam", "", nil); err != nil {
		t.Fatal(err)
	}
	if len(e.log) != 2 || e.log[0] != "audit" || e.log[1] != "tx" {
		t.Fatalf("call order = %v, want [audit tx]", e.log)
	}
	if statusAtAudit != repository.StatusActive {
		t.Fatalf("user already changed when audit was written: %q", statusAtAudit)
	}
	if c := e.audit.calls[0]; c.user != uid || c.actor != admin {
		t.Fatalf("audit ids = %+v", c)
	}
}

func TestSetUserStatusTxFailureAfterAudit(t *testing.T) { // User enforcement: Change failure after audit keeps the audit row
	e := newEnfEnv(t)
	uid, admin := e.user("buyer1", "buyer"), e.admin("root")
	e.seedSessions(uid, 2)
	e.start()
	e.wrap.setErr = errors.New("tx exploded")
	_, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusBanned, "scam", "", nil)
	if err == nil || errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("want internal (non-audit) error, got %v", err)
	}
	if e.audit.count() != 1 {
		t.Fatalf("audit calls = %d, want 1 (not retracted)", e.audit.count())
	}
	if e.stored(uid).Status != repository.StatusActive {
		t.Fatal("user changed despite tx failure")
	}
	if live, _ := e.sessionCounts(uid); live != 2 {
		t.Fatalf("live sessions = %d, want 2", live)
	}
	if len(e.sess.OutboxRows()) != 0 {
		t.Fatal("outbox rows written despite tx failure")
	}
}

func TestSetUserStatusNilAuditUnavailable(t *testing.T) { // User enforcement: No audit upstream configured
	var typedNil *upstream.AuditClient
	cases := map[string]service.AuditWriter{"nil interface": nil, "typed nil client": typedNil}
	for name, w := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnfEnv(t)
			uid, admin := e.user("buyer1", "buyer"), e.admin("root")
			e.seedSessions(uid, 1)
			e.start()
			e.svc.WithAudit(w)
			_, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusBanned, "scam", "", nil)
			if !errors.Is(err, service.ErrAuditUnavailable) {
				t.Fatalf("want ErrAuditUnavailable, got %v", err)
			}
			if e.stored(uid).Status != repository.StatusActive {
				t.Fatal("user changed without audit")
			}
			if live, _ := e.sessionCounts(uid); live != 1 {
				t.Fatal("session revoked without audit")
			}
			if len(e.sess.OutboxRows()) != 0 {
				t.Fatal("outbox rows without audit")
			}
		})
	}
}

func TestSetUserStatusAuditErrorUnavailableNoChange(t *testing.T) { // User enforcement: Audit down refuses the change
	e := newEnfEnv(t)
	uid, admin := e.user("buyer1", "buyer"), e.admin("root")
	e.seedSessions(uid, 1)
	e.start()
	for _, ae := range []error{errors.New("down"), context.DeadlineExceeded} {
		e.audit.err = ae
		_, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusSuspended, "spam", "", tp(enfNow.Add(time.Hour)))
		if !errors.Is(err, service.ErrAuditUnavailable) {
			t.Fatalf("audit err %v: want ErrAuditUnavailable, got %v", ae, err)
		}
	}
	if e.stored(uid).Status != repository.StatusActive || len(e.sess.OutboxRows()) != 0 {
		t.Fatal("state changed while audit down")
	}
	for _, c := range e.log {
		if c == "tx" {
			t.Fatal("transaction invoked while audit down")
		}
	}
}

func TestSetUserStatusRestrictRevokesSessions(t *testing.T) { // User enforcement: Restrict revokes sessions and keeps the user able to log in
	e := newEnfEnv(t)
	uid, admin := e.user("seller1", "seller"), e.admin("root")
	e.seedSessions(uid, 2)
	e.start()
	res, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusRestricted, "counterfeit", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionsRevoked != 2 || e.stored(uid).Status != repository.StatusRestricted {
		t.Fatalf("res=%+v user=%+v", res, e.stored(uid))
	}
	if _, err := e.svc.Login(e.ctx, "seller1", "password123"); err != nil {
		t.Fatalf("restricted user must still log in: %v", err)
	}
}

func TestSetUserStatusActiveRevokesNothing(t *testing.T) { // User enforcement: Reinstating to active revokes nothing
	cases := []struct {
		name   string
		status string
		until  time.Time
	}{
		{"restricted", repository.StatusRestricted, time.Time{}},
		{"suspended with until", repository.StatusSuspended, enfNow.Add(time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnfEnv(t)
			uid, admin := e.user("u1", "seller"), e.admin("root")
			e.seedSessions(uid, 1)
			e.fixture(uid, tc.status, "old", tc.until)
			e.start()
			// fixture ran before attach: the seeded session is still live.
			res, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusActive, "appeal_accepted", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if res.SessionsRevoked != 0 {
				t.Fatalf("revoked = %d, want 0", res.SessionsRevoked)
			}
			if live, _ := e.sessionCounts(uid); live != 1 {
				t.Fatal("session revoked on reinstatement")
			}
			u := e.stored(uid)
			if u.Status != repository.StatusActive || u.StatusReason != "appeal_accepted" || !u.StatusUntil.IsZero() || !u.StatusChangedAt.Equal(enfNow) {
				t.Fatalf("stored = %+v", u)
			}
			if len(e.rows(events.SessionRevokedEventType)) != 0 || len(e.statusEvents()) != 1 {
				t.Fatal("expected only one UserStatusChanged row")
			}
		})
	}
}

func TestSetUserStatusSuspendExtend(t *testing.T) { // User enforcement: Setting the same status updates reason and until
	e := newEnfEnv(t)
	uid, admin := e.user("u1", "buyer"), e.admin("root")
	e.seedSessions(uid, 1)
	e.fixture(uid, repository.StatusSuspended, "spam", enfNow.Add(time.Hour))
	e.start()
	later := enfNow.Add(48 * time.Hour)
	res, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusSuspended, "spam_again", "", &later)
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionsRevoked != 1 {
		t.Fatalf("revoked = %d, want 1", res.SessionsRevoked)
	}
	if u := e.stored(uid); !u.StatusUntil.Equal(later) || u.StatusReason != "spam_again" {
		t.Fatalf("stored = %+v", u)
	}
	if e.audit.count() != 1 || len(e.rows(events.SessionRevokedEventType)) != 1 {
		t.Fatal("want 1 audit and 1 SessionRevoked")
	}
	evs := e.statusEvents()
	if len(evs) != 1 || evs[0].PreviousStatus != identityv1.UserStatus_USER_STATUS_SUSPENDED {
		t.Fatalf("status events = %v", evs)
	}
}

func TestSetUserStatusNotFoundNoAudit(t *testing.T) { // User enforcement: Unknown user is NotFound without audit
	e := newEnfEnv(t)
	admin := e.admin("root")
	e.start()
	_, err := e.svc.SetUserStatus(e.ctx, admin, uuid.NewString(), repository.StatusBanned, "scam", "", nil)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if e.audit.count() != 0 {
		t.Fatal("audit written for unknown user")
	}
}

func TestSetUserStatusSelfRefused(t *testing.T) { // User enforcement: Admin cannot change their own status
	e := newEnfEnv(t)
	admin := e.admin("root")
	e.start()
	_, err := e.svc.SetUserStatus(e.ctx, admin, admin, repository.StatusBanned, "scam", "", nil)
	if !errors.Is(err, service.ErrStatusPrecondition) {
		t.Fatalf("want ErrStatusPrecondition, got %v", err)
	}
	if e.audit.count() != 0 || e.stored(admin).Status != repository.StatusActive {
		t.Fatal("audit written or user changed")
	}
}

func TestSetUserStatusAdminTargetRefused(t *testing.T) { // User enforcement: Admin cannot change another admin
	e := newEnfEnv(t)
	a1, a2 := e.admin("root"), e.admin("root2")
	e.start()
	_, err := e.svc.SetUserStatus(e.ctx, a1, a2, repository.StatusBanned, "scam", "", nil)
	if !errors.Is(err, service.ErrStatusPrecondition) {
		t.Fatalf("want ErrStatusPrecondition, got %v", err)
	}
	if e.audit.count() != 0 || e.stored(a2).Status != repository.StatusActive {
		t.Fatal("audit written or admin changed")
	}
}

func TestSetUserStatusServiceValidatesBeforeAudit(t *testing.T) { // User enforcement: Invalid input is rejected before any audit
	e := newEnfEnv(t)
	uid, admin := e.user("u1", "buyer"), e.admin("root")
	e.start()
	cases := map[string]struct {
		user, status, reason string
		until                *time.Time
	}{
		"unknown status":      {uid, "frozen", "scam", nil},
		"empty status":        {uid, "", "scam", nil},
		"empty reason":        {uid, repository.StatusBanned, "", nil},
		"empty user id":       {"", repository.StatusBanned, "scam", nil},
		"suspended no until":  {uid, repository.StatusSuspended, "scam", nil},
		"suspended until now": {uid, repository.StatusSuspended, "scam", tp(enfNow)},
		"suspended past":      {uid, repository.StatusSuspended, "scam", tp(enfNow.Add(-time.Minute))},
		"banned with until":   {uid, repository.StatusBanned, "scam", tp(enfNow.Add(time.Hour))},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := e.svc.SetUserStatus(e.ctx, admin, c.user, c.status, c.reason, "", c.until)
			if !errors.Is(err, service.ErrInvalidInput) {
				t.Fatalf("want ErrInvalidInput, got %v", err)
			}
			if e.audit.count() != 0 {
				t.Fatal("audit written for invalid input")
			}
		})
	}
}

// ---------------------------------------------------------------------- Login

func TestLoginBannedPermissionDenied(t *testing.T) { // Login enforcement: Banned user is refused after a correct password
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, repository.StatusBanned, "scam", time.Time{})
	e.start()
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if !errors.Is(err, service.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if _, total := e.sessionCounts(uid); total != 0 {
		t.Fatalf("sessions created: %d", total)
	}
	if e.failedLogins(uid) != 1 {
		t.Fatalf("failed attempts = %d, want 1", e.failedLogins(uid))
	}
}

func TestLoginSuspendedPermissionDenied(t *testing.T) { // Login enforcement: Suspended user is refused until the end time
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, repository.StatusSuspended, "spam", enfNow.Add(time.Hour))
	e.start()
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if !errors.Is(err, service.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if _, total := e.sessionCounts(uid); total != 0 {
		t.Fatal("session created")
	}
	if e.stored(uid).Status != repository.StatusSuspended {
		t.Fatal("status changed")
	}
}

func TestLoginWrongPasswordNoStatusOracle(t *testing.T) { // Login enforcement: Wrong password reveals no status
	e := newEnfEnv(t)
	b, s, a := e.user("banned1", "buyer"), e.user("susp1", "buyer"), e.user("active1", "buyer")
	_ = a
	e.fixture(b, repository.StatusBanned, "scam", time.Time{})
	e.fixture(s, repository.StatusSuspended, "spam", enfNow.Add(time.Hour))
	e.start()
	for _, name := range []string{"banned1", "susp1", "active1"} {
		_, err := e.svc.Login(e.ctx, name, "wrong-password")
		if err != service.ErrInvalidCredentials {
			t.Fatalf("%s: want exactly ErrInvalidCredentials, got %v", name, err)
		}
	}
}

func TestLoginRestrictedAndActiveSucceed(t *testing.T) { // Login enforcement: Restricted and active users log in
	e := newEnfEnv(t)
	r, a := e.user("restr1", "seller"), e.user("active1", "seller")
	e.fixture(r, repository.StatusRestricted, "counterfeit", time.Time{})
	e.start()
	for name, id := range map[string]string{"restr1": r, "active1": a} {
		if _, err := e.svc.Login(e.ctx, name, "password123"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if live, total := e.sessionCounts(id); live != 1 || total != 1 {
			t.Fatalf("%s: live=%d total=%d", name, live, total)
		}
	}
}

func TestLoginRestrictedSellerNoListingWrite(t *testing.T) { // Login enforcement: Restricted seller token lacks listing.write
	e := newEnfEnv(t)
	r := e.user("restr1", "seller")
	e.fixture(r, repository.StatusRestricted, "counterfeit", time.Time{})
	e.start()
	res, err := e.svc.Login(e.ctx, "restr1", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if hasScope(res.Scopes, "listing.write") {
		t.Fatalf("restricted scopes contain listing.write: %v", res.Scopes)
	}
	for _, s := range []string{"listing.read", "engagement:write", "search:write"} {
		if !hasScope(res.Scopes, s) {
			t.Fatalf("missing scope %s in %v", s, res.Scopes)
		}
	}
}

func TestLoginLiftsExpiredSuspension(t *testing.T) { // Login enforcement: Expired suspension is lifted
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, repository.StatusSuspended, "spam", enfNow.Add(-time.Second))
	e.start()
	if _, err := e.svc.Login(e.ctx, "u1", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if u := e.stored(uid); u.Status != repository.StatusActive || !u.StatusUntil.IsZero() {
		t.Fatalf("stored = %+v", u)
	}
	evs := e.statusEvents()
	if len(evs) != 1 || evs[0].ReasonCode != "suspension_expired" || evs[0].PreviousStatus != identityv1.UserStatus_USER_STATUS_SUSPENDED || evs[0].Status != identityv1.UserStatus_USER_STATUS_ACTIVE {
		t.Fatalf("status events = %v", evs)
	}
}

func TestLoginSuspensionBoundaryIsLifted(t *testing.T) { // Login enforcement: Suspension ending exactly now is lifted
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, repository.StatusSuspended, "spam", enfNow)
	e.start()
	if _, err := e.svc.Login(e.ctx, "u1", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if e.stored(uid).Status != repository.StatusActive {
		t.Fatal("not lifted at the boundary")
	}
}

func TestLoginLiftMissRereadsStatus(t *testing.T) { // Login enforcement: Ban racing the lift refuses the login
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, repository.StatusSuspended, "spam", enfNow.Add(-time.Minute))
	stale := e.stored(uid)
	e.fixture(uid, repository.StatusBanned, "scam", time.Time{}) // ban lands after Login's read
	e.wrap.stale = &stale
	e.start()
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if !errors.Is(err, service.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if _, total := e.sessionCounts(uid); total != 0 {
		t.Fatal("session created")
	}
	if e.stored(uid).Status != repository.StatusBanned {
		t.Fatal("ban overwritten")
	}
	for _, ev := range e.statusEvents() {
		if ev.ReasonCode == "suspension_expired" {
			t.Fatal("lift event emitted")
		}
	}
}

func TestLoginBanRaceNoLiveSession(t *testing.T) { // Login enforcement: Ban committed during login leaves no live session
	t.Run("ban before the session insert", func(t *testing.T) {
		e := newEnfEnv(t)
		uid := e.user("u1", "buyer")
		stale := e.stored(uid) // Login passes the gate on this active read
		e.fixture(uid, repository.StatusBanned, "scam", time.Time{})
		e.wrap.stale = &stale
		e.start()
		_, err := e.svc.Login(e.ctx, "u1", "password123")
		if !errors.Is(err, service.ErrAccountLocked) {
			t.Fatalf("want ErrAccountLocked, got %v", err)
		}
		if _, total := e.sessionCounts(uid); total != 0 {
			t.Fatalf("sessions = %d, want 0", total)
		}
	})
	t.Run("session insert before the ban", func(t *testing.T) {
		e := newEnfEnv(t)
		uid, admin := e.user("u1", "buyer"), e.admin("root")
		e.start()
		if _, err := e.svc.Login(e.ctx, "u1", "password123"); err != nil {
			t.Fatal(err)
		}
		res, err := e.svc.SetUserStatus(e.ctx, admin, uid, repository.StatusBanned, "scam", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.SessionsRevoked != 1 || len(e.rows(events.SessionRevokedEventType)) != 1 {
			t.Fatalf("res=%+v", res)
		}
		if live, _ := e.sessionCounts(uid); live != 0 {
			t.Fatal("live session after ban")
		}
	})
}

func TestLoginLiftFailureFailsClosed(t *testing.T) { // Login enforcement: Failed lift fails the login closed
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, repository.StatusSuspended, "spam", enfNow.Add(-time.Minute))
	e.start()
	e.wrap.liftErr = errors.New("db down")
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if err == nil || errors.Is(err, service.ErrAccountLocked) || errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("want an unavailable-class error, got %v", err)
	}
	if e.wrap.liftHits == 0 {
		t.Fatal("lift was never attempted")
	}
	if _, total := e.sessionCounts(uid); total != 0 {
		t.Fatal("session created")
	}
	if e.stored(uid).Status != repository.StatusSuspended {
		t.Fatal("status changed")
	}
}

func TestLoginLiftNotAudited(t *testing.T) { // Login enforcement: Lift writes no audit event
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, repository.StatusSuspended, "spam", enfNow.Add(-time.Second))
	e.start()
	if _, err := e.svc.Login(e.ctx, "u1", "password123"); err != nil {
		t.Fatal(err)
	}
	if e.wrap.liftHits == 0 {
		t.Fatal("lift was never attempted")
	}
	if e.audit.count() != 0 {
		t.Fatalf("audit calls = %d, want 0", e.audit.count())
	}
}

func TestLoginBannedNeverLifted(t *testing.T) { // Login enforcement: A banned user is never lifted by time
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, repository.StatusBanned, "scam", enfNow.Add(-24*time.Hour))
	e.start()
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if !errors.Is(err, service.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if e.stored(uid).Status != repository.StatusBanned {
		t.Fatal("banned user was lifted")
	}
}
