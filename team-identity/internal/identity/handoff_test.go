package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/authz"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

const (
	hoUser   = "u-mint-1"
	hoSID    = "sid-mint-1"
	hoSeller = "HANDOFF_APP_SELLER"
	hoCap    = 10
)

var hoNow = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

type hoRepo struct {
	mu   sync.Mutex
	rows []store.HandoffCode
}

func (r *hoRepo) live(userID string, now time.Time) int {
	n := 0
	for _, c := range r.rows {
		if c.UserID == userID && c.RedeemedAt.IsZero() && !c.ExpiresAt.Before(now) {
			n++
		}
	}
	return n
}

func (r *hoRepo) InsertHandoffCode(ctx context.Context, c store.HandoffCode, maxLive int, now time.Time, inTx func(context.Context) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live(c.UserID, now) >= maxLive {
		return store.ErrHandoffCapReached
	}
	r.rows = append(r.rows, c)
	if inTx != nil {
		if err := inTx(ctx); err != nil {
			r.rows = r.rows[:len(r.rows)-1]
			return err
		}
	}
	return nil
}

func (r *hoRepo) RedeemHandoffCode(ctx context.Context, h []byte, app string, now time.Time, inTx func(context.Context, store.HandoffCode) error) (store.HandoffCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.rows {
		if bytes.Equal(c.CodeHash, h) && c.TargetApp == app && c.RedeemedAt.IsZero() && !c.ExpiresAt.Before(now) {
			r.rows[i].RedeemedAt = now
			c.RedeemedAt = now
			if inTx != nil {
				if err := inTx(ctx, c); err != nil {
					r.rows[i].RedeemedAt = time.Time{}
					return store.HandoffCode{}, err
				}
			}
			return c, nil
		}
	}
	return store.HandoffCode{}, store.ErrHandoffInvalid
}

func (r *hoRepo) PeekHandoffCode(_ context.Context, h []byte, app string, now time.Time) (store.HandoffCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.rows {
		if bytes.Equal(c.CodeHash, h) && c.TargetApp == app && c.RedeemedAt.IsZero() && !c.ExpiresAt.Before(now) {
			return c, nil
		}
	}
	return store.HandoffCode{}, store.ErrHandoffInvalid
}

func (r *hoRepo) CountLiveHandoffCodes(_ context.Context, userID string, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live(userID, now), nil
}

func (r *hoRepo) PurgeHandoffCodes(context.Context, time.Time, int) (int, error) { return 0, nil }

func (r *hoRepo) seed(userID string, n int, expiresAt, redeemedAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < n; i++ {
		h := sha256.Sum256([]byte(userID + expiresAt.String() + redeemedAt.String() + string(rune('a'+i))))
		r.rows = append(r.rows, store.HandoffCode{
			CodeHash: h[:], UserID: userID, SourceSID: hoSID, TargetApp: hoSeller,
			ExpiresAt: expiresAt, RedeemedAt: redeemedAt, CreatedAt: hoNow.Add(-time.Hour),
		})
	}
}

func (r *hoRepo) snapshot() []store.HandoffCode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]store.HandoffCode(nil), r.rows...)
}

type hoAudit struct {
	mu     sync.Mutex
	events []auditx.Event
	err    error
}

func (a *hoAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	a.events = append(a.events, ev)
	return nil
}

func (a *hoAudit) mints() []auditx.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []auditx.Event
	for _, e := range a.events {
		if e.Action == "auth.handoff_mint" {
			out = append(out, e)
		}
	}
	return out
}

type hoEnv struct {
	svc   *identity.AuthService
	repo  *hoRepo
	audit *hoAudit
}

func newHoEnv(t *testing.T, status string) hoEnv {
	t.Helper()
	users := storetest.NewInMemoryUserRepository()
	if _, err := users.Create(context.Background(), store.User{ID: hoUser, Username: "minter", Roles: []string{"buyer"}, Status: status}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	repo := &hoRepo{}
	audit := &hoAudit{}
	svc := newSvcAudit(t, users, storetest.NewInMemorySessionRepository(), audit).
		WithClock(func() time.Time { return hoNow }).
		WithHandoff(repo)
	return hoEnv{svc: svc, repo: repo, audit: audit}
}

func userCtx() context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: hoUser, Type: auth.TypeUser})
}

