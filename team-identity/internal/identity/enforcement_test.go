package identity_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

var enfNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

type auditCall struct {
	actor, user, status, reason, note, action, targetType string
	until                                                 *time.Time
}

type fakeAudit struct {
	mu    sync.Mutex
	calls []auditCall
	err   error
	log   *[]string
	onHit func()
}

func (f *fakeAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var until *time.Time
	if v, ok := ev.Metadata["until"]; ok {
		ut, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		until = &ut
	}
	f.calls = append(f.calls, auditCall{
		actor: ev.ActorID, user: ev.TargetID, status: ev.Metadata["status"], reason: ev.Metadata["reason_code"],
		note: ev.Metadata["note"], until: until, action: ev.Action, targetType: ev.TargetType,
	})
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
	*storetest.InMemoryUserRepository
	log      *[]string
	setErr   error
	liftErr  error
	stale    *store.User
	liftHits int
}

func (r *txRepo) GetByUsername(ctx context.Context, name string) (store.User, error) {
	if r.stale != nil && r.stale.Username == name {
		u := *r.stale
		r.stale = nil
		return u, nil
	}
	return r.InMemoryUserRepository.GetByUsername(ctx, name)
}

func (r *txRepo) SetStatus(ctx context.Context, userID, status, reason string, until, now time.Time, revoke store.EnqueueFn, changed store.StatusEnqueueFn) (store.StatusResult, error) {
	*r.log = append(*r.log, "tx")
	if r.setErr != nil {
		return store.StatusResult{}, r.setErr
	}
	return r.InMemoryUserRepository.SetStatus(ctx, userID, status, reason, until, now, revoke, changed)
}

func (r *txRepo) LiftExpiredSuspension(ctx context.Context, userID string, now time.Time, changed store.StatusEnqueueFn) (bool, error) {
	r.liftHits++
	if r.liftErr != nil {
		return false, r.liftErr
	}
	return r.InMemoryUserRepository.LiftExpiredSuspension(ctx, userID, now, changed)
}

type enfEnv struct {
	t     *testing.T
	ctx   context.Context
	users *storetest.InMemoryUserRepository
	reg   *identity.AuthService
	wrap  *txRepo
	sess  *storetest.InMemorySessionRepository
	audit *fakeAudit
	log   []string
	svc   *identity.AuthService
}

func newEnfEnv(t *testing.T) *enfEnv {
	t.Helper()
	e := &enfEnv{t: t, ctx: context.Background(), users: storetest.NewInMemoryUserRepository(), sess: storetest.NewInMemorySessionRepository()}
	e.audit = &fakeAudit{log: &e.log}
	e.wrap = &txRepo{InMemoryUserRepository: e.users, log: &e.log}
	e.svc = newSvcAudit(t, e.wrap, e.sess, e.audit).WithClock(func() time.Time { return enfNow })
	e.reg = newSvc(t, e.users, storetest.NewInMemorySessionRepository())
	return e
}

// start links the twins and enables sessions and audit. Seed users, sessions and
// fixture statuses BEFORE calling it.
func (e *enfEnv) start() {
	e.users.AttachSessions(e.sess)
}

