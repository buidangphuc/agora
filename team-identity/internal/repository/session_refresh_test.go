package repository

// Test expectations for the refresh-token surface of SessionRepository
// (change identity-session-refresh, task 2). The names below are chosen by the
// test author where the spec leaves them open and are the contract the
// implementer must provide:
//
//	Session.RefreshHash, Session.PrevRefreshHash string   ("" == NULL)
//	Session.RefreshExpiresAt, Session.RotatedAt time.Time (zero == NULL)
//	type RotateOutcome; RotateRotated, RotateReplayedInGrace, RotateStale, RotateReuse
//	type RotateResult struct { Outcome RotateOutcome; Session Session }
//	RotateRefresh(ctx, presentedHash, newHash string, now time.Time, grace time.Duration, ev EnqueueFn) (RotateResult, error)
//
// A hash matching neither refresh_hash nor prev_refresh_hash of a live session
// (unknown, older than one rotation, or belonging to a revoked session whose
// hashes were cleared) yields Outcome RotateStale with a nil error and no change.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var refreshT0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

const (
	refreshGrace  = 10 * time.Second
	refreshMaxAge = 90 * 24 * time.Hour
)

func seedRefreshSession(t *testing.T, r *InMemorySessionRepository, user, hash string) Session {
	t.Helper()
	s, err := r.CreateSession(context.Background(), Session{
		UserID: user, RefreshHash: hash, RefreshExpiresAt: refreshT0.Add(30 * 24 * time.Hour),
		CreatedAt: refreshT0.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s
}

func okEv(id string) EnqueueFn {
	return func(s Session) (OutboxRow, error) {
		return OutboxRow{EventID: id, AggregateType: "Session", AggregateID: s.UserID,
			EventType: "platform.identity.v1.SessionRevoked", Payload: []byte{1}}, nil
	}
}

func getSession(t *testing.T, r SessionRepository, user, id string) Session {
	t.Helper()
	items, err := r.ListSessions(context.Background(), user)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, s := range items {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("session %s not listed", id)
	return Session{}
}

func mustRotate(t *testing.T, r SessionRepository, presented, next string, now time.Time, ev EnqueueFn) RotateResult {
	t.Helper()
	res, err := r.RotateRefresh(context.Background(), presented, next, now, refreshGrace, refreshMaxAge, ev)
	if err != nil {
		t.Fatalf("RotateRefresh: %v", err)
	}
	return res
}

func TestCreateSessionStoresHash_OnlyHashIsPersisted_InMemory(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "hash-1")
	if got := getSession(t, r, "u1", s.ID); got.RefreshHash != "hash-1" || got.PrevRefreshHash != "" {
		t.Fatalf("hashes = %q/%q", got.RefreshHash, got.PrevRefreshHash)
	}
}

func TestRotateRefresh_Rotated_SlidesAndMovesHash_SuccessfulRotation(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	res := mustRotate(t, r, "h1", "h2", refreshT0, okEv("e"))
	if res.Outcome != RotateRotated {
		t.Fatalf("outcome = %v", res.Outcome)
	}
	got := getSession(t, r, "u1", s.ID)
	if got.RefreshHash != "h2" || got.PrevRefreshHash != "h1" || !got.RotatedAt.Equal(refreshT0) || !got.LastSeen.Equal(refreshT0) {
		t.Fatalf("row after rotate: %+v", got)
	}
	if res.Session.ID != s.ID || res.Session.RefreshHash != "h2" {
		t.Fatalf("result session: %+v", res.Session)
	}
	if got.Revoked || len(r.OutboxRows()) != 0 {
		t.Fatal("rotation must not revoke or write outbox")
	}
}

func TestRotateRefresh_Chain_TheRotatedTokenIsTheOnlyLiveOne(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	mustRotate(t, r, "h1", "h2", refreshT0, okEv("n"))
	res := mustRotate(t, r, "h2", "h3", refreshT0.Add(time.Minute), okEv("n"))
	if res.Outcome != RotateRotated {
		t.Fatalf("outcome = %v", res.Outcome)
	}
	got := getSession(t, r, "u1", s.ID)
	if got.RefreshHash != "h3" || got.PrevRefreshHash != "h2" {
		t.Fatalf("hashes = %q/%q", got.RefreshHash, got.PrevRefreshHash)
	}
}

func TestRotateRefresh_GraceBoundary_InclusiveAtTenSeconds(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	mustRotate(t, r, "h1", "h2", refreshT0, okEv("n"))
	before := getSession(t, r, "u1", s.ID)

	res := mustRotate(t, r, "h1", "h3", refreshT0.Add(10*time.Second), okEv("e1"))
	if res.Outcome != RotateReplayedInGrace {
		t.Fatalf("at 10s outcome = %v", res.Outcome)
	}
	after := getSession(t, r, "u1", s.ID)
	if after != before {
		t.Fatalf("grace replay must not change the row:\nbefore %+v\nafter  %+v", before, after)
	}
	if after.RefreshHash != "h2" || !after.RefreshExpiresAt.Equal(before.RefreshExpiresAt) {
		t.Fatalf("row must still hold R2: %+v", after)
	}
	if len(r.OutboxRows()) != 0 {
		t.Fatal("grace replay must not write outbox")
	}

	res = mustRotate(t, r, "h1", "h4", refreshT0.Add(10*time.Second+time.Millisecond), okEv("e2"))
	if res.Outcome != RotateReuse {
		t.Fatalf("at 10s+1ms outcome = %v", res.Outcome)
	}
}

func TestRotateRefresh_ReuseAfterGraceRevokes_ClearsHashesAndWritesOutbox(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	mustRotate(t, r, "h1", "h2", refreshT0, okEv("n"))

	res := mustRotate(t, r, "h1", "h3", refreshT0.Add(11*time.Second), okEv("evt-reuse"))
	if res.Outcome != RotateReuse {
		t.Fatalf("outcome = %v", res.Outcome)
	}
	got := getSession(t, r, "u1", s.ID)
	if !got.Revoked || got.RefreshHash != "" || got.PrevRefreshHash != "" {
		t.Fatalf("want revoked with cleared hashes: %+v", got)
	}
	rows := r.OutboxRows()
	if len(rows) != 1 || rows[0].EventID != "evt-reuse" || rows[0].AggregateID != "u1" {
		t.Fatalf("outbox rows = %+v", rows)
	}
	// R2 is dead afterwards.
	if res := mustRotate(t, r, "h2", "h5", refreshT0.Add(12*time.Second), okEv("x")); res.Outcome != RotateStale {
		t.Fatalf("R2 after reuse outcome = %v", res.Outcome)
	}
	if len(r.OutboxRows()) != 1 {
		t.Fatal("no further outbox rows expected")
	}
}

func TestRotateRefresh_OutboxFailureRollsBack_RevokeAndEventAreAtomic(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	mustRotate(t, r, "h1", "h2", refreshT0, okEv("n"))

	boom := errors.New("boom")
	_, err := r.RotateRefresh(context.Background(), "h1", "h3", refreshT0.Add(time.Minute), refreshGrace, refreshMaxAge,
		func(Session) (OutboxRow, error) { return OutboxRow{}, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want builder error, got %v", err)
	}
	r.FailOutbox = true
	if _, err := r.RotateRefresh(context.Background(), "h1", "h3", refreshT0.Add(time.Minute), refreshGrace, refreshMaxAge, okEv("e")); err == nil {
		t.Fatal("want error from failing outbox write")
	}
	got := getSession(t, r, "u1", s.ID)
	if got.Revoked || got.RefreshHash != "h2" || got.PrevRefreshHash != "h1" {
		t.Fatalf("session must be untouched: %+v", got)
	}
	if len(r.OutboxRows()) != 0 {
		t.Fatal("no outbox row on failure")
	}
}

func TestRotateRefresh_StaleTokenNoRevoke_OlderThanOneRotation(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	mustRotate(t, r, "h1", "h2", refreshT0, okEv("n"))
	mustRotate(t, r, "h2", "h3", refreshT0.Add(time.Minute), okEv("n"))
	before := getSession(t, r, "u1", s.ID)

	res := mustRotate(t, r, "h1", "h9", refreshT0.Add(2*time.Minute), okEv("e"))
	if res.Outcome != RotateStale {
		t.Fatalf("outcome = %v", res.Outcome)
	}
	if after := getSession(t, r, "u1", s.ID); after != before || after.Revoked {
		t.Fatalf("stale must not change or revoke: %+v", after)
	}
	if len(r.OutboxRows()) != 0 {
		t.Fatal("no outbox row for stale")
	}
	if res := mustRotate(t, r, "h3", "h4", refreshT0.Add(3*time.Minute), okEv("n")); res.Outcome != RotateRotated {
		t.Fatalf("R3 must still be accepted, got %v", res.Outcome)
	}
}

func TestRotateRefresh_UnknownAndEmptyHash_StaleNoChange(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	legacy, _ := r.CreateSession(context.Background(), Session{UserID: "u1"}) // no hash
	before := getSession(t, r, "u1", s.ID)
	for _, p := range []string{"nope", ""} {
		if res := mustRotate(t, r, p, "n", refreshT0, okEv("e")); res.Outcome != RotateStale {
			t.Fatalf("presented %q outcome = %v", p, res.Outcome)
		}
	}
	if getSession(t, r, "u1", s.ID) != before || getSession(t, r, "u1", legacy.ID).RotatedAt != (time.Time{}) {
		t.Fatal("rows must be unchanged")
	}
}

func TestRotateRefresh_BindsToOwningSession_TokenResolvesOnlyToItsSession(t *testing.T) {
	r := NewInMemorySessionRepository()
	s1 := seedRefreshSession(t, r, "userA", "a1")
	s2 := seedRefreshSession(t, r, "userB", "b1")
	before := getSession(t, r, "userA", s1.ID)
	res := mustRotate(t, r, "b1", "b2", refreshT0, okEv("n"))
	if res.Outcome != RotateRotated || res.Session.ID != s2.ID || res.Session.UserID != "userB" {
		t.Fatalf("result: %+v", res)
	}
	if after := getSession(t, r, "userA", s1.ID); after != before {
		t.Fatalf("S1 changed: %+v", after)
	}
}

func TestRevokeSessionClearsHashes_EveryPath(t *testing.T) {
	for _, withEv := range []bool{false, true} {
		r := NewInMemorySessionRepository()
		s := seedRefreshSession(t, r, "u1", "h1")
		mustRotate(t, r, "h1", "h2", refreshT0, okEv("n"))
		var ev EnqueueFn
		if withEv {
			ev = okEv("e")
		}
		if err := r.RevokeSession(context.Background(), s.ID, "u1", ev); err != nil {
			t.Fatal(err)
		}
		got := getSession(t, r, "u1", s.ID)
		if !got.Revoked || got.RefreshHash != "" || got.PrevRefreshHash != "" {
			t.Fatalf("withEv=%v: %+v", withEv, got)
		}
		// Prev-token replay within grace after logout is just unknown.
		res := mustRotate(t, r, "h1", "h3", refreshT0.Add(5*time.Second), okEv("x"))
		if res.Outcome != RotateStale {
			t.Fatalf("withEv=%v: prev replay after logout outcome = %v", withEv, res.Outcome)
		}
	}
}

// --- round 2: expiry, absolute lifetime, mandatory ev ---
// Added contract: RotateRefresh(ctx, presented, new, now, grace, maxAge, ev);
// new outcomes RotateExpired (refresh_expires_at <= now: no change, ev not
// called) and RotateLifetimeExceeded (now - created_at > maxAge: revoke, clear
// hashes, ev called once in the same tx). ev == nil is an error, no change.

type evProbe struct{ calls int }

func (p *evProbe) fn(id string) EnqueueFn {
	return func(s Session) (OutboxRow, error) {
		p.calls++
		return okEv(id)(s)
	}
}

func seedRaw(t *testing.T, r *InMemorySessionRepository, s Session) Session {
	t.Helper()
	out, err := r.CreateSession(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRotateRefresh_ExpiredLiveToken_ReturnsExpiredNoChange_RefreshTokenExpired(t *testing.T) {
	for name, exp := range map[string]time.Time{"boundary-equal-now": refreshT0, "past": refreshT0.Add(-time.Second)} {
		t.Run(name, func(t *testing.T) {
			r := NewInMemorySessionRepository()
			s := seedRaw(t, r, Session{UserID: "u1", RefreshHash: "h1", RefreshExpiresAt: exp, CreatedAt: refreshT0.Add(-time.Hour)})
			before := getSession(t, r, "u1", s.ID)
			p := &evProbe{}
			res := mustRotate(t, r, "h1", "h2", refreshT0, p.fn("e"))
			if res.Outcome != RotateExpired {
				t.Fatalf("outcome = %v", res.Outcome)
			}
			if after := getSession(t, r, "u1", s.ID); after != before || after.Revoked {
				t.Fatalf("expired must not change row:\n%+v\n%+v", before, after)
			}
			if p.calls != 0 || len(r.OutboxRows()) != 0 {
				t.Fatalf("ev calls=%d outbox=%d", p.calls, len(r.OutboxRows()))
			}
		})
	}
}

func TestRotateRefresh_JustBeforeExpiry_StillRotates_RefreshTokenExpired(t *testing.T) {
	r := NewInMemorySessionRepository()
	seedRaw(t, r, Session{UserID: "u1", RefreshHash: "h1", RefreshExpiresAt: refreshT0.Add(time.Millisecond), CreatedAt: refreshT0.Add(-time.Hour)})
	if res := mustRotate(t, r, "h1", "h2", refreshT0, okEv("e")); res.Outcome != RotateRotated {
		t.Fatalf("outcome = %v", res.Outcome)
	}
}

func TestRotateRefresh_ExpiredSessionGraceReplay_ReturnsExpiredNoChange_RefreshTokenExpired(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRaw(t, r, Session{UserID: "u1", RefreshHash: "h2", PrevRefreshHash: "h1", RotatedAt: refreshT0,
		RefreshExpiresAt: refreshT0.Add(5 * time.Second), CreatedAt: refreshT0.Add(-time.Hour)})
	before := getSession(t, r, "u1", s.ID)
	p := &evProbe{}
	res := mustRotate(t, r, "h1", "h3", refreshT0.Add(6*time.Second), p.fn("e"))
	if res.Outcome != RotateExpired {
		t.Fatalf("outcome = %v", res.Outcome)
	}
	if after := getSession(t, r, "u1", s.ID); after != before {
		t.Fatalf("row changed: %+v", after)
	}
	if p.calls != 0 || len(r.OutboxRows()) != 0 {
		t.Fatal("no ev / outbox expected")
	}
}

func TestRotateRefresh_AbsoluteLifetimeExceeded_RevokesWithOneEvent_AbsoluteLifetimeExceeded(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRaw(t, r, Session{UserID: "u1", RefreshHash: "h1", RefreshExpiresAt: refreshT0.Add(time.Hour),
		CreatedAt: refreshT0.Add(-refreshMaxAge - time.Millisecond)})
	p := &evProbe{}
	res := mustRotate(t, r, "h1", "h2", refreshT0, p.fn("evt-life"))
	if res.Outcome != RotateLifetimeExceeded {
		t.Fatalf("outcome = %v", res.Outcome)
	}
	got := getSession(t, r, "u1", s.ID)
	if !got.Revoked || got.RefreshHash != "" || got.PrevRefreshHash != "" {
		t.Fatalf("want revoked + cleared: %+v", got)
	}
	rows := r.OutboxRows()
	if p.calls != 1 || len(rows) != 1 || rows[0].EventID != "evt-life" {
		t.Fatalf("calls=%d rows=%+v", p.calls, rows)
	}
	if res := mustRotate(t, r, "h1", "h3", refreshT0.Add(time.Second), p.fn("again")); res.Outcome != RotateStale {
		t.Fatalf("after revoke outcome = %v", res.Outcome)
	}
	if len(r.OutboxRows()) != 1 {
		t.Fatal("exactly one outbox row expected")
	}
}

func TestRotateRefresh_ExactlyMaxAge_StillRotates_AbsoluteLifetimeBoundary(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRaw(t, r, Session{UserID: "u1", RefreshHash: "h1", RefreshExpiresAt: refreshT0.Add(time.Hour),
		CreatedAt: refreshT0.Add(-refreshMaxAge)})
	p := &evProbe{}
	res := mustRotate(t, r, "h1", "h2", refreshT0, p.fn("e"))
	if res.Outcome != RotateRotated || p.calls != 0 {
		t.Fatalf("outcome=%v calls=%d", res.Outcome, p.calls)
	}
	if got := getSession(t, r, "u1", s.ID); got.Revoked || got.RefreshHash != "h2" {
		t.Fatalf("row: %+v", got)
	}
}

func TestRotateRefresh_LifetimeExceededOutboxFailure_Atomic_AbsoluteLifetimeExceeded(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRaw(t, r, Session{UserID: "u1", RefreshHash: "h1", RefreshExpiresAt: refreshT0.Add(time.Hour),
		CreatedAt: refreshT0.Add(-refreshMaxAge - time.Second)})
	before := getSession(t, r, "u1", s.ID)
	boom := errors.New("boom")
	if _, err := r.RotateRefresh(context.Background(), "h1", "h2", refreshT0, refreshGrace, refreshMaxAge,
		func(Session) (OutboxRow, error) { return OutboxRow{}, boom }); !errors.Is(err, boom) {
		t.Fatalf("want builder error, got %v", err)
	}
	r.FailOutbox = true
	if _, err := r.RotateRefresh(context.Background(), "h1", "h2", refreshT0, refreshGrace, refreshMaxAge, okEv("e")); err == nil {
		t.Fatal("want error from failing outbox write")
	}
	if after := getSession(t, r, "u1", s.ID); after != before || after.Revoked {
		t.Fatalf("must be unchanged: %+v", after)
	}
	if len(r.OutboxRows()) != 0 {
		t.Fatal("no outbox rows expected")
	}
}

func TestRotateRefresh_NilEvIsRejected_NoChange(t *testing.T) {
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	before := getSession(t, r, "u1", s.ID)
	if _, err := r.RotateRefresh(context.Background(), "h1", "h2", refreshT0, refreshGrace, refreshMaxAge, nil); err == nil {
		t.Fatal("want error for nil ev")
	}
	if after := getSession(t, r, "u1", s.ID); after != before {
		t.Fatalf("row changed: %+v", after)
	}
}

func TestRotateRefresh_FaultsWrapErrRotateFault_NoHashText(t *testing.T) { // scenario: Revoke and event are atomic (typed fault)
	const secret = "LEAKCHECK-deadbeefcafef00d"
	boom := errors.New("boom")
	cases := map[string]func(t *testing.T) error{
		"ev error": func(t *testing.T) error {
			r := NewInMemorySessionRepository()
			seedRefreshSession(t, r, "u1", "h1")
			mustRotate(t, r, "h1", "h2", refreshT0, okEv("n"))
			_, err := r.RotateRefresh(context.Background(), "h1", "h3", refreshT0.Add(time.Minute), refreshGrace, refreshMaxAge,
				func(Session) (OutboxRow, error) { return OutboxRow{}, boom })
			return err
		},
		"FailOutbox": func(t *testing.T) error {
			r := NewInMemorySessionRepository()
			seedRefreshSession(t, r, "u1", "h1")
			mustRotate(t, r, "h1", "h2", refreshT0, okEv("n"))
			r.FailOutbox = true
			_, err := r.RotateRefresh(context.Background(), "h1", "h3", refreshT0.Add(time.Minute), refreshGrace, refreshMaxAge, okEv("e"))
			return err
		},
		"lifetime exceeded FailOutbox": func(t *testing.T) error {
			r := NewInMemorySessionRepository()
			seedRaw(t, r, Session{UserID: "u1", RefreshHash: "h1", RefreshExpiresAt: refreshT0.Add(time.Hour),
				CreatedAt: refreshT0.Add(-refreshMaxAge - time.Second)})
			r.FailOutbox = true
			_, err := r.RotateRefresh(context.Background(), "h1", "h2", refreshT0, refreshGrace, refreshMaxAge, okEv("e"))
			return err
		},
		"newHash conflict": func(t *testing.T) error {
			r := NewInMemorySessionRepository()
			seedRefreshSession(t, r, "u1", "k1")
			seedRefreshSession(t, r, "u2", secret)
			_, err := r.RotateRefresh(context.Background(), "k1", secret, refreshT0, refreshGrace, refreshMaxAge, okEv("e"))
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			err := run(t)
			if !errors.Is(err, ErrRotateFault) {
				t.Fatalf("want errors.Is ErrRotateFault, got %v", err)
			}
			if err != nil && strings.Contains(err.Error(), secret) {
				t.Fatalf("error leaks the hash: %v", err)
			}
		})
	}
}

func TestRotateRefresh_PlainStaleAndNilEvAreNotRotateFaults(t *testing.T) { // scenario: Identity database down is not a logout (faults are only ev/outbox/conflict)
	r := NewInMemorySessionRepository()
	seedRefreshSession(t, r, "u1", "h1")
	res, err := r.RotateRefresh(context.Background(), "nope", "h2", refreshT0, refreshGrace, refreshMaxAge, okEv("e"))
	if err != nil || res.Outcome != RotateStale {
		t.Fatalf("stale must be nil error: %+v %v", res, err)
	}
}

func TestRevokeSession_EnqueueFailuresWrapErrRotateFault(t *testing.T) { // scenario: Revoke and event are atomic (revoke path)
	boom := errors.New("boom")
	r := NewInMemorySessionRepository()
	s := seedRefreshSession(t, r, "u1", "h1")
	err := r.RevokeSession(context.Background(), s.ID, "u1", func(Session) (OutboxRow, error) { return OutboxRow{}, boom })
	if !errors.Is(err, ErrRotateFault) {
		t.Fatalf("ev error: want ErrRotateFault, got %v", err)
	}
	r.FailOutbox = true
	if err := r.RevokeSession(context.Background(), s.ID, "u1", okEv("e")); !errors.Is(err, ErrRotateFault) {
		t.Fatalf("FailOutbox: want ErrRotateFault, got %v", err)
	}
	if getSession(t, r, "u1", s.ID).Revoked {
		t.Fatal("must stay unrevoked")
	}
}