func hoCode(err error) codes.Code {
	switch {
	case err == nil:
		return codes.OK
	case errors.Is(err, identity.ErrAccountLocked):
		return codes.PermissionDenied
	case errors.Is(err, identity.ErrInvalidInput):
		return codes.InvalidArgument
	case errors.Is(err, store.ErrHandoffCapReached):
		return codes.ResourceExhausted
	}
	return status.Code(err)
}

func hoWantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := hoCode(err); got != want {
		t.Fatalf("code = %v (err %v), want %v", got, err, want)
	}
}

func (e hoEnv) mint(ctx context.Context) (identity.MintResult, error) {
	return e.svc.Mint(ctx, hoSID, identityv1.HandoffApp_HANDOFF_APP_SELLER)
}

func (e hoEnv) mustMint(t *testing.T) identity.MintResult {
	t.Helper()
	res, err := e.mint(userCtx())
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	return res
}

func (e hoEnv) wantUntouched(t *testing.T) {
	t.Helper()
	if n := len(e.repo.snapshot()); n != 0 {
		t.Errorf("rows = %d, want 0", n)
	}
	if n := len(e.audit.mints()); n != 0 {
		t.Errorf("audit mint entries = %d, want 0", n)
	}
}

// Scenario: Authenticated user mints a code for the seller app
// Scenario: Default TTL without override
func TestMintReturnsCodeAndTTL(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	res := e.mustMint(t)
	if len(res.Code) != 43 {
		t.Fatalf("code length = %d, want 43", len(res.Code))
	}
	raw, err := base64.RawURLEncoding.DecodeString(res.Code)
	if err != nil || len(raw) != 32 {
		t.Fatalf("code must be 32 bytes of unpadded base64url: len=%d err=%v", len(raw), err)
	}
	if want := hoNow.Add(60 * time.Second); !res.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", res.ExpiresAt, want)
	}
	rows := e.repo.snapshot()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	sum := sha256.Sum256([]byte(res.Code))
	row := rows[0]
	if !bytes.Equal(row.CodeHash, sum[:]) {
		t.Errorf("row key is not the SHA-256 of the code")
	}
	if row.UserID != hoUser || row.SourceSID != hoSID || row.TargetApp != hoSeller {
		t.Errorf("row = user %q sid %q target %q", row.UserID, row.SourceSID, row.TargetApp)
	}
	if !row.ExpiresAt.Equal(res.ExpiresAt) {
		t.Errorf("row ExpiresAt = %v, want %v", row.ExpiresAt, res.ExpiresAt)
	}
	for _, f := range []string{row.UserID, row.SourceSID, row.TargetApp, string(row.CodeHash)} {
		if strings.Contains(f, res.Code) {
			t.Errorf("a stored field contains the plaintext code")
		}
	}
}

// Scenario: Override accepted in test
func TestMintHonoursConfiguredTTL(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	e.svc.WithHandoffTTL(5 * time.Second)
	res := e.mustMint(t)
	if want := hoNow.Add(5 * time.Second); !res.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", res.ExpiresAt, want)
	}
}

// Scenario: Authenticated user mints a code for the seller app
func TestMintCodesAreRandomAndDistinct(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	seen := map[string]bool{}
	for i := 0; i < hoCap; i++ {
		c := e.mustMint(t).Code
		if seen[c] {
			t.Fatalf("duplicate code %q", c)
		}
		seen[c] = true
	}
}

