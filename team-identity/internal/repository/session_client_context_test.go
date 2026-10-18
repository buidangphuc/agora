package repository

// Tests for change frontend-session-client-context, task 1.1.
// Contract: RotateRefresh(ctx, presented, new, now, grace, maxAge, ip, device string, ev EnqueueFn).
// Only the rotated branch writes ip/device (COALESCE(NULLIF(x,''), stored)) next to last_seen.

import (
	"context"
	"testing"
	"time"
)

const (
	ccOldIP  = "192.0.2.1"
	ccOldDev = "AgoraTest/1"
	ccNewIP  = "198.51.100.4"
	ccNewDev = "AgoraTest/2"
)


func ccSeed(t *testing.T, r SessionRepository, user, hash string, created time.Time) Session {
	t.Helper()
	s, err := r.CreateSession(context.Background(), Session{
		UserID: user, IP: ccOldIP, Device: ccOldDev, RefreshHash: hash,
		RefreshExpiresAt: refreshT0.Add(30 * 24 * time.Hour), CreatedAt: created, LastSeen: created,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s
}

func ccRotate(t *testing.T, r SessionRepository, p, n string, now time.Time, ip, dev string) RotateResult {
	t.Helper()
	res, err := r.RotateRefresh(context.Background(), p, n, now, refreshGrace, refreshMaxAge, ip, dev, okEv("cc-ev"))
	if err != nil {
		t.Fatalf("RotateRefresh: %v", err)
	}
	return res
}

func ccRotationRecordsClient(t *testing.T, r SessionRepository, user string) {
	s := ccSeed(t, r, user, "cc-h1", refreshT0.Add(-time.Hour))
	if res := ccRotate(t, r, "cc-h1", "cc-h2", refreshT0, ccNewIP, ccNewDev); res.Outcome != RotateRotated {
		t.Fatalf("outcome = %v", res.Outcome)
	}
	got := getSession(t, r, user, s.ID)
	if got.IP != ccNewIP || got.Device != ccNewDev || !got.LastSeen.Equal(refreshT0) || got.RefreshHash != "cc-h2" {
		t.Fatalf("row after rotate: %+v", got)
	}
}

func ccEmptyKeepsStored(t *testing.T, r SessionRepository, user string) {
	s := ccSeed(t, r, user, "cc-h1", refreshT0.Add(-time.Hour))
	ccRotate(t, r, "cc-h1", "cc-h2", refreshT0, "", "")
	got := getSession(t, r, user, s.ID)
	if got.IP != ccOldIP || got.Device != ccOldDev || !got.LastSeen.Equal(refreshT0) || got.RefreshHash != "cc-h2" {
		t.Fatalf("empty client must keep ip/device and advance last_seen: %+v", got)
	}
	// Partially empty: only the non-empty field is replaced.
	ccRotate(t, r, "cc-h2", "cc-h3", refreshT0.Add(time.Minute), ccNewIP, "")
	got = getSession(t, r, user, s.ID)
	if got.IP != ccNewIP || got.Device != ccOldDev {
		t.Fatalf("partial update: %+v", got)
	}
}

func ccGraceReplayNoUpdate(t *testing.T, r SessionRepository, user string) {
	s := ccSeed(t, r, user, "cc-h1", refreshT0.Add(-time.Hour))
	ccRotate(t, r, "cc-h1", "cc-h2", refreshT0, ccNewIP, ccNewDev)
	before := getSession(t, r, user, s.ID)
	res := ccRotate(t, r, "cc-h1", "cc-h3", refreshT0.Add(5*time.Second), "203.0.113.9", "Other/9")
	if res.Outcome != RotateReplayedInGrace {
		t.Fatalf("outcome = %v", res.Outcome)
	}
	if after := getSession(t, r, user, s.ID); after != before {
		t.Fatalf("grace replay changed row:\nbefore %+v\nafter  %+v", before, after)
	}
}

func ccRejectedUpdatesNothing(t *testing.T, r SessionRepository, user string) {
	// stale (unknown hash)
	live := ccSeed(t, r, user, "cc-live", refreshT0.Add(-time.Hour))
	before := getSession(t, r, user, live.ID)
	if res := ccRotate(t, r, "cc-unknown", "cc-x", refreshT0, ccNewIP, ccNewDev); res.Outcome != RotateStale {
		t.Fatalf("stale outcome = %v", res.Outcome)
	}
	if after := getSession(t, r, user, live.ID); after != before {
		t.Fatalf("stale changed row: %+v", after)
	}
	// expired
	exp, err := r.CreateSession(context.Background(), Session{UserID: user, IP: ccOldIP, Device: ccOldDev, RefreshHash: "cc-exp",
		RefreshExpiresAt: refreshT0.Add(-time.Minute), CreatedAt: refreshT0.Add(-time.Hour), LastSeen: refreshT0.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	beforeExp := getSession(t, r, user, exp.ID)
	if res := ccRotate(t, r, "cc-exp", "cc-y", refreshT0, ccNewIP, ccNewDev); res.Outcome != RotateExpired {
		t.Fatalf("expired outcome = %v", res.Outcome)
	}
	if after := getSession(t, r, user, exp.ID); after != beforeExp {
		t.Fatalf("expired changed row: %+v", after)
	}
	// reuse after grace: revoked, ip/device untouched
	reu := ccSeed(t, r, user, "cc-r1", refreshT0.Add(-time.Hour))
	ccRotate(t, r, "cc-r1", "cc-r2", refreshT0, ccOldIP, ccOldDev)
	if res := ccRotate(t, r, "cc-r1", "cc-r3", refreshT0.Add(time.Minute), ccNewIP, ccNewDev); res.Outcome != RotateReuse {
		t.Fatalf("reuse outcome = %v", res.Outcome)
	}
	if got := getSession(t, r, user, reu.ID); got.IP != ccOldIP || got.Device != ccOldDev || !got.Revoked {
		t.Fatalf("reuse must revoke without updating client: %+v", got)
	}
	// revoked session's token is stale
	if res := ccRotate(t, r, "cc-r2", "cc-r4", refreshT0.Add(2*time.Minute), ccNewIP, ccNewDev); res.Outcome != RotateStale {
		t.Fatalf("revoked outcome = %v", res.Outcome)
	}
	if got := getSession(t, r, user, reu.ID); got.IP != ccOldIP || got.Device != ccOldDev {
		t.Fatalf("revoked session changed: %+v", got)
	}
	// absolute lifetime exceeded
	old := ccSeed(t, r, user, "cc-l1", refreshT0.Add(-refreshMaxAge-time.Second))
	if res := ccRotate(t, r, "cc-l1", "cc-l2", refreshT0, ccNewIP, ccNewDev); res.Outcome != RotateLifetimeExceeded {
		t.Fatalf("lifetime outcome = %v", res.Outcome)
	}
	if got := getSession(t, r, user, old.ID); got.IP != ccOldIP || got.Device != ccOldDev {
		t.Fatalf("lifetime-revoked session changed client: %+v", got)
	}
}

func TestSessionClientContext_RotationRecordsIPViaRepository_InMemory(t *testing.T) {
	ccRotationRecordsClient(t, NewInMemorySessionRepository(), "u1")
}

func TestSessionClientContext_EmptyClientKeepsStoredValues_Boundary_InMemory(t *testing.T) {
	ccEmptyKeepsStored(t, NewInMemorySessionRepository(), "u1")
}

func TestSessionClientContext_GraceReplayDoesNotUpdate_Boundary_InMemory(t *testing.T) {
	ccGraceReplayNoUpdate(t, NewInMemorySessionRepository(), "u1")
}

func TestSessionClientContext_RejectedRefreshUpdatesNothing_Error_InMemory(t *testing.T) {
	ccRejectedUpdatesNothing(t, NewInMemorySessionRepository(), "u1")
}

func TestSessionClientContext_RotationRecordsIPViaRepository_Postgres(t *testing.T) {
	_, repo := refreshPG(t)
	ccRotationRecordsClient(t, repo, "u-rf1")
}

func TestSessionClientContext_EmptyClientKeepsStoredValues_Boundary_Postgres(t *testing.T) {
	_, repo := refreshPG(t)
	ccEmptyKeepsStored(t, repo, "u-rf1")
}

func TestSessionClientContext_GraceReplayDoesNotUpdate_Boundary_Postgres(t *testing.T) {
	_, repo := refreshPG(t)
	ccGraceReplayNoUpdate(t, repo, "u-rf1")
}

func TestSessionClientContext_RejectedRefreshUpdatesNothing_Error_Postgres(t *testing.T) {
	_, repo := refreshPG(t)
	ccRejectedUpdatesNothing(t, repo, "u-rf1")
}
