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
	"testing"
	"time"
)

var refreshT0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

const refreshGrace = 10 * time.Second

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
	res, err := r.RotateRefresh(context.Background(), presented, next, now, refreshGrace, ev)
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
	mustRotate(t, r, "h1", "h2", refreshT0, nil)
	res := mustRotate(t, r, "h2", "h3", refreshT0.Add(time.Minute), nil)
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
	mustRotate(t, r, "h1", "h2", refreshT0, nil)
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
	mustRotate(t, r, "h1", "h2", refreshT0, nil)

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
	mustRotate(t, r, "h1", "h2", refreshT0, nil)

	boom := errors.New("boom")
	_, err := r.RotateRefresh(context.Background(), "h1", "h3", refreshT0.Add(time.Minute), refreshGrace,
		func(Session) (OutboxRow, error) { return OutboxRow{}, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want builder error, got %v", err)
	}
	r.FailOutbox = true
	if _, err := r.RotateRefresh(context.Background(), "h1", "h3", refreshT0.Add(time.Minute), refreshGrace, okEv("e")); err == nil {
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
	mustRotate(t, r, "h1", "h2", refreshT0, nil)
	mustRotate(t, r, "h2", "h3", refreshT0.Add(time.Minute), nil)
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
	if res := mustRotate(t, r, "h3", "h4", refreshT0.Add(3*time.Minute), nil); res.Outcome != RotateRotated {
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
	res := mustRotate(t, r, "b1", "b2", refreshT0, nil)
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
		mustRotate(t, r, "h1", "h2", refreshT0, nil)
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