// Scenario: Mint writes its audit entry and the row commits together
func TestMintAuditMetadataHasNoSecret(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	res := e.mustMint(t)
	evs := e.audit.mints()
	if len(evs) != 1 {
		t.Fatalf("auth.handoff_mint entries = %d, want 1", len(evs))
	}
	ev := evs[0]
	if ev.ActorID != hoUser {
		t.Errorf("actor = %q, want the minting user", ev.ActorID)
	}
	sum := sha256.Sum256([]byte(res.Code))
	secrets := []string{res.Code, hex.EncodeToString(sum[:]), base64.RawURLEncoding.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sum[:]), hoSID}
	fields := []string{ev.ActorID, ev.Action, ev.TargetType, ev.TargetID}
	hasTarget := false
	for _, v := range ev.Metadata {
		fields = append(fields, v)
		if strings.Contains(strings.ToUpper(v), "SELLER") {
			hasTarget = true
		}
	}
	for k := range ev.Metadata {
		fields = append(fields, k)
	}
	if !hasTarget {
		t.Errorf("metadata %v does not carry the target app", ev.Metadata)
	}
	for _, f := range fields {
		for _, s := range secrets {
			if strings.Contains(f, s) {
				t.Errorf("audit field %q leaks a secret", f)
			}
		}
	}
}

// Scenario: Unspecified target is refused
func TestMintUnspecifiedTarget(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	_, err := e.svc.Mint(userCtx(), hoSID, identityv1.HandoffApp_HANDOFF_APP_UNSPECIFIED)
	hoWantCode(t, err, codes.InvalidArgument)
	e.wantUntouched(t)
}

// Scenario: Service principal cannot mint
func TestMintServicePrincipalDenied(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "svc-x", Type: auth.TypeService})
	_, err := e.mint(ctx)
	hoWantCode(t, err, codes.PermissionDenied)
	e.wantUntouched(t)
}

// Scenario: Anonymous caller cannot mint
func TestMintAnonymousUnauthenticated(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	anon := auth.WithPrincipal(context.Background(), auth.Principal{ID: "anonymous", Type: auth.TypeAnonymous})
	for name, ctx := range map[string]context.Context{"no principal": context.Background(), "anonymous principal": anon} {
		_, err := e.mint(ctx)
		if got := hoCode(err); got != codes.Unauthenticated {
			t.Errorf("%s: code = %v (err %v), want Unauthenticated", name, got, err)
		}
	}
	e.wantUntouched(t)
}

// Scenario: Non-ACTIVE account cannot mint
func TestMintSuspendedDenied(t *testing.T) {
	for _, st := range []string{store.StatusSuspended, store.StatusBanned} {
		e := newHoEnv(t, st)
		_, err := e.mint(userCtx())
		if got := hoCode(err); got != codes.PermissionDenied {
			t.Errorf("status %s: code = %v (err %v), want PermissionDenied", st, got, err)
		}
		e.wantUntouched(t)
	}
}

// Scenario: Audit down blocks the mint
func TestMintAuditDownUnavailable(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	e.audit.err = errors.New("audit unreachable")
	_, err := e.mint(userCtx())
	hoWantCode(t, err, codes.Unavailable)
	if n := len(e.repo.snapshot()); n != 0 {
		t.Fatalf("rows after failed audit = %d, want 0", n)
	}
	e.audit.err = nil
	e.mustMint(t)
}

// Scenario: Eleventh live code is refused
func TestMintCapExceeded(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	e.repo.seed(hoUser, hoCap, hoNow.Add(30*time.Second), time.Time{})
	_, err := e.mint(userCtx())
	hoWantCode(t, err, codes.ResourceExhausted)
	if n := len(e.repo.snapshot()); n != hoCap {
		t.Fatalf("rows = %d, want %d", n, hoCap)
	}
}

// Scenario: Eleventh live code is refused
func TestMintCapRefusalWritesNoAudit(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	for i := 0; i < hoCap; i++ {
		e.mustMint(t)
	}
	if _, err := e.mint(userCtx()); hoCode(err) != codes.ResourceExhausted {
		t.Fatalf("eleventh mint: %v, want ResourceExhausted", err)
	}
	if n := len(e.audit.mints()); n != hoCap {
		t.Fatalf("audit mint entries = %d, want %d (refusal must write none)", n, hoCap)
	}
}

