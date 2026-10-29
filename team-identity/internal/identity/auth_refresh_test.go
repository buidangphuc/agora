package identity_test

// G2 (identity-session-refresh task 3): refresh-token minting in issue() and
// AuthService.RefreshSession.
//
// Service API assumed by these tests (the implementer must provide it):
//   - (*AuthService).WithClock(func() time.Time) *AuthService   // injected clock; default time.Now
//   - AuthResult.RefreshToken string; AuthResult.RefreshExpiresAt time.Time
//   - (*AuthService).RefreshSession(ctx, refreshToken string) (AuthResult, error)
//   - var ErrInvalidRefreshToken  // the single generic failure (handler -> Unauthenticated)
//   - var ErrRefreshUnavailable   // DB error (handler -> Unavailable), session not revoked
//   - issue() stamps Session{CreatedAt: clock(), RefreshHash: hex(sha256(token)), RefreshExpiresAt: clock()+RefreshTTL}
//   - RefreshSession loads the user through UserRepository.GetByID (ErrNotFound = deleted).
//   - grace = 10s, maxAge = 90 days passed to SessionRepository.RotateRefresh.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/authz"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

var t0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

// fakeUsers wraps the in-memory repo to simulate deletion, role changes and DB errors.
type fakeUsers struct {
	*storetest.InMemoryUserRepository
	deleted bool
	roles   []string
	getErr  error
}

func (f *fakeUsers) GetByID(ctx context.Context, id string) (store.User, error) {
	if f.getErr != nil {
		return store.User{}, f.getErr
	}
	if f.deleted {
		return store.User{}, store.ErrNotFound
	}
	u, err := f.InMemoryUserRepository.GetByID(ctx, id)
	if err == nil && f.roles != nil {
		u.Roles = f.roles
	}
	return u, err
}

// spySessions records RotateRefresh arguments and can inject a DB error.
type spySessions struct {
	*storetest.InMemorySessionRepository
	rotateErr error
	calls     int
	grace     time.Duration
	maxAge    time.Duration
	lastNew   string
}

func (s *spySessions) RotateRefresh(ctx context.Context, presented, newHash string, now time.Time, grace, maxAge time.Duration, ip, device string, ev store.EnqueueFn) (store.RotateResult, error) {
	s.calls++
	s.grace, s.maxAge, s.lastNew = grace, maxAge, newHash
	if s.rotateErr != nil {
		return store.RotateResult{}, s.rotateErr
	}
	return s.InMemorySessionRepository.RotateRefresh(ctx, presented, newHash, now, grace, maxAge, ip, device, ev)
}

type refreshEnv struct {
	svc   *identity.AuthService
	users *fakeUsers
	sess  *spySessions
	clock *fakeClock
	uid   string
	first identity.AuthResult // result of Register
}

func newRefreshEnv(t *testing.T) *refreshEnv {
	t.Helper()
	e := &refreshEnv{
		users: &fakeUsers{InMemoryUserRepository: storetest.NewInMemoryUserRepository()},
		sess:  &spySessions{InMemorySessionRepository: storetest.NewInMemorySessionRepository()},
		clock: &fakeClock{t: t0},
	}
	e.svc = newSvc(t, e.users, e.sess).WithClock(e.clock.Now)
	res, err := e.svc.Register(context.Background(), "alice", "password123", "buyer")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	e.uid, e.first = res.UserID, res
	return e
}

func sha(tok string) string { h := sha256.Sum256([]byte(tok)); return hex.EncodeToString(h[:]) }

