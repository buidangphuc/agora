package rpc_test

// G3 (identity-session-refresh tasks 5): AuthHandler Login/Register field mapping and
// the RefreshSession RPC.
//
// API assumed: (*AuthHandler).RefreshSession(ctx, *identityv1.RefreshSessionRequest)
// (*identityv1.RefreshSessionResponse, error); AuthResult fields 4/5 are
// refresh_token and refresh_expires_at.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

var rt0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

type rClock struct{ t time.Time }

func (c *rClock) Now() time.Time          { return c.t }
func (c *rClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

type rUsers struct {
	*storetest.InMemoryUserRepository
	deleted bool
}

func (u *rUsers) GetByID(ctx context.Context, id string) (store.User, error) {
	if u.deleted {
		return store.User{}, store.ErrNotFound
	}
	return u.InMemoryUserRepository.GetByID(ctx, id)
}

type rSessions struct {
	*storetest.InMemorySessionRepository
	rotateErr error
	calls     int
}

func (s *rSessions) RotateRefresh(ctx context.Context, p, n string, now time.Time, g, m time.Duration, ip, device string, ev store.EnqueueFn) (store.RotateResult, error) {
	s.calls++
	if s.rotateErr != nil {
		return store.RotateResult{}, s.rotateErr
	}
	return s.InMemorySessionRepository.RotateRefresh(ctx, p, n, now, g, m, ip, device, ev)
}

type rEnv struct {
	h     *rpc.AuthHandler
	users *rUsers
	sess  *rSessions
	clock *rClock
}

func newREnv(t *testing.T) *rEnv {
	t.Helper()
	e := &rEnv{
		users: &rUsers{InMemoryUserRepository: storetest.NewInMemoryUserRepository()},
		sess:  &rSessions{InMemorySessionRepository: storetest.NewInMemorySessionRepository()},
		clock: &rClock{t: rt0},
	}
	svc := newSvc(t, e.users, e.sess).WithClock(e.clock.Now)
	e.h = rpc.NewAuthHandler(svc)
	return e
}

func (e *rEnv) register(ctx context.Context, t *testing.T) *identityv1.AuthResult {
	t.Helper()
	r, err := e.h.Register(ctx, &identityv1.RegisterRequest{Username: "alice", Password: "password123", Role: "buyer"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return r.GetResult()
}

func (e *rEnv) refresh(ctx context.Context, tok string) (*identityv1.RefreshSessionResponse, error) {
	return e.h.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: tok})
}

func hexSHA(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// Scenarios: Login returns a refresh token, Register returns a refresh token.
func TestAuthHandlerMapsRefreshFields_LoginAndRegisterReturnRefreshToken(t *testing.T) {
	ctx := t.Context()
	e := newREnv(t)
	reg := e.register(ctx, t)
	login, err := e.h.Login(context.Background(), &identityv1.LoginRequest{Username: "alice", Password: "password123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	want := rt0.Add(store.RefreshTTL)
	for name, r := range map[string]*identityv1.AuthResult{"register": reg, "login": login.GetResult()} {
		if r.GetRefreshToken() == "" {
			t.Errorf("%s: refresh_token not mapped", name)
		}
		if r.GetRefreshExpiresAt() == nil || !r.GetRefreshExpiresAt().AsTime().Equal(want) {
			t.Errorf("%s: refresh_expires_at = %v, want %v", name, r.GetRefreshExpiresAt(), want)
		}
	}
	if reg.GetRefreshToken() == login.GetResult().GetRefreshToken() {
		t.Error("register and login must mint distinct refresh tokens")
	}
}

// Scenario: Successful rotation, through the handler (also: callable without a Principal).
func TestRefreshSessionHandlerRotates_CallableWithoutPrincipal(t *testing.T) {
	ctx := t.Context()
	e := newREnv(t)
	reg := e.register(ctx, t)
	e.clock.Advance(time.Minute)
	res, err := e.refresh(ctx, reg.GetRefreshToken()) // context.Background(): no Principal
	if err != nil {
		t.Fatalf("refresh without principal: %v", err)
	}
	r := res.GetResult()
	if r.GetToken() == "" || r.GetRefreshToken() == "" || r.GetRefreshToken() == reg.GetRefreshToken() {
		t.Fatalf("want new access and refresh token, got %+v", r)
	}
	if r.GetPrincipal().GetId() != reg.GetPrincipal().GetId() {
		t.Fatalf("principal = %v, want %v", r.GetPrincipal(), reg.GetPrincipal())
	}
	if want := e.clock.Now().Add(store.RefreshTTL); !r.GetRefreshExpiresAt().AsTime().Equal(want) {
		t.Fatalf("refresh_expires_at = %v, want %v", r.GetRefreshExpiresAt().AsTime(), want)
	}
}

// Scenario: Grace replay returns an access token only (handler maps empty refresh token).
func TestRefreshSessionHandlerGraceReplay_AccessTokenOnly(t *testing.T) {
	ctx := t.Context()
	e := newREnv(t)
	reg := e.register(ctx, t)
	if _, err := e.refresh(ctx, reg.GetRefreshToken()); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(5 * time.Second)
	res, err := e.refresh(ctx, reg.GetRefreshToken())
	if err != nil {
		t.Fatalf("grace replay: %v", err)
	}
	if res.GetResult().GetToken() == "" || res.GetResult().GetRefreshToken() != "" {
		t.Fatalf("want access token only, got %+v", res.GetResult())
	}
}

// Scenarios: Missing or oversized token is Unauthenticated; Maximum-length token is accepted for lookup.
func TestRefreshSessionRejectsBadInput_MissingOrOversizedIsUnauthenticated(t *testing.T) {
	ctx := t.Context()
	e := newREnv(t)
	reg := e.register(ctx, t)
	_, unknownErr := e.refresh(ctx, "does-not-exist")
	if status.Code(unknownErr) != codes.Unauthenticated {
		t.Fatalf("unknown: %v", unknownErr)
	}
	before, _ := e.sess.ListSessions(context.Background(), reg.GetPrincipal().GetId())
	callsBefore := e.sess.calls

	for name, tok := range map[string]string{"empty": "", "257 bytes": strings.Repeat("a", 257), "huge": strings.Repeat("a", 1<<16)} {
		_, err := e.refresh(ctx, tok)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: code = %v, want Unauthenticated (never InvalidArgument)", name, status.Code(err))
			continue
		}
		if status.Convert(err).Message() != status.Convert(unknownErr).Message() {
			t.Errorf("%s: message %q differs from unknown-token message %q", name, status.Convert(err).Message(), status.Convert(unknownErr).Message())
		}
	}
	if e.sess.calls != callsBefore {
		t.Errorf("bad input must be rejected before any repository lookup (calls %d -> %d)", callsBefore, e.sess.calls)
	}
	after, _ := e.sess.ListSessions(context.Background(), reg.GetPrincipal().GetId())
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Errorf("session rows changed: %v -> %v", before, after)
	}
}

func TestRefreshSessionMaxLengthBoundary_256BytesGoesThroughLookup(t *testing.T) {
	ctx := t.Context()
	e := newREnv(t)
	e.register(ctx, t)
	_, err := e.refresh(ctx, strings.Repeat("a", 256))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
	if e.sess.calls != 1 {
		t.Fatalf("256-byte token must reach the lookup exactly once, calls = %d", e.sess.calls)
	}
}

// Scenario: Failure messages do not leak the cause.
func TestRefreshErrorsAreIndistinguishable_SixCausesOneStatus(t *testing.T) {
	ctx := context.Background()
	collect := func() map[string]error {
		out := map[string]error{}
		e := newREnv(t)
		reg := e.register(ctx, t)
		out["unknown"] = func() error { _, err := e.refresh(ctx, "nope"); return err }()
		out["empty"] = func() error { _, err := e.refresh(ctx, ""); return err }()
		out["257 bytes"] = func() error { _, err := e.refresh(ctx, strings.Repeat("a", 257)); return err }()

		// expired: past the sliding expiry
		e.clock.Advance(store.RefreshTTL + time.Second)
		out["expired"] = func() error { _, err := e.refresh(ctx, reg.GetRefreshToken()); return err }()

		// revoked
		e2 := newREnv(t)
		reg2 := e2.register(ctx, t)
		sessions, _ := e2.sess.ListSessions(ctx, reg2.GetPrincipal().GetId())
		if err := e2.sess.RevokeSession(ctx, sessions[0].ID, reg2.GetPrincipal().GetId(), nil); err != nil {
			t.Fatal(err)
		}
		out["revoked"] = func() error { _, err := e2.refresh(ctx, reg2.GetRefreshToken()); return err }()

		// deleted user
		e3 := newREnv(t)
		reg3 := e3.register(ctx, t)
		e3.users.deleted = true
		out["deleted user"] = func() error { _, err := e3.refresh(ctx, reg3.GetRefreshToken()); return err }()
		return out
	}
	errs := collect()
	if len(errs) != 6 {
		t.Fatalf("want 6 causes, got %d", len(errs))
	}
	var wantMsg string
	for name, err := range errs {
		st, _ := status.FromError(err)
		if st.Code() != codes.Unauthenticated {
			t.Errorf("%s: code = %v, want Unauthenticated", name, st.Code())
			continue
		}
		if wantMsg == "" {
			wantMsg = st.Message()
		}
		if st.Message() != wantMsg || wantMsg == "" {
			t.Errorf("%s: message %q, want identical non-empty %q", name, st.Message(), wantMsg)
		}
	}
}

// Scenario: Identity database down is not a logout (handler mapping: Unavailable).
func TestRefreshSessionHandlerMapsDBDownToUnavailable(t *testing.T) {
	ctx := t.Context()
	e := newREnv(t)
	reg := e.register(ctx, t)
	e.sess.rotateErr = errors.New("pq: connection refused host=10.0.0.5")
	_, err := e.refresh(ctx, reg.GetRefreshToken())
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable (err=%v)", st.Code(), err)
	}
	if strings.Contains(st.Message(), "pq:") || strings.Contains(st.Message(), "10.0.0.5") {
		t.Errorf("driver text leaked: %q", st.Message())
	}
	e.sess.rotateErr = nil
	if _, err := e.refresh(ctx, reg.GetRefreshToken()); err != nil {
		t.Fatalf("token must still work after recovery: %v", err)
	}
}

// Scenario: atomic revoke/outbox failure -> Internal (task 5), generic message.
func TestRefreshSessionHandlerMapsRotateFaultToInternal(t *testing.T) {
	ctx := t.Context()
	e := newREnv(t)
	reg := e.register(ctx, t)
	e.sess.rotateErr = fmt.Errorf("enqueue revoke event: pq: deadlock detected: %w", store.ErrRotateFault)
	_, err := e.refresh(ctx, reg.GetRefreshToken())
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Fatalf("code = %v, want Internal (err=%v)", st.Code(), err)
	}
	if strings.Contains(st.Message(), "pq:") || strings.Contains(st.Message(), "deadlock") {
		t.Errorf("driver text leaked: %q", st.Message())
	}
}