// Scenario: Concurrent mints cannot exceed the cap
func TestMintAuditCountUnderConcurrency(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	const calls = 20
	errs := make([]error, calls)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = e.mint(userCtx())
		}()
	}
	close(start)
	wg.Wait()
	ok, exhausted := 0, 0
	for _, err := range errs {
		switch hoCode(err) {
		case codes.OK:
			ok++
		case codes.ResourceExhausted:
			exhausted++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != hoCap || exhausted != calls-hoCap {
		t.Fatalf("ok=%d exhausted=%d, want %d and %d", ok, exhausted, hoCap, calls-hoCap)
	}
	if n := len(e.repo.snapshot()); n != hoCap {
		t.Errorf("rows = %d, want %d", n, hoCap)
	}
	if n := len(e.audit.mints()); n != hoCap {
		t.Errorf("audit mint entries = %d, want %d", n, hoCap)
	}
}

// Scenario: Expired and redeemed codes do not count toward the cap
func TestMintCapIgnoresExpiredAndRedeemed(t *testing.T) {
	e := newHoEnv(t, store.StatusActive)
	e.repo.seed(hoUser, 9, hoNow.Add(30*time.Second), time.Time{})
	e.repo.seed(hoUser, 5, hoNow.Add(-time.Second), time.Time{})
	e.repo.seed(hoUser, 5, hoNow.Add(30*time.Second), hoNow.Add(-5*time.Second))
	e.mustMint(t)
	n, err := e.repo.CountLiveHandoffCodes(context.Background(), hoUser, hoNow)
	if err != nil || n != hoCap {
		t.Fatalf("live codes = %d (err %v), want %d", n, err, hoCap)
	}
}

type rdUsers struct {
	*storetest.InMemoryUserRepository
	mu     sync.Mutex
	status string
	roles  []string
	gets   atomic.Int64
}

func (u *rdUsers) GetByID(ctx context.Context, id string) (store.User, error) {
	u.gets.Add(1)
	usr, err := u.InMemoryUserRepository.GetByID(ctx, id)
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.status != "" {
		usr.Status = u.status
	}
	if u.roles != nil {
		usr.Roles = u.roles
	}
	return usr, err
}

func (u *rdUsers) set(status string, roles []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status, u.roles = status, roles
}

type rdRepo struct {
	*hoRepo
	calls atomic.Int64
}

func (r *rdRepo) RedeemHandoffCode(ctx context.Context, h []byte, app string, now time.Time, inTx func(context.Context, store.HandoffCode) error) (store.HandoffCode, error) {
	r.calls.Add(1)
	return r.hoRepo.RedeemHandoffCode(ctx, h, app, now, inTx)
}

func (r *rdRepo) PeekHandoffCode(ctx context.Context, h []byte, app string, now time.Time) (store.HandoffCode, error) {
	r.calls.Add(1)
	return r.hoRepo.PeekHandoffCode(ctx, h, app, now)
}

type rdEnv struct {
	svc      *identity.AuthService
	repo     *rdRepo
	audit    *hoAudit
	users    *rdUsers
	sessions *storetest.InMemorySessionRepository
	clock    *fakeClock
	srcSID   string
}

func newRdEnv(t *testing.T) rdEnv {
	t.Helper()
	base := storetest.NewInMemoryUserRepository()
	if _, err := base.Create(context.Background(), store.User{ID: hoUser, Username: "minter", Roles: []string{"buyer"}, Status: store.StatusActive}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	users := &rdUsers{InMemoryUserRepository: base}
	sessions := storetest.NewInMemorySessionRepository()
	src, err := sessions.CreateSession(context.Background(), store.Session{UserID: hoUser, CreatedAt: hoNow, RefreshHash: "src-refresh-hash", RefreshExpiresAt: hoNow.Add(store.RefreshTTL)})
	if err != nil {
		t.Fatalf("seed source session: %v", err)
	}
	repo := &rdRepo{hoRepo: &hoRepo{}}
	audit := &hoAudit{}
	clock := &fakeClock{t: hoNow}
	svc := newSvcAudit(t, users, sessions, audit).WithClock(clock.Now).WithHandoff(repo)
	return rdEnv{svc: svc, repo: repo, audit: audit, users: users, sessions: sessions, clock: clock, srcSID: src.ID}
}

func (e rdEnv) mintFor(t *testing.T, target identityv1.HandoffApp) string {
	t.Helper()
	res, err := e.svc.Mint(userCtx(), e.srcSID, target)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	return res.Code
}

func (e rdEnv) mintSeller(t *testing.T) string {
	t.Helper()
	return e.mintFor(t, identityv1.HandoffApp_HANDOFF_APP_SELLER)
}

func (e rdEnv) redeem(code string) (identity.AuthResult, error) {
	return e.svc.Redeem(context.Background(), code, identityv1.HandoffApp_HANDOFF_APP_SELLER)
}

func (e rdEnv) peek(code string) (identity.PeekResult, error) {
	return e.svc.Peek(context.Background(), code, identityv1.HandoffApp_HANDOFF_APP_SELLER)
}

func (e rdEnv) redeems() []auditx.Event {
	e.audit.mu.Lock()
	defer e.audit.mu.Unlock()
	var out []auditx.Event
	for _, ev := range e.audit.events {
		if ev.Action == "auth.handoff_redeem" {
			out = append(out, ev)
		}
	}
	return out
}

func (e rdEnv) auditCount() int {
	e.audit.mu.Lock()
	defer e.audit.mu.Unlock()
	return len(e.audit.events)
}

func (e rdEnv) sessionCount(t *testing.T) int {
	t.Helper()
	items, err := e.sessions.ListSessions(context.Background(), hoUser)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	return len(items)
}

func wantFixedUnauthenticated(t *testing.T, label string, err error) string {
	t.Helper()
	st, ok := status.FromError(err)
	if err == nil || !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("%s: err = %v, want gRPC Unauthenticated", label, err)
	}
	if st.Message() == "" {
		t.Fatalf("%s: empty message", label)
	}
	return st.Message()
}

func unknownCode() string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i*7 + 3)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Scenario: Valid code opens a new session for the target app
func TestRedeemOpensNewSession(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	e.clock.Advance(10 * time.Second)
	res, err := e.redeem(code)
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	if res.Token == "" || res.RefreshToken == "" {
		t.Fatalf("missing tokens: access %t refresh %t", res.Token != "", res.RefreshToken != "")
	}
	if res.UserID != hoUser || res.Username != "minter" {
		t.Errorf("identity = %q/%q", res.UserID, res.Username)
	}
	sid, _ := jwtClaims(t, res.Token)
	if sid == "" || sid == e.srcSID {
		t.Fatalf("new sid = %q, source sid = %q; want a different non-empty sid", sid, e.srcSID)
	}
	if n := e.sessionCount(t); n != 2 {
		t.Errorf("sessions = %d, want 2", n)
	}
	items, _ := e.sessions.ListSessions(context.Background(), hoUser)
	for _, s := range items {
		if s.ID == e.srcSID && s.Revoked {
			t.Errorf("source session was revoked by redeem")
		}
	}
	if _, err := e.svc.RefreshSession(context.Background(), res.RefreshToken); err != nil {
		t.Errorf("new session refresh: %v", err)
	}
	if rows := e.repo.snapshot(); len(rows) != 1 || rows[0].RedeemedAt.IsZero() {
		t.Errorf("code row not marked redeemed: %+v", rows)
	}
}