func jwtClaims(t *testing.T, tok string) (sid string, scopes []string) {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", tok)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var c struct {
		Sid    string   `json:"sid"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	return c.Sid, c.Scopes
}

func (e *refreshEnv) sessionRow(t *testing.T, uid string) store.Session {
	t.Helper()
	items, err := e.sess.ListSessions(context.Background(), uid)
	if err != nil || len(items) != 1 {
		t.Fatalf("want exactly 1 session for %s, got %d err=%v", uid, len(items), err)
	}
	return items[0]
}

func (e *refreshEnv) refresh(tok string) (identity.AuthResult, error) {
	return e.svc.RefreshSession(context.Background(), tok)
}

func (e *refreshEnv) mustRefresh(t *testing.T, tok string) identity.AuthResult {
	t.Helper()
	res, err := e.refresh(tok)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return res
}

func (e *refreshEnv) wantInvalid(t *testing.T, tok string) {
	t.Helper()
	res, err := e.refresh(tok)
	if !errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("want ErrInvalidRefreshToken, got %v", err)
	}
	if res.Token != "" || res.RefreshToken != "" {
		t.Fatalf("failure must issue no tokens: %+v", res)
	}
}

func (e *refreshEnv) wantRevokedOnce(t *testing.T) {
	t.Helper()
	s := e.sessionRow(t, e.uid)
	if !s.Revoked || s.RefreshHash != "" || s.PrevRefreshHash != "" {
		t.Fatalf("want revoked with both hashes cleared, got %+v", s)
	}
	if n := len(e.sess.OutboxRows()); n != 1 {
		t.Fatalf("want exactly 1 SessionRevoked outbox row, got %d", n)
	}
}

func (e *refreshEnv) wantNotRevoked(t *testing.T) {
	t.Helper()
	if s := e.sessionRow(t, e.uid); s.Revoked {
		t.Fatalf("session must not be revoked: %+v", s)
	}
	if n := len(e.sess.OutboxRows()); n != 0 {
		t.Fatalf("want no outbox rows, got %d", n)
	}
}

// seed inserts a session with a known token, bypassing the service (for old created_at etc).
func (e *refreshEnv) seed(t *testing.T, uid, tok string, created, expires time.Time) store.Session {
	t.Helper()
	s, err := e.sess.CreateSession(context.Background(), store.Session{
		UserID: uid, CreatedAt: created, RefreshHash: sha(tok), RefreshExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return s
}

func assertRefreshTokenShape(t *testing.T, tok string) {
	t.Helper()
	if len(tok) != 43 {
		t.Fatalf("refresh token must be 43 chars, got %d", len(tok))
	}
	b, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(b) != 32 {
		t.Fatalf("refresh token must be base64url of 32 bytes: len=%d err=%v", len(b), err)
	}
}

func near(got, want time.Time) bool {
	d := got.Sub(want)
	return d > -5*time.Second && d < 5*time.Second
}

// --- Login / Register ---

func TestLoginIssuesRefreshToken(t *testing.T) { // scenario: Login returns a refresh token
	e := newRefreshEnv(t)
	res, err := e.svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	assertRefreshTokenShape(t, res.RefreshToken)
	if !near(res.RefreshExpiresAt, e.clock.Now().Add(30*24*time.Hour)) {
		t.Fatalf("refresh_expires_at = %v, want now+30d", res.RefreshExpiresAt)
	}
	sid, _ := jwtClaims(t, res.Token)
	found := false
	items, _ := e.sess.ListSessions(context.Background(), e.uid)
	for _, s := range items {
		if s.ID == sid {
			found = true
			if s.RefreshHash != sha(res.RefreshToken) {
				t.Fatalf("stored hash must be hex sha256 of the token")
			}
		}
	}
	if !found {
		t.Fatalf("login session %s not found", sid)
	}
}

func TestRegisterIssuesRefreshToken(t *testing.T) { // scenario: Register returns a refresh token
	e := newRefreshEnv(t)
	assertRefreshTokenShape(t, e.first.RefreshToken)
	s := e.sessionRow(t, e.uid)
	if s.RefreshHash == "" || s.RefreshHash == e.first.RefreshToken || s.RefreshHash != sha(e.first.RefreshToken) {
		t.Fatalf("session must hold sha256 of the token, not the token: %q", s.RefreshHash)
	}
	if strings.Contains(fmt.Sprintf("%+v", s), e.first.RefreshToken) {
		t.Fatalf("raw token persisted on the session row")
	}
}

func TestFailedLoginCreatesNoSession(t *testing.T) { // scenario: Failed login issues nothing
	e := newRefreshEnv(t)
	if _, err := e.svc.Login(context.Background(), "alice", "wrong-password"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	items, _ := e.sess.ListSessions(context.Background(), e.uid)
	if len(items) != 1 { // only the Register session
		t.Fatalf("failed login must create no session, have %d", len(items))
	}
}

// --- Rotation ---

func TestRefreshRotatesAndKeepsSid(t *testing.T) { // scenario: Successful rotation
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	origSid, _ := jwtClaims(t, e.first.Token)
	e.clock.Advance(time.Hour)
	res := e.mustRefresh(t, r1)
	sid, _ := jwtClaims(t, res.Token)
	if sid != origSid {
		t.Fatalf("sid changed: %s -> %s", origSid, sid)
	}
	assertRefreshTokenShape(t, res.RefreshToken)
	if res.RefreshToken == r1 {
		t.Fatalf("refresh token must change")
	}
	if !res.RefreshExpiresAt.Equal(e.clock.Now().Add(30 * 24 * time.Hour)) {
		t.Fatalf("refresh_expires_at = %v", res.RefreshExpiresAt)
	}
	if res.UserID != e.uid || res.Username != "alice" || res.Type != "user" {
		t.Fatalf("identity wrong: %+v", res)
	}
	s := e.sessionRow(t, e.uid)
	if s.RefreshHash != sha(res.RefreshToken) || s.PrevRefreshHash != sha(r1) || !s.RotatedAt.Equal(e.clock.Now()) {
		t.Fatalf("row not rotated correctly: %+v", s)
	}
}

func TestRefreshExtendsExpiry(t *testing.T) { // scenario: Expiry slides on use
	e := newRefreshEnv(t)
	tok := "seeded-token-expiry-slides"
	sd := e.seed(t, e.uid, tok, e.clock.Now().Add(-time.Hour), e.clock.Now().Add(24*time.Hour))
	res := e.mustRefresh(t, tok)
	if !res.RefreshExpiresAt.Equal(e.clock.Now().Add(30 * 24 * time.Hour)) {
		t.Fatalf("expiry did not slide: %v", res.RefreshExpiresAt)
	}
	for _, s := range mustList(t, e) {
		if s.ID == sd.ID && !s.RefreshExpiresAt.Equal(res.RefreshExpiresAt) {
			t.Fatalf("persisted expiry %v != %v", s.RefreshExpiresAt, res.RefreshExpiresAt)
		}
	}
}

func mustList(t *testing.T, e *refreshEnv) []store.Session {
	t.Helper()
	items, err := e.sess.ListSessions(context.Background(), e.uid)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestRefreshChain(t *testing.T) { // scenario: The rotated token is the only live one
	e := newRefreshEnv(t)
	r2 := e.mustRefresh(t, e.first.RefreshToken).RefreshToken
	e.clock.Advance(11 * time.Second)
	r3 := e.mustRefresh(t, r2).RefreshToken
	assertRefreshTokenShape(t, r3)
	if r3 == r2 || r3 == e.first.RefreshToken {
		t.Fatalf("each rotation must mint a fresh token")
	}
	e.wantNotRevoked(t)
}

func TestRefreshRederivesScopes(t *testing.T) { // scenario: Scopes follow current roles
	e := newRefreshEnv(t)
	e.users.roles = []string{"seller"}
	want := authz.ScopesForRoles([]string{"seller"})
	if fmt.Sprint(want) == fmt.Sprint(e.first.Scopes) {
		t.Fatalf("test premise: seller and buyer scopes must differ")
	}
	res := e.mustRefresh(t, e.first.RefreshToken)
	_, claimScopes := jwtClaims(t, res.Token)
	if fmt.Sprint(res.Scopes) != fmt.Sprint(want) || fmt.Sprint(claimScopes) != fmt.Sprint(want) {
		t.Fatalf("scopes = %v / claims %v, want %v", res.Scopes, claimScopes, want)
	}
}

func TestRefreshBindsToOwningSession(t *testing.T) { // scenario: A token resolves only to its owning session
	e := newRefreshEnv(t)
	b, err := e.svc.Register(context.Background(), "bob", "password123", "seller")
	if err != nil {
		t.Fatal(err)
	}
	before := e.sessionRow(t, e.uid)
	bSid, _ := jwtClaims(t, b.Token)
	res := e.mustRefresh(t, b.RefreshToken)
	sid, _ := jwtClaims(t, res.Token)
	if res.UserID != b.UserID || sid != bSid {
		t.Fatalf("must resolve to user B / S2: %+v sid=%s", res, sid)
	}
	if after := e.sessionRow(t, e.uid); fmt.Sprintf("%+v", after) != fmt.Sprintf("%+v", before) {
		t.Fatalf("user A session changed:\nbefore %+v\nafter  %+v", before, after)
	}
}

func TestRefreshEmptyTokenRejectedWithoutLookup(t *testing.T) { // scenario: Missing or oversized token is Unauthenticated (service defence)
	e := newRefreshEnv(t)
	e.wantInvalid(t, "")
	if e.sess.calls != 0 {
		t.Fatalf("empty token must not reach the repository")
	}
}

// --- Failures ---

func TestRefreshUnknownToken(t *testing.T) { // scenario: Unknown token
	e := newRefreshEnv(t)
	e.wantInvalid(t, strings.Repeat("A", 43))
	e.wantNotRevoked(t)
}

func TestRefreshExpiredToken(t *testing.T) { // scenario: Expired token
	e := newRefreshEnv(t)
	e.clock.Advance(30*24*time.Hour + time.Second) // past the sliding expiry, within 90d
	e.wantInvalid(t, e.first.RefreshToken)
	e.wantNotRevoked(t)
	if e.sessionRow(t, e.uid).RefreshHash != sha(e.first.RefreshToken) {
		t.Fatalf("expired token must leave the row unchanged")
	}
}

func TestRefreshRevokedSession(t *testing.T) { // scenario: Revoked session
	e := newRefreshEnv(t)
	sid, _ := jwtClaims(t, e.first.Token)
	revokeAsLogout(t, e, sid)
	e.wantInvalid(t, e.first.RefreshToken)
	if n := len(e.sess.OutboxRows()); n != 1 {
		t.Fatalf("no further event expected, outbox rows = %d", n)
	}
}

func revokeAsLogout(t *testing.T, e *refreshEnv, sid string) {
	t.Helper()
	ev := func(s store.Session) (store.OutboxRow, error) {
		return store.OutboxRow{EventID: "logout-" + s.ID, AggregateType: "Session", AggregateID: s.UserID,
			EventType: "platform.identity.v1.SessionRevoked", Payload: []byte("x")}, nil
	}
	if err := e.sess.RevokeSession(context.Background(), sid, e.uid, ev); err != nil {
		t.Fatalf("logout revoke: %v", err)
	}
}

func TestRefreshDeletedUserRevokes(t *testing.T) { // scenario: Owning user deleted
	e := newRefreshEnv(t)
	e.users.deleted = true
	e.wantInvalid(t, e.first.RefreshToken)
	e.wantRevokedOnce(t)
}

func TestRefreshGraceDeletedUserRevokes(t *testing.T) { // scenario: Owning user deleted, grace replay
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	e.mustRefresh(t, r1)
	e.users.deleted = true
	e.clock.Advance(5 * time.Second)
	e.wantInvalid(t, r1)
	e.wantRevokedOnce(t)
}

func TestRefreshGraceRederivesScopes(t *testing.T) { // scenario: Grace replay re-derives scopes
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	e.mustRefresh(t, r1)
	e.users.roles = []string{"seller"}
	e.clock.Advance(3 * time.Second)
	res := e.mustRefresh(t, r1)
	_, claimScopes := jwtClaims(t, res.Token)
	want := authz.ScopesForRoles([]string{"seller"})
	if fmt.Sprint(claimScopes) != fmt.Sprint(want) || fmt.Sprint(res.Scopes) != fmt.Sprint(want) {
		t.Fatalf("grace scopes = %v / %v, want %v", res.Scopes, claimScopes, want)
	}
}

func TestRefreshAbsoluteLifetimeRevokes(t *testing.T) { // scenario: Absolute lifetime exceeded
	e := newRefreshEnv(t)
	tok := "seeded-token-old-session"
	old := e.clock.Now().Add(-(90*24*time.Hour + time.Second))
	// Drop the Register session so the seeded one is the only row.
	e.sess = &spySessions{InMemorySessionRepository: storetest.NewInMemorySessionRepository()}
	e.svc = newSvc(t, e.users, e.sess).WithClock(e.clock.Now)
	e.seed(t, e.uid, tok, old, e.clock.Now().Add(24*time.Hour))
	e.wantInvalid(t, tok)
	e.wantRevokedOnce(t)
}

func TestRefreshAbsoluteLifetimeBoundary(t *testing.T) { // scenario: Absolute lifetime boundary
	e := newRefreshEnv(t)
	tok := "seeded-token-boundary"
	e.sess = &spySessions{InMemorySessionRepository: storetest.NewInMemorySessionRepository()}
	e.svc = newSvc(t, e.users, e.sess).WithClock(e.clock.Now)
	e.seed(t, e.uid, tok, e.clock.Now().Add(-90*24*time.Hour), e.clock.Now().Add(24*time.Hour))
	res := e.mustRefresh(t, tok)
	assertRefreshTokenShape(t, res.RefreshToken)
	e.wantNotRevoked(t)
}

func TestRefreshPassesPositiveNinetyDayMaxAge(t *testing.T) { // reviewer carry-over: maxAge positive (90d), grace 10s
	e := newRefreshEnv(t)
	e.mustRefresh(t, e.first.RefreshToken)
	if e.sess.maxAge != 90*24*time.Hour || e.sess.maxAge <= 0 {
		t.Fatalf("maxAge = %v, want 90d", e.sess.maxAge)
	}
	if e.sess.grace != 10*time.Second {
		t.Fatalf("grace = %v, want 10s", e.sess.grace)
	}
	if e.sess.lastNew != sha(e.sessionRow(t, e.uid).RefreshHash) && e.sess.lastNew != e.sessionRow(t, e.uid).RefreshHash {
		t.Fatalf("service must pass the sha256 hex of the new token as newHash")
	}
}

// --- Reuse / grace ---

func TestRefreshReuseRevokesSession(t *testing.T) { // scenario: Reuse after the grace window revokes
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	r2 := e.mustRefresh(t, r1).RefreshToken
	e.clock.Advance(11 * time.Second)
	e.wantInvalid(t, r1)
	e.wantInvalid(t, r2)
	e.wantRevokedOnce(t)
}

func TestRefreshGraceBoundary(t *testing.T) { // scenario: Grace window boundary
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	origSid, _ := jwtClaims(t, e.first.Token)
	e.mustRefresh(t, r1)
	e.clock.Advance(10 * time.Second)
	res := e.mustRefresh(t, r1)
	if sid, _ := jwtClaims(t, res.Token); sid != origSid || res.RefreshToken != "" {
		t.Fatalf("exactly 10s: want same sid and empty refresh token, got sid=%s rt=%q", sid, res.RefreshToken)
	}
	e.clock.Advance(time.Millisecond)
	e.wantInvalid(t, r1)
	e.wantRevokedOnce(t)
}

func TestRefreshGraceReturnsAccessOnly(t *testing.T) { // scenario: Grace replay returns an access token only
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	origSid, _ := jwtClaims(t, e.first.Token)
	r2res := e.mustRefresh(t, r1)
	before := e.sessionRow(t, e.uid)
	e.clock.Advance(4 * time.Second)
	res := e.mustRefresh(t, r1)
	if sid, _ := jwtClaims(t, res.Token); sid != origSid {
		t.Fatalf("sid = %s, want %s", sid, origSid)
	}
	if res.RefreshToken != "" || !res.RefreshExpiresAt.IsZero() {
		t.Fatalf("grace replay must mint no refresh token: %+v", res)
	}
	after := e.sessionRow(t, e.uid)
	if after.RefreshHash != sha(r2res.RefreshToken) || !after.RefreshExpiresAt.Equal(before.RefreshExpiresAt) {
		t.Fatalf("row must be unchanged by a grace replay: %+v", after)
	}
	e.wantNotRevoked(t)
}

func TestRefreshStaleTokenNoRevoke(t *testing.T) { // scenario: Token older than one rotation is rejected without revoke
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	r2 := e.mustRefresh(t, r1).RefreshToken
	e.clock.Advance(time.Second)
	r3 := e.mustRefresh(t, r2).RefreshToken
	e.wantInvalid(t, r1)
	e.wantNotRevoked(t)
	e.clock.Advance(time.Minute)
	if res := e.mustRefresh(t, r3); res.RefreshToken == "" {
		t.Fatalf("R3 must still be accepted")
	}
}

func TestRefreshPrevTokenAfterLogoutUnauthenticated(t *testing.T) { // scenario: Prev-token replay within grace after logout
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	sid, _ := jwtClaims(t, e.first.Token)
	e.mustRefresh(t, r1)
	revokeAsLogout(t, e, sid)
	e.clock.Advance(5 * time.Second)
	e.wantInvalid(t, r1)
	if n := len(e.sess.OutboxRows()); n != 1 {
		t.Fatalf("no further outbox row expected, got %d", n)
	}
}

func TestRefreshLostResponseRetryIsReuse(t *testing.T) { // scenario: Lost response retried after the grace window counts as reuse
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	e.mustRefresh(t, r1) // response lost
	e.clock.Advance(11 * time.Second)
	e.wantInvalid(t, r1)
	e.wantRevokedOnce(t)
}

func TestRefreshRevokedAfterDenylistExpiry(t *testing.T) { // scenario: Revocation outlives the denylist
	e := newRefreshEnv(t)
	sid, _ := jwtClaims(t, e.first.Token)
	last := e.mustRefresh(t, e.first.RefreshToken).RefreshToken
	revokeAsLogout(t, e, sid)
	e.clock.Advance(2 * time.Hour) // beyond the 1h access TTL
	e.wantInvalid(t, last)
}

// --- Errors ---

func TestRefreshDBDownIsUnavailableNotRevoked(t *testing.T) { // scenario: Identity database down is not a logout
	e := newRefreshEnv(t)
	e.sess.rotateErr = errors.New("connection refused")
	res, err := e.refresh(e.first.RefreshToken)
	if !errors.Is(err, identity.ErrRefreshUnavailable) || errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("want ErrRefreshUnavailable, got %v", err)
	}
	if res.Token != "" || res.RefreshToken != "" {
		t.Fatalf("no tokens on failure: %+v", res)
	}
	e.sess.rotateErr = nil
	e.wantNotRevoked(t)
	if got := e.mustRefresh(t, e.first.RefreshToken); got.RefreshToken == "" {
		t.Fatalf("same token must work after recovery")
	}
}

func TestRefreshUserLookupDBErrorIsUnavailableNotRevoked(t *testing.T) { // scenario: Identity database down is not a logout (user read)
	e := newRefreshEnv(t)
	e.users.getErr = errors.New("connection refused")
	_, err := e.refresh(e.first.RefreshToken)
	if !errors.Is(err, identity.ErrRefreshUnavailable) {
		t.Fatalf("want ErrRefreshUnavailable, got %v", err)
	}
	if s := e.sessionRow(t, e.uid); s.Revoked {
		t.Fatalf("user-lookup DB error must not revoke")
	}
	if n := len(e.sess.OutboxRows()); n != 0 {
		t.Fatalf("no outbox rows expected, got %d", n)
	}
}

func TestRefreshErrorsAreGenericAndLeakNothing(t *testing.T) { // scenario: Failure messages do not leak the cause (service level)
	var msgs []string
	collect := func(err error) {
		if !errors.Is(err, identity.ErrInvalidRefreshToken) {
			t.Fatalf("want ErrInvalidRefreshToken, got %v", err)
		}
		msgs = append(msgs, err.Error())
	}
	// unknown
	e := newRefreshEnv(t)
	_, err := e.refresh(strings.Repeat("B", 43))
	collect(err)
	// expired
	e = newRefreshEnv(t)
	e.clock.Advance(31 * 24 * time.Hour)
	_, err = e.refresh(e.first.RefreshToken)
	collect(err)
	// reuse
	e = newRefreshEnv(t)
	e.mustRefresh(t, e.first.RefreshToken)
	e.clock.Advance(time.Minute)
	_, err = e.refresh(e.first.RefreshToken)
	collect(err)
	// deleted user
	e = newRefreshEnv(t)
	e.users.deleted = true
	_, err = e.refresh(e.first.RefreshToken)
	collect(err)
	// empty
	e = newRefreshEnv(t)
	_, err = e.refresh("")
	collect(err)
	for _, m := range msgs[1:] {
		if m != msgs[0] {
			t.Fatalf("failure messages differ: %q vs %q", m, msgs[0])
		}
	}
	if strings.Contains(strings.ToLower(msgs[0]), "conflict") {
		t.Fatalf("message leaks cause: %q", msgs[0])
	}
}

// conflictSessions makes RotateRefresh fail with a hash-conflict-like error.
type conflictSessions struct{ *spySessions }

func (c conflictSessions) RotateRefresh(context.Context, string, string, time.Time, time.Duration, time.Duration, string, string, store.EnqueueFn) (store.RotateResult, error) {
	return store.RotateResult{}, errors.New("refresh hash conflict")
}

func TestRefreshHashConflictNotSurfacedVerbatim(t *testing.T) { // reviewer carry-over: errRefreshHashConflict not surfaced
	e := newRefreshEnv(t)
	svc := newSvc(t, e.users, conflictSessions{e.sess}).WithClock(e.clock.Now)
	_, err := svc.RefreshSession(context.Background(), e.first.RefreshToken)
	if err == nil {
		t.Fatalf("want an error")
	}
	if strings.Contains(err.Error(), "refresh hash conflict") {
		t.Fatalf("raw repository error leaked: %v", err)
	}
	if errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("a repository fault must not look like an invalid token (no logout on infra errors)")
	}
}

func TestRefreshOutboxFailureIsInternal(t *testing.T) { // scenario: Revoke and event are atomic (outbox failure -> Internal)
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	e.mustRefresh(t, r1)
	e.clock.Advance(time.Minute)
	e.sess.FailOutbox = true
	res, err := e.refresh(r1)
	if !errors.Is(err, identity.ErrRefreshInternal) {
		t.Fatalf("want ErrRefreshInternal, got %v", err)
	}
	if errors.Is(err, identity.ErrRefreshUnavailable) || errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("outbox failure must be neither Unavailable nor Invalid: %v", err)
	}
	if res.Token != "" || res.RefreshToken != "" {
		t.Fatalf("no tokens on failure: %+v", res)
	}
	if s := e.sessionRow(t, e.uid); s.Revoked {
		t.Fatalf("session must stay unrevoked when the outbox write fails")
	}
}

// faultSessions makes RotateRefresh fail with the given error.
type faultSessions struct {
	*spySessions
	err error
}

func (c faultSessions) RotateRefresh(context.Context, string, string, time.Time, time.Duration, time.Duration, string, string, store.EnqueueFn) (store.RotateResult, error) {
	return store.RotateResult{}, c.err
}

func TestRefreshWrappedRotateFaultIsInternal(t *testing.T) { // scenario: Revoke and event are atomic (hash conflict / ev fault -> Internal)
	e := newRefreshEnv(t)
	fault := fmt.Errorf("refresh hash conflict: %w", store.ErrRotateFault)
	svc := newSvc(t, e.users, faultSessions{e.sess, fault}).WithClock(e.clock.Now)
	_, err := svc.RefreshSession(context.Background(), e.first.RefreshToken)
	if !errors.Is(err, identity.ErrRefreshInternal) {
		t.Fatalf("want ErrRefreshInternal, got %v", err)
	}
	if errors.Is(err, identity.ErrRefreshUnavailable) || errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("rotate fault must be neither Unavailable nor Invalid: %v", err)
	}
	if strings.Contains(err.Error(), "refresh hash conflict") {
		t.Fatalf("raw repository error leaked: %v", err)
	}
}

func TestRefreshPlainRotateErrorIsUnavailableNotInternal(t *testing.T) { // scenario: Identity database down is not a logout
	e := newRefreshEnv(t)
	e.sess.rotateErr = errors.New("connection refused")
	_, err := e.refresh(e.first.RefreshToken)
	if !errors.Is(err, identity.ErrRefreshUnavailable) || errors.Is(err, identity.ErrRefreshInternal) {
		t.Fatalf("want Unavailable and not Internal, got %v", err)
	}
}

// revokeFailSessions injects an error into RevokeSession (deleted-user path).
type revokeFailSessions struct {
	*spySessions
	err error
}

func (c revokeFailSessions) RevokeSession(context.Context, string, string, store.EnqueueFn) error {
	return c.err
}

func TestRefreshDeletedUserRevokeOutboxFailureIsInternal(t *testing.T) { // scenario: Revoke and event are atomic (deleted-user revoke)
	e := newRefreshEnv(t)
	e.users.deleted = true
	e.sess.FailOutbox = true
	_, err := e.refresh(e.first.RefreshToken)
	if !errors.Is(err, identity.ErrRefreshInternal) {
		t.Fatalf("want ErrRefreshInternal, got %v", err)
	}
	if errors.Is(err, identity.ErrRefreshUnavailable) || errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("must be neither Unavailable nor Invalid: %v", err)
	}
	if s := e.sessionRow(t, e.uid); s.Revoked {
		t.Fatalf("session must stay unrevoked when the outbox write fails")
	}
}

func TestRefreshDeletedUserRevokeDBErrorIsUnavailable(t *testing.T) { // scenario: Identity database down is not a logout (deleted-user revoke)
	e := newRefreshEnv(t)
	e.users.deleted = true
	svc := newSvc(t, e.users, revokeFailSessions{e.sess, errors.New("connection refused")}).WithClock(e.clock.Now)
	_, err := svc.RefreshSession(context.Background(), e.first.RefreshToken)
	if !errors.Is(err, identity.ErrRefreshUnavailable) || errors.Is(err, identity.ErrRefreshInternal) || errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("want Unavailable only, got %v", err)
	}
}

func TestRefreshDeletedUserRevokeFaultIsInternal(t *testing.T) { // scenario: Revoke and event are atomic (RevokeSession wraps ErrRotateFault)
	e := newRefreshEnv(t)
	e.users.deleted = true
	fault := fmt.Errorf("enqueue: %w", store.ErrRotateFault)
	svc := newSvc(t, e.users, revokeFailSessions{e.sess, fault}).WithClock(e.clock.Now)
	_, err := svc.RefreshSession(context.Background(), e.first.RefreshToken)
	if !errors.Is(err, identity.ErrRefreshInternal) {
		t.Fatalf("want ErrRefreshInternal, got %v", err)
	}
}

func TestRefreshResultNeverExposesHashes(t *testing.T) { // reviewer carry-over: Session hashes never in returned results
	e := newRefreshEnv(t)
	r1 := e.first.RefreshToken
	var all []string
	res2 := e.mustRefresh(t, r1)
	all = append(all, fmt.Sprintf("%+v", e.first), fmt.Sprintf("%+v", res2))
	e.clock.Advance(2 * time.Second)
	res3 := e.mustRefresh(t, r1) // grace
	all = append(all, fmt.Sprintf("%+v", res3))
	hashes := []string{sha(r1), sha(res2.RefreshToken)}
	for _, h := range hashes {
		for _, out := range all {
			if strings.Contains(out, h) {
				t.Fatalf("hash %s leaked in result %s", h, out)
			}
		}
	}
	// The raw token appears only in RefreshToken of the rotation result.
	if strings.Contains(fmt.Sprintf("%+v", res3), r1) || strings.Contains(fmt.Sprintf("%+v", res3), res2.RefreshToken) {
		t.Fatalf("grace result must not carry any refresh token")
	}
}