// capture is a slog handler recording every formatted line (message plus all attributes).
type capture struct {
	mu    sync.Mutex
	lines []string
}

func (c *capture) Enabled(context.Context, slog.Level) bool { return true }
func (c *capture) Handle(_ context.Context, r slog.Record) error {
	var b bytes.Buffer
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool { fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any()); return true })
	c.mu.Lock()
	c.lines = append(c.lines, b.String())
	c.mu.Unlock()
	return nil
}
func (c *capture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *capture) WithGroup(string) slog.Handler      { return c }

// Scenario: Token and hash never reach logs, spans or outbox.
// Covers Login, rotation, grace replay, reuse revocation and error paths; both the
// handler logger and slog.Default (used by the service) are captured. Span capture
// needs a tracer fixture the repo lacks; attributes are covered by the same code paths
// and are asserted by the implementer's review, not here.
func TestRefreshSecretsNeverLogged(t *testing.T) {
	lc := &capture{}
	prev := slog.Default()
	slog.SetDefault(slog.New(lc))
	t.Cleanup(func() { slog.SetDefault(prev) })

	e := newREnv(t)
	e.h.WithLogger(slog.New(lc))
	ctx := context.Background()

	secrets := []string{}
	track := func(tok string) {
		if tok != "" {
			secrets = append(secrets, tok, hexSHA(tok))
		}
	}
	reg := e.register(ctx, t)
	track(reg.GetRefreshToken())
	login, err := e.h.Login(ctx, &identityv1.LoginRequest{Username: "alice", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	track(login.GetResult().GetRefreshToken())

	r1 := reg.GetRefreshToken()
	rot, err := e.refresh(ctx, r1) // rotation
	if err != nil {
		t.Fatal(err)
	}
	track(rot.GetResult().GetRefreshToken())
	e.clock.Advance(3 * time.Second)
	if _, err := e.refresh(ctx, r1); err != nil { // grace replay
		t.Fatal(err)
	}
	e.clock.Advance(20 * time.Second)
	if _, err := e.refresh(ctx, r1); status.Code(err) != codes.Unauthenticated { // reuse -> revoke
		t.Fatalf("reuse: %v", err)
	}
	// error paths whose underlying error text may embed values
	badTok := "bad-token-" + strings.Repeat("z", 20)
	track(badTok)
	_, _ = e.refresh(ctx, badTok)
	e.sess.rotateErr = errors.New("pq: failure")
	_, _ = e.refresh(ctx, login.GetResult().GetRefreshToken())
	e.sess.rotateErr = fmt.Errorf("boom: %w", store.ErrRotateFault)
	_, _ = e.refresh(ctx, login.GetResult().GetRefreshToken())

	for _, line := range lc.lines {
		for _, s := range secrets {
			if strings.Contains(line, s) {
				t.Errorf("log line leaks secret %q...: %s", s[:8], line)
			}
		}
	}
	for _, row := range e.sess.OutboxRows() {
		for _, s := range secrets {
			if bytes.Contains(row.Payload, []byte(s)) || strings.Contains(row.EventID+row.AggregateID+row.RequestID, s) {
				t.Errorf("outbox row leaks secret %q...", s[:8])
			}
		}
	}
	if len(e.sess.OutboxRows()) == 0 {
		t.Fatal("precondition: the reuse revocation must have written an outbox row")
	}
}

// Scenarios: Session client context: rotation records IP via repository; refresh needs no
// principal and grants nothing (permission). The ClientInfo in ctx is the only caller input.
// Regression guards: behaviour delivered by 1.1/1.2, so green at base by design.
func TestSessionClientContext_HandlerRefreshNoPrincipalRecordsClient_Permission(t *testing.T) {
	e := newREnv(t)
	reg, err := e.h.Register(gwCtx("203.0.113.1", "AgoraTest/1"),
		&identityv1.RegisterRequest{Username: "alice", Password: "password123", Role: "buyer"})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Minute)
	ctx := gwCtx("198.51.100.4", "AgoraTest/2") // no Principal
	res, err := e.h.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: reg.GetResult().GetRefreshToken()})
	if err != nil {
		t.Fatalf("refresh without principal: %v", err)
	}
	if fmt.Sprint(res.GetResult().GetPrincipal().GetScopes()) != fmt.Sprint(reg.GetResult().GetPrincipal().GetScopes()) || res.GetResult().GetPrincipal().GetId() != reg.GetResult().GetPrincipal().GetId() {
		t.Fatalf("refresh must grant nothing beyond the session owner's principal, got %v", res.GetResult().GetPrincipal())
	}
	items, err := e.sess.ListSessions(context.Background(), reg.GetResult().GetPrincipal().GetId())
	if err != nil || len(items) != 1 {
		t.Fatalf("want 1 session, got %d err=%v", len(items), err)
	}
	if items[0].IP != "198.51.100.4" || items[0].Device != "AgoraTest/2" {
		t.Fatalf("want ip/device from ctx, got ip=%q device=%q", items[0].IP, items[0].Device)
	}
}

// Scenario: Session client context: rejected refresh updates nothing (error), via the handler.
func TestSessionClientContext_HandlerRejectedRefreshUpdatesNothing_Error(t *testing.T) {
	e := newREnv(t)
	reg, err := e.h.Register(gwCtx("203.0.113.1", "AgoraTest/1"),
		&identityv1.RegisterRequest{Username: "alice", Password: "password123", Role: "buyer"})
	if err != nil {
		t.Fatal(err)
	}
	uid := reg.GetResult().GetPrincipal().GetId()
	ctx := gwCtx("198.51.100.4", "AgoraTest/2")
	_, err = e.h.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: "not-a-real-token"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
	items, _ := e.sess.ListSessions(context.Background(), uid)
	if len(items) != 1 || items[0].IP != "203.0.113.1" || items[0].Device != "AgoraTest/1" {
		t.Fatalf("rejected refresh must leave ip/device untouched, got %+v", items)
	}
}

// gwCtx simulates the gateway-forwarded client context (no Principal).
func gwCtx(ip, ua string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-client-ip", ip, "x-client-user-agent", ua))
}