// Scenario: Redeem at the TTL boundary is covered by the store; here the domain accepts exactly expires_at
func TestRedeemAtExpiryInstant(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	e.clock.Advance(60 * time.Second)
	if _, err := e.redeem(code); err != nil {
		t.Fatalf("Redeem at expires_at: %v", err)
	}
}

// Scenario: Reused code is refused
func TestRedeemReuseRefused(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	if _, err := e.redeem(code); err != nil {
		t.Fatalf("first Redeem: %v", err)
	}
	_, err := e.redeem(code)
	wantFixedUnauthenticated(t, "reuse", err)
	if n := e.sessionCount(t); n != 2 {
		t.Errorf("sessions = %d, want 2 (source + one derived)", n)
	}
}

// Scenario: Expired code is refused
func TestRedeemExpired(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	e.clock.Advance(61 * time.Second)
	_, err := e.redeem(code)
	wantFixedUnauthenticated(t, "expired", err)
	if n := e.sessionCount(t); n != 1 {
		t.Errorf("sessions = %d, want 1", n)
	}
}

// Scenario: Wrong target is refused
func TestRedeemWrongTarget(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintFor(t, identityv1.HandoffApp_HANDOFF_APP_BUYER)
	_, err := e.redeem(code)
	wantFixedUnauthenticated(t, "wrong target", err)
	if rows := e.repo.snapshot(); len(rows) != 1 || !rows[0].RedeemedAt.IsZero() {
		t.Errorf("code must stay unredeemed: %+v", rows)
	}
	if _, err := e.svc.Redeem(context.Background(), code, identityv1.HandoffApp_HANDOFF_APP_BUYER); err != nil {
		t.Errorf("redeem for the true target: %v", err)
	}
}

