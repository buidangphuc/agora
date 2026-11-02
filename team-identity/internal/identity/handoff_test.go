package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
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
