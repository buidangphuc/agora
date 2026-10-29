package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

// In-memory PurgeSessions / PurgeLoginHistory semantics (change identity-session-retention, task 2).
// Revoked rows age from last_seen here (the in-memory model has no revoked_at).

func memSess(t *testing.T, r *storetest.InMemorySessionRepository, s store.Session) {
	t.Helper()
	if _, err := r.CreateSession(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func memHas(t *testing.T, r *storetest.InMemorySessionRepository, id string) bool {
	t.Helper()
	list, err := r.ListSessions(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == id {
			return true
		}
	}
	return false
}

func TestInMemoryPurgeSessions(t *testing.T) { // Session retention: revoked/expired/absolute/active/boundary (in-memory)
	r := storetest.NewInMemorySessionRepository()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	memSess(t, r, store.Session{ID: "rev-old", UserID: "u1", Revoked: true, CreatedAt: now.Add(-50 * day), LastSeen: now.Add(-31 * day)})
	memSess(t, r, store.Session{ID: "rev-recent", UserID: "u1", Revoked: true, CreatedAt: now.Add(-50 * day), LastSeen: now.Add(-29 * day)})
	memSess(t, r, store.Session{ID: "exp-old", UserID: "u1", CreatedAt: now.Add(-60 * day), LastSeen: now, RefreshExpiresAt: now.Add(-31 * day)})
	memSess(t, r, store.Session{ID: "abs-old", UserID: "u1", CreatedAt: now.Add(-121 * day), LastSeen: now, RefreshExpiresAt: now.Add(10 * day)})
	memSess(t, r, store.Session{ID: "active", UserID: "u1", CreatedAt: now.Add(-80 * day), LastSeen: now, RefreshExpiresAt: now.Add(5 * day)})
	memSess(t, r, store.Session{ID: "exp-yday", UserID: "u1", CreatedAt: now.Add(-50 * day), LastSeen: now, RefreshExpiresAt: now.Add(-1 * day)})
	memSess(t, r, store.Session{ID: "exact", UserID: "u1", Revoked: true, CreatedAt: now.Add(-50 * day), LastSeen: now.Add(-purgeRetention)})
	memSess(t, r, store.Session{ID: "plus1s", UserID: "u1", Revoked: true, CreatedAt: now.Add(-50 * day), LastSeen: now.Add(-purgeRetention - time.Second)})

	n, err := r.PurgeSessions(context.Background(), now, purgeRetention, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("deleted = %d, want 4", n)
	}
	for _, id := range []string{"rev-old", "exp-old", "abs-old", "plus1s"} {
		if memHas(t, r, id) {
			t.Errorf("%s should be purged", id)
		}
	}
	for _, id := range []string{"rev-recent", "active", "exp-yday", "exact"} {
		if !memHas(t, r, id) {
			t.Errorf("%s should be kept", id)
		}
	}
}

func TestInMemoryPurgeSessionsLimit(t *testing.T) { // Session retention: batches of 500 (in-memory limit)
	r := storetest.NewInMemorySessionRepository()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"a", "b", "c"} {
		memSess(t, r, store.Session{ID: id, UserID: "u1", Revoked: true, CreatedAt: now.Add(-60 * day), LastSeen: now.Add(-40 * day)})
	}
	n, err := r.PurgeSessions(context.Background(), now, purgeRetention, 2)
	if err != nil || n != 2 {
		t.Fatalf("batch = %d err=%v, want 2", n, err)
	}
	n, err = r.PurgeSessions(context.Background(), now, purgeRetention, 2)
	if err != nil || n != 1 {
		t.Fatalf("batch = %d err=%v, want 1", n, err)
	}
}

func TestInMemoryPurgeLoginHistory(t *testing.T) { // Session retention: old login history is deleted / login history boundary
	r := storetest.NewInMemorySessionRepository()
	ctx := context.Background()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	before := now.Add(-180 * day)
	for id, at := range map[string]time.Time{"old": now.Add(-181 * day), "exact": before, "new": now.Add(-179 * day)} {
		if _, err := r.RecordLogin(ctx, store.LoginEvent{ID: id, UserID: "u1", CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := r.PurgeLoginHistory(ctx, before, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	got, total, _ := r.ListLoginHistory(ctx, "u1", 10, 0)
	if total != 2 || len(got) != 2 {
		t.Fatalf("remaining = %+v, want exact and new", got)
	}
	n, err = r.PurgeLoginHistory(ctx, before, 500)
	if err != nil || n != 0 {
		t.Fatalf("second purge = %d err=%v, want 0", n, err)
	}
}