// Scenario: Revoked source session is refused
func TestRedeemRevokedSource(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	if err := e.sessions.RevokeSession(context.Background(), e.srcSID, hoUser, nil); err != nil {
		t.Fatalf("revoke source: %v", err)
	}
	_, err := e.redeem(code)
	wantFixedUnauthenticated(t, "revoked source", err)
	if n := e.sessionCount(t); n != 1 {
		t.Errorf("sessions = %d, want 1", n)
	}
	if e.auditCount() != 1 {
		t.Errorf("audit entries = %d, want only the mint entry", e.auditCount())
	}
}

// Scenario: Account suspended after minting
func TestRedeemSuspendedAccount(t *testing.T) {
	for _, st := range []string{store.StatusSuspended, store.StatusBanned, store.StatusRestricted} {
		e := newRdEnv(t)
		code := e.mintSeller(t)
		e.users.set(st, nil)
		_, err := e.redeem(code)
		wantFixedUnauthenticated(t, st, err)
		if n := e.sessionCount(t); n != 1 {
			t.Errorf("%s: sessions = %d, want 1", st, n)
		}
		if len(e.redeems()) != 0 {
			t.Errorf("%s: redeem audit entry written", st)
		}
	}
}

// Scenario: Every failure is indistinguishable
func TestRedeemFailuresIndistinguishable(t *testing.T) {
	e := newRdEnv(t)
	used := e.mintSeller(t)
	if _, err := e.redeem(used); err != nil {
		t.Fatalf("first Redeem: %v", err)
	}
	expired := e.mintSeller(t)
	wrongTarget := e.mintFor(t, identityv1.HandoffApp_HANDOFF_APP_BUYER)
	revoked := e.mintSeller(t)
	if err := e.sessions.RevokeSession(context.Background(), e.srcSID, hoUser, nil); err != nil {
		t.Fatalf("revoke source: %v", err)
	}
	e.clock.Advance(61 * time.Second)
	fresh := e.mintSeller(t)
	_ = fresh
	cases := map[string]string{
		"unknown":       unknownCode(),
		"used":          used,
		"expired":       expired,
		"wrong target":  wrongTarget,
		"malformed":     "short",
		"revoked":       revoked,
		"empty":         "",
		"too long":      unknownCode() + "A",
		"fresh-revoked": fresh,
	}
	var want string
	names := make([]string, 0, len(cases))
	for n := range cases {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		_, err := e.redeem(cases[n])
		msg := wantFixedUnauthenticated(t, n, err)
		if want == "" {
			want = msg
		}
		if msg != want {
			t.Errorf("%s: message %q differs from %q", n, msg, want)
		}
	}
}

// Scenario: Audit down blocks the redeem and keeps the code usable
func TestRedeemAuditDownKeepsCodeUsable(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	e.audit.mu.Lock()
	e.audit.err = errors.New("audit unreachable")
	e.audit.mu.Unlock()
	_, err := e.redeem(code)
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("code = %v (err %v), want Unavailable", got, err)
	}
	if rows := e.repo.snapshot(); len(rows) != 1 || !rows[0].RedeemedAt.IsZero() {
		t.Fatalf("code must stay redeemable after audit failure: %+v", rows)
	}
	e.audit.mu.Lock()
	e.audit.err = nil
	e.audit.mu.Unlock()
	e.clock.Advance(30 * time.Second)
	if _, err := e.redeem(code); err != nil {
		t.Fatalf("redeem after audit recovered: %v", err)
	}
}