func (e *enfEnv) user(name, role string) string {
	e.t.Helper()
	if _, err := e.reg.Register(e.ctx, name, "password123", role); err != nil {
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
	if err := e.reg.EnsureAdmin(e.ctx, name, "password123"); err != nil {
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
		if _, err := e.sess.CreateSession(e.ctx, store.Session{UserID: uid}); err != nil {
			e.t.Fatal(err)
		}
	}
}

// fixture sets a status directly in the store (before start(): no revokes, no outbox).
func (e *enfEnv) fixture(uid, status, reason string, until time.Time) {
	e.t.Helper()
	row := func() (store.OutboxRow, error) {
		return store.OutboxRow{EventID: uuid.NewString(), AggregateType: "User", AggregateID: uid, EventType: "fixture", Payload: []byte("x")}, nil
	}
	_, err := e.users.SetStatus(e.ctx, uid, status, reason, until, enfNow,
		func(store.Session) (store.OutboxRow, error) { return row() },
		func(store.StatusChange) (store.OutboxRow, error) { return row() })
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *enfEnv) stored(uid string) store.User {
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

func (e *enfEnv) rows(eventType string) []store.OutboxRow {
	var out []store.OutboxRow
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
	res, err := e.svc.SetUserStatus(e.ctx, admin, uid, store.StatusBanned, "scam", "a note", nil)
	if err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if res.SessionsRevoked != 3 {
		t.Fatalf("sessions revoked = %d, want 3", res.SessionsRevoked)
	}
	if u := e.stored(uid); u.Status != store.StatusBanned || u.StatusReason != "scam" {
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
	if _, err := e.svc.SetUserStatus(e.ctx, admin, uid, store.StatusBanned, "scam", "", nil); err != nil {
		t.Fatal(err)
	}
	if len(e.log) != 2 || e.log[0] != "audit" || e.log[1] != "tx" {
		t.Fatalf("call order = %v, want [audit tx]", e.log)
	}
	if statusAtAudit != store.StatusActive {
		t.Fatalf("user already changed when audit was written: %q", statusAtAudit)
	}
	if c := e.audit.calls[0]; c.user != uid || c.actor != admin || c.action != "user.set_status" || c.targetType != "user" {
		t.Fatalf("audit event = %+v", c)
	}
}

func TestSetUserStatusTxFailureAfterAudit(t *testing.T) { // User enforcement: Change failure after audit keeps the audit row
	e := newEnfEnv(t)
	uid, admin := e.user("buyer1", "buyer"), e.admin("root")
	e.seedSessions(uid, 2)
	e.start()
	e.wrap.setErr = errors.New("tx exploded")
	_, err := e.svc.SetUserStatus(e.ctx, admin, uid, store.StatusBanned, "scam", "", nil)
	if err == nil || status.Code(err) == codes.Unavailable {
		t.Fatalf("want internal (non-audit) error, got %v", err)
	}
	if e.audit.count() != 1 {
		t.Fatalf("audit calls = %d, want 1 (not retracted)", e.audit.count())
	}
	if e.stored(uid).Status != store.StatusActive {
		t.Fatal("user changed despite tx failure")
	}
	if live, _ := e.sessionCounts(uid); live != 2 {
		t.Fatalf("live sessions = %d, want 2", live)
	}
	if len(e.sess.OutboxRows()) != 0 {
		t.Fatal("outbox rows written despite tx failure")
	}
}

func TestSetUserStatusAuditDownUnavailableNoChange(t *testing.T) { // Audit down refuses the change
	e := newEnfEnv(t)
	uid, admin := e.user("buyer1", "buyer"), e.admin("root")
	e.seedSessions(uid, 1)
	e.start()
	for _, ae := range []error{errors.New("down"), context.DeadlineExceeded} {
		e.audit.err = ae
		_, err := e.svc.SetUserStatus(e.ctx, admin, uid, store.StatusSuspended, "spam", "", tp(enfNow.Add(time.Hour)))
		if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != auditx.Message {
			t.Fatalf("audit err %v: want Unavailable %q, got %v", ae, auditx.Message, err)
		}
	}
	if e.stored(uid).Status != store.StatusActive || len(e.sess.OutboxRows()) != 0 {
		t.Fatal("state changed while audit down")
	}
	if live, _ := e.sessionCounts(uid); live != 1 {
		t.Fatalf("live sessions = %d, want 1", live)
	}
	for _, c := range e.log {
		if c == "tx" {
			t.Fatal("transaction invoked while audit down")
		}
	}
}

func TestSetUserStatusNilWriterUnavailableNoChange(t *testing.T) { // Audit down refuses the change
	e := newEnfEnv(t)
	uid, admin := e.user("buyer1", "buyer"), e.admin("root")
	e.seedSessions(uid, 1)
	e.start()
	svc, err := identity.NewAuthService(e.wrap, e.sess, testSigner(t), time.Hour, newGate(nil))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SetUserStatus(e.ctx, admin, uid, store.StatusBanned, "scam", "", nil)
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != auditx.Message {
		t.Fatalf("want Unavailable %q, got %v", auditx.Message, err)
	}
	if e.stored(uid).Status != store.StatusActive || len(e.sess.OutboxRows()) != 0 {
		t.Fatal("state changed without a writer")
	}
}

func TestNewAuthServiceRequiresSessionStoreAndGate(t *testing.T) { // Sessions disabled yields no refresh token
	users := storetest.NewInMemoryUserRepository()
	sess := storetest.NewInMemorySessionRepository()
	cases := []struct {
		name     string
		sessions store.SessionRepository
		gate     *auditx.Gate
		wantErr  bool
	}{
		{"nil session store", nil, newGate(nopAudit{}), true},
		{"nil gate", sess, nil, true},
		{"both present", sess, newGate(nopAudit{}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := identity.NewAuthService(users, tc.sessions, testSigner(t), time.Hour, tc.gate)
			if tc.wantErr {
				if err == nil || svc != nil {
					t.Fatalf("want error and no service, got svc=%v err=%v", svc, err)
				}
				return
			}
			if err != nil || svc == nil {
				t.Fatalf("want a service, got svc=%v err=%v", svc, err)
			}
		})
	}
}

func TestAuditGateLogsNoTargetID(t *testing.T) { // The audit gate logs no target and no cause
	users := storetest.NewInMemoryUserRepository()
	sess := storetest.NewInMemorySessionRepository()
	users.AttachSessions(sess)
	if _, err := users.Create(context.Background(), store.User{ID: "u-secret", Username: "victim", PasswordHash: "x", Roles: []string{"buyer"}, Status: store.StatusActive}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gate := auditx.NewGate(failingAudit{err: errors.New("writer-cause-text")}, slog.New(slog.NewJSONHandler(&buf, nil)))
	svc, err := identity.NewAuthService(users, sess, testSigner(t), time.Hour, gate)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SetUserStatus(context.Background(), "admin-1", "u-secret", store.StatusBanned, "scam", "", nil)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	logs := buf.String()
	for _, want := range []string{"action", "target_type"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("log lacks %q:\n%s", want, logs)
		}
	}
	for _, secret := range []string{"u-secret", "writer-cause-text"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log leaks %q:\n%s", secret, logs)
		}
	}
}

type failingAudit struct{ err error }

func (f failingAudit) WriteAuditEvent(context.Context, auditx.Event) error { return f.err }

func TestSetUserStatusRestrictRevokesSessions(t *testing.T) { // User enforcement: Restrict revokes sessions and keeps the user able to log in
	e := newEnfEnv(t)
	uid, admin := e.user("seller1", "seller"), e.admin("root")
	e.seedSessions(uid, 2)
	e.start()
	res, err := e.svc.SetUserStatus(e.ctx, admin, uid, store.StatusRestricted, "counterfeit", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionsRevoked != 2 || e.stored(uid).Status != store.StatusRestricted {
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
		{"restricted", store.StatusRestricted, time.Time{}},
		{"suspended with until", store.StatusSuspended, enfNow.Add(time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnfEnv(t)
			uid, admin := e.user("u1", "seller"), e.admin("root")
			e.seedSessions(uid, 1)
			e.fixture(uid, tc.status, "old", tc.until)
			e.start()
			// fixture ran before attach: the seeded session is still live.
			res, err := e.svc.SetUserStatus(e.ctx, admin, uid, store.StatusActive, "appeal_accepted", "", nil)
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
			if u.Status != store.StatusActive || u.StatusReason != "appeal_accepted" || !u.StatusUntil.IsZero() || !u.StatusChangedAt.Equal(enfNow) {
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
	e.fixture(uid, store.StatusSuspended, "spam", enfNow.Add(time.Hour))
	e.start()
	later := enfNow.Add(48 * time.Hour)
	res, err := e.svc.SetUserStatus(e.ctx, admin, uid, store.StatusSuspended, "spam_again", "", &later)
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
	_, err := e.svc.SetUserStatus(e.ctx, admin, uuid.NewString(), store.StatusBanned, "scam", "", nil)
	if !errors.Is(err, store.ErrNotFound) {
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
	_, err := e.svc.SetUserStatus(e.ctx, admin, admin, store.StatusBanned, "scam", "", nil)
	if !errors.Is(err, identity.ErrStatusPrecondition) {
		t.Fatalf("want ErrStatusPrecondition, got %v", err)
	}
	if e.audit.count() != 0 || e.stored(admin).Status != store.StatusActive {
		t.Fatal("audit written or user changed")
	}
}

func TestSetUserStatusAdminTargetRefused(t *testing.T) { // User enforcement: Admin cannot change another admin
	e := newEnfEnv(t)
	a1, a2 := e.admin("root"), e.admin("root2")
	e.start()
	_, err := e.svc.SetUserStatus(e.ctx, a1, a2, store.StatusBanned, "scam", "", nil)
	if !errors.Is(err, identity.ErrStatusPrecondition) {
		t.Fatalf("want ErrStatusPrecondition, got %v", err)
	}
	if e.audit.count() != 0 || e.stored(a2).Status != store.StatusActive {
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
		"empty reason":        {uid, store.StatusBanned, "", nil},
		"empty user id":       {"", store.StatusBanned, "scam", nil},
		"suspended no until":  {uid, store.StatusSuspended, "scam", nil},
		"suspended until now": {uid, store.StatusSuspended, "scam", tp(enfNow)},
		"suspended past":      {uid, store.StatusSuspended, "scam", tp(enfNow.Add(-time.Minute))},
		"banned with until":   {uid, store.StatusBanned, "scam", tp(enfNow.Add(time.Hour))},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := e.svc.SetUserStatus(e.ctx, admin, c.user, c.status, c.reason, "", c.until)
			if !errors.Is(err, identity.ErrInvalidInput) {
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
	e.fixture(uid, store.StatusBanned, "scam", time.Time{})
	e.start()
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if !errors.Is(err, identity.ErrAccountLocked) {
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
	e.fixture(uid, store.StatusSuspended, "spam", enfNow.Add(time.Hour))
	e.start()
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if !errors.Is(err, identity.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if _, total := e.sessionCounts(uid); total != 0 {
		t.Fatal("session created")
	}
	if e.stored(uid).Status != store.StatusSuspended {
		t.Fatal("status changed")
	}
}

func TestLoginWrongPasswordNoStatusOracle(t *testing.T) { // Login enforcement: Wrong password reveals no status
	e := newEnfEnv(t)
	b, s, a := e.user("banned1", "buyer"), e.user("susp1", "buyer"), e.user("active1", "buyer")
	_ = a
	e.fixture(b, store.StatusBanned, "scam", time.Time{})
	e.fixture(s, store.StatusSuspended, "spam", enfNow.Add(time.Hour))
	e.start()
	for _, name := range []string{"banned1", "susp1", "active1"} {
		_, err := e.svc.Login(e.ctx, name, "wrong-password")
		if !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("%s: want exactly ErrInvalidCredentials, got %v", name, err)
		}
	}
}

func TestLoginRestrictedAndActiveSucceed(t *testing.T) { // Login enforcement: Restricted and active users log in
	e := newEnfEnv(t)
	r, a := e.user("restr1", "seller"), e.user("active1", "seller")
	e.fixture(r, store.StatusRestricted, "counterfeit", time.Time{})
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
	e.fixture(r, store.StatusRestricted, "counterfeit", time.Time{})
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
	e.fixture(uid, store.StatusSuspended, "spam", enfNow.Add(-time.Second))
	e.start()
	if _, err := e.svc.Login(e.ctx, "u1", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if u := e.stored(uid); u.Status != store.StatusActive || !u.StatusUntil.IsZero() {
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
	e.fixture(uid, store.StatusSuspended, "spam", enfNow)
	e.start()
	if _, err := e.svc.Login(e.ctx, "u1", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if e.stored(uid).Status != store.StatusActive {
		t.Fatal("not lifted at the boundary")
	}
}

func TestLoginLiftMissRereadsStatus(t *testing.T) { // Login enforcement: Ban racing the lift refuses the login
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, store.StatusSuspended, "spam", enfNow.Add(-time.Minute))
	stale := e.stored(uid)
	e.fixture(uid, store.StatusBanned, "scam", time.Time{}) // ban lands after Login's read
	e.wrap.stale = &stale
	e.start()
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if !errors.Is(err, identity.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if _, total := e.sessionCounts(uid); total != 0 {
		t.Fatal("session created")
	}
	if e.stored(uid).Status != store.StatusBanned {
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
		e.fixture(uid, store.StatusBanned, "scam", time.Time{})
		e.wrap.stale = &stale
		e.start()
		_, err := e.svc.Login(e.ctx, "u1", "password123")
		if !errors.Is(err, identity.ErrAccountLocked) {
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
		res, err := e.svc.SetUserStatus(e.ctx, admin, uid, store.StatusBanned, "scam", "", nil)
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
	e.fixture(uid, store.StatusSuspended, "spam", enfNow.Add(-time.Minute))
	e.start()
	e.wrap.liftErr = errors.New("db down")
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if err == nil || errors.Is(err, identity.ErrAccountLocked) || errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("want an unavailable-class error, got %v", err)
	}
	if e.wrap.liftHits == 0 {
		t.Fatal("lift was never attempted")
	}
	if _, total := e.sessionCounts(uid); total != 0 {
		t.Fatal("session created")
	}
	if e.stored(uid).Status != store.StatusSuspended {
		t.Fatal("status changed")
	}
}

func TestLoginLiftNotAudited(t *testing.T) { // Login enforcement: Lift writes no audit event
	e := newEnfEnv(t)
	uid := e.user("u1", "buyer")
	e.fixture(uid, store.StatusSuspended, "spam", enfNow.Add(-time.Second))
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
	e.fixture(uid, store.StatusBanned, "scam", enfNow.Add(-24*time.Hour))
	e.start()
	_, err := e.svc.Login(e.ctx, "u1", "password123")
	if !errors.Is(err, identity.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if e.stored(uid).Status != store.StatusBanned {
		t.Fatal("banned user was lifted")
	}
}