// Scenario: Redeem audit entry
func TestRedeemAuditEntry(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	res, err := e.redeem(code)
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	evs := e.redeems()
	if len(evs) != 1 {
		t.Fatalf("auth.handoff_redeem entries = %d, want 1", len(evs))
	}
	ev := evs[0]
	if ev.ActorID != hoUser {
		t.Errorf("actor = %q, want the user id stored with the code", ev.ActorID)
	}
	newSID, _ := jwtClaims(t, res.Token)
	sum := sha256.Sum256([]byte(code))
	secrets := []string{code, hex.EncodeToString(sum[:]), base64.RawURLEncoding.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sum[:]), e.srcSID, newSID}
	fields := []string{ev.ActorID, ev.Action, ev.TargetType, ev.TargetID}
	hasTarget, hasOK := false, false
	for k, v := range ev.Metadata {
		fields = append(fields, k, v)
		if strings.Contains(strings.ToUpper(v), "SELLER") {
			hasTarget = true
		}
		if v == "ok" {
			hasOK = true
		}
	}
	if !hasTarget || !hasOK {
		t.Errorf("metadata %v must carry the target app and result ok", ev.Metadata)
	}
	for _, f := range fields {
		for _, sec := range secrets {
			if strings.Contains(f, sec) {
				t.Errorf("audit field %q leaks a secret", f)
			}
		}
	}
}

// Scenario: Derived session carries the roles at redeem time
func TestRedeemRolesAtRedeemTime(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	e.users.set("", []string{"buyer", "seller"})
	res, err := e.redeem(code)
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	_, scopes := jwtClaims(t, res.Token)
	want := authz.ScopesForUser([]string{"buyer", "seller"}, store.StatusActive)
	if len(want) <= len(authz.ScopesForUser([]string{"buyer"}, store.StatusActive)) {
		t.Fatalf("fixture: seller role adds no scope")
	}
	sort.Strings(scopes)
	sort.Strings(want)
	if strings.Join(scopes, ",") != strings.Join(want, ",") {
		t.Errorf("token scopes = %v, want %v", scopes, want)
	}
	got := append([]string(nil), res.Scopes...)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("result scopes = %v, want %v", got, want)
	}
}

// Scenario: Derived session survives source logout
func TestRedeemSessionIndependentOfSource(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	res, err := e.redeem(code)
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	if err := e.sessions.RevokeSession(context.Background(), e.srcSID, hoUser, nil); err != nil {
		t.Fatalf("logout source: %v", err)
	}
	if _, err := e.svc.RefreshSession(context.Background(), res.RefreshToken); err != nil {
		t.Fatalf("derived session refresh after source logout: %v", err)
	}
}

// Scenario: Concurrent redeems create one session
func TestRedeemConcurrentOneAudit(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	const calls = 20
	errs := make([]error, calls)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = e.redeem(code)
		}()
	}
	close(start)
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
			continue
		}
		wantFixedUnauthenticated(t, "loser", err)
	}
	if ok != 1 {
		t.Fatalf("successes = %d, want 1", ok)
	}
	if n := e.sessionCount(t); n != 2 {
		t.Errorf("sessions = %d, want 2", n)
	}
	if n := len(e.redeems()); n != 1 {
		t.Errorf("auth.handoff_redeem entries = %d, want 1", n)
	}
}

// Scenario: Concurrent redeems create one session (losers write no audit and answer within the p95)
func TestRedeemLoserNoAuditWithinP95(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	if _, err := e.redeem(code); err != nil {
		t.Fatalf("first Redeem: %v", err)
	}
	before := e.auditCount()
	const calls = 20
	durs := make([]time.Duration, calls)
	errs := make([]error, calls)
	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			begin := time.Now()
			_, errs[i] = e.redeem(code)
			durs[i] = time.Since(begin)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		wantFixedUnauthenticated(t, "loser", err)
	}
	if after := e.auditCount(); after != before {
		t.Errorf("losers wrote %d audit entries", after-before)
	}
	sort.Slice(durs, func(a, b int) bool { return durs[a] < durs[b] })
	if p95 := durs[calls*95/100]; p95 > 150*time.Millisecond {
		t.Errorf("loser p95 = %v, want <= 150ms", p95)
	}
}

// Scenario: Peek returns the username and does not consume
func TestPeekReturnsUsername(t *testing.T) {
	e := newRdEnv(t)
	code := e.mintSeller(t)
	auditBefore := e.auditCount()
	for i := 0; i < 3; i++ {
		res, err := e.peek(code)
		if err != nil {
			t.Fatalf("Peek %d: %v", i, err)
		}
		if res.Username != "minter" {
			t.Errorf("username = %q, want minter", res.Username)
		}
		if want := hoNow.Add(60 * time.Second); !res.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", res.ExpiresAt, want)
		}
	}
	if rows := e.repo.snapshot(); len(rows) != 1 || !rows[0].RedeemedAt.IsZero() {
		t.Errorf("peek consumed the code: %+v", rows)
	}
	if e.auditCount() != auditBefore {
		t.Errorf("peek wrote audit entries")
	}
	if n := e.sessionCount(t); n != 1 {
		t.Errorf("peek opened a session: %d", n)
	}
	if _, err := e.redeem(code); err != nil {
		t.Fatalf("redeem after peeks: %v", err)
	}
}

// Scenario: Peek of an invalid code is indistinguishable
func TestPeekFailuresIndistinguishable(t *testing.T) {
	e := newRdEnv(t)
	used := e.mintSeller(t)
	if _, err := e.redeem(used); err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	_, redeemErr := e.redeem(used)
	want := wantFixedUnauthenticated(t, "redeem reference", redeemErr)

	expired := e.mintSeller(t)
	wrongTarget := e.mintFor(t, identityv1.HandoffApp_HANDOFF_APP_BUYER)
	e.clock.Advance(61 * time.Second)

	cases := map[string]string{
		"unknown":      unknownCode(),
		"used":         used,
		"expired":      expired,
		"wrong target": wrongTarget,
		"malformed":    "short",
	}
	for n, c := range cases {
		_, err := e.peek(c)
		if msg := wantFixedUnauthenticated(t, n, err); msg != want {
			t.Errorf("%s: message %q differs from redeem's %q", n, msg, want)
		}
	}

	revoked := newRdEnv(t)
	rc := revoked.mintSeller(t)
	if err := revoked.sessions.RevokeSession(context.Background(), revoked.srcSID, hoUser, nil); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_, err := revoked.peek(rc)
	if msg := wantFixedUnauthenticated(t, "revoked source", err); msg != want {
		t.Errorf("revoked source: message %q differs from %q", msg, want)
	}

	for _, st := range []string{store.StatusSuspended, store.StatusBanned, store.StatusRestricted} {
		ne := newRdEnv(t)
		c := ne.mintSeller(t)
		ne.users.set(st, nil)
		_, err := ne.peek(c)
		if msg := wantFixedUnauthenticated(t, "non-active "+st, err); msg != want {
			t.Errorf("non-active %s: message %q differs from %q", st, msg, want)
		}
	}
}

// Scenario: Peek of a malformed code does no lookup
func TestPeekMalformedNoLookup(t *testing.T) {
	e := newRdEnv(t)
	e.mintSeller(t)
	repoBefore, usersBefore := e.repo.calls.Load(), e.users.gets.Load()
	for _, c := range []string{"", "short", unknownCode() + "A", unknownCode()[:42]} {
		_, err := e.peek(c)
		wantFixedUnauthenticated(t, "peek "+c, err)
		_, err = e.redeem(c)
		wantFixedUnauthenticated(t, "redeem "+c, err)
	}
	if got := e.repo.calls.Load(); got != repoBefore {
		t.Errorf("handoff store queried %d times for malformed codes", got-repoBefore)
	}
	if got := e.users.gets.Load(); got != usersBefore {
		t.Errorf("user store queried %d times for malformed codes", got-usersBefore)
	}
}
