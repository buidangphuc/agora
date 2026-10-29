package identity_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

var sweepNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

const (
	sweepSessionRetention = 720 * time.Hour
	sweepHistoryRetention = 4320 * time.Hour
)

// recordingRepo wraps a real in-memory repository and records every purge call.
type recordingRepo struct {
	store.SessionRepository

	mu           sync.Mutex
	limits       []int
	returned     []int
	failSessions int  // fail this many initial PurgeSessions calls
	failHistory  bool // every PurgeLoginHistory call fails
	calls        int  // PurgeSessions call count
}

func (r *recordingRepo) PurgeSessions(ctx context.Context, now time.Time, retention time.Duration, limit int) (int, error) {
	r.mu.Lock()
	r.calls++
	fail := r.calls <= r.failSessions
	r.limits = append(r.limits, limit)
	r.mu.Unlock()
	var n int
	var err error
	if fail {
		err = errors.New("simulated db error")
	} else {
		n, err = r.SessionRepository.PurgeSessions(ctx, now, retention, limit)
	}
	r.mu.Lock()
	r.returned = append(r.returned, n)
	r.mu.Unlock()
	return n, err
}

func (r *recordingRepo) PurgeLoginHistory(ctx context.Context, before time.Time, limit int) (int, error) {
	if r.failHistory {
		return 0, errors.New("simulated login history db error")
	}
	return r.SessionRepository.PurgeLoginHistory(ctx, before, limit)
}

func seedDeadSessions(t *testing.T, repo store.SessionRepository, n int, userID, ip, device string) {
	t.Helper()
	seedDeadSessionsAt(t, repo, n, userID, ip, device, sweepNow)
}

func seedDeadSessionsAt(t *testing.T, repo store.SessionRepository, n int, userID, ip, device string, ref time.Time) {
	t.Helper()
	old := ref.Add(-sweepSessionRetention - 24*time.Hour)
	for i := 0; i < n; i++ {
		_, err := repo.CreateSession(context.Background(), store.Session{
			UserID: userID, IP: ip, Device: device,
			CreatedAt: old, LastSeen: old, Revoked: true,
		})
		if err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
}

func liveSessionCount(t *testing.T, repo store.SessionRepository, userID string) int {
	t.Helper()
	items, err := repo.ListSessions(context.Background(), userID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	return len(items)
}

func newSweeper(repo store.SessionRepository, logger *slog.Logger) *identity.SessionSweeper {
	return identity.NewSessionSweeper(repo, identity.SessionSweeperConfig{
		SessionRetention:      sweepSessionRetention,
		LoginHistoryRetention: sweepHistoryRetention,
		BatchSize:             500,
	}, logger)
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func jsonLogger(buf *syncBuf) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// Scenario: Session retention: batches of 500.
func TestSweepOnceBatches(t *testing.T) {
	mem := storetest.NewInMemorySessionRepository()
	rec := &recordingRepo{SessionRepository: mem}
	seedDeadSessions(t, mem, 1200, "user-batch", "10.0.0.1", "dev")

	res, err := newSweeper(rec, jsonLogger(&syncBuf{})).SweepOnce(context.Background(), sweepNow)
	if err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	rec.mu.Lock()
	got := append([]int(nil), rec.returned...)
	limits := append([]int(nil), rec.limits...)
	rec.mu.Unlock()
	if fmt.Sprint(got) != "[500 500 200]" {
		t.Fatalf("session purge batches = %v, want [500 500 200]", got)
	}
	for _, l := range limits {
		if l != 500 {
			t.Fatalf("batch limit = %d, want 500 (all: %v)", l, limits)
		}
	}
	if res.Sessions != 1200 {
		t.Fatalf("result Sessions = %d, want 1200", res.Sessions)
	}
	if left := liveSessionCount(t, mem, "user-batch"); left != 0 {
		t.Fatalf("%d eligible rows remain, want 0", left)
	}
}

// Scenario: Session retention: batches of 500 (boundary: an exact multiple needs one empty probe batch).
func TestSweepOnceBatchesExactMultiple(t *testing.T) {
	mem := storetest.NewInMemorySessionRepository()
	rec := &recordingRepo{SessionRepository: mem}
	seedDeadSessions(t, mem, 1000, "user-exact", "10.0.0.1", "dev")

	res, err := newSweeper(rec, jsonLogger(&syncBuf{})).SweepOnce(context.Background(), sweepNow)
	if err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	rec.mu.Lock()
	got := append([]int(nil), rec.returned...)
	rec.mu.Unlock()
	if fmt.Sprint(got) != "[500 500 0]" {
		t.Fatalf("session purge batches = %v, want [500 500 0]", got)
	}
	if res.Sessions != 1000 {
		t.Fatalf("result Sessions = %d, want 1000", res.Sessions)
	}
}

// Scenario: Session retention: batches of 500 (nothing eligible: a single empty batch, active rows untouched).
func TestSweepOnceNothingEligibleKeepsActive(t *testing.T) {
	mem := storetest.NewInMemorySessionRepository()
	rec := &recordingRepo{SessionRepository: mem}
	if _, err := mem.CreateSession(context.Background(), store.Session{
		UserID: "user-live", CreatedAt: sweepNow.Add(-time.Hour), LastSeen: sweepNow,
		RefreshExpiresAt: sweepNow.Add(24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	res, err := newSweeper(rec, jsonLogger(&syncBuf{})).SweepOnce(context.Background(), sweepNow)
	if err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	rec.mu.Lock()
	got := append([]int(nil), rec.returned...)
	rec.mu.Unlock()
	if fmt.Sprint(got) != "[0]" {
		t.Fatalf("session purge batches = %v, want [0]", got)
	}
	if res.Sessions != 0 || res.LoginEvents != 0 {
		t.Fatalf("result = %+v, want zero", res)
	}
	if n := liveSessionCount(t, mem, "user-live"); n != 1 {
		t.Fatalf("active session count = %d, want 1", n)
	}
}

// Scenario: Session retention: batches of 500 (login history is purged in batches too).
func TestSweepOnceLoginHistoryBatches(t *testing.T) {
	mem := storetest.NewInMemorySessionRepository()
	old := sweepNow.Add(-sweepHistoryRetention - time.Hour)
	for i := 0; i < 600; i++ {
		if _, err := mem.RecordLogin(context.Background(), store.LoginEvent{UserID: "user-h", CreatedAt: old, Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	recent := sweepNow.Add(-time.Hour)
	if _, err := mem.RecordLogin(context.Background(), store.LoginEvent{UserID: "user-h", CreatedAt: recent, Success: true}); err != nil {
		t.Fatal(err)
	}
	res, err := newSweeper(mem, jsonLogger(&syncBuf{})).SweepOnce(context.Background(), sweepNow)
	if err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	if res.LoginEvents != 600 {
		t.Fatalf("result LoginEvents = %d, want 600", res.LoginEvents)
	}
	_, total, err := mem.ListLoginHistory(context.Background(), "user-h", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("login history total = %d, want 1 (recent kept)", total)
	}
}

// Scenario: Session retention: sweep error is logged and retried (SweepOnce surfaces the error, next call recovers).
func TestSweeperSurvivesErrorSweepOnce(t *testing.T) {
	mem := storetest.NewInMemorySessionRepository()
	rec := &recordingRepo{SessionRepository: mem, failSessions: 1}
	seedDeadSessions(t, mem, 3, "user-err", "10.0.0.1", "dev")
	sw := newSweeper(rec, jsonLogger(&syncBuf{}))

	if _, err := sw.SweepOnce(context.Background(), sweepNow); err == nil {
		t.Fatal("first SweepOnce: want the purge error, got nil")
	}
	if n := liveSessionCount(t, mem, "user-err"); n != 3 {
		t.Fatalf("rows after failed sweep = %d, want 3 untouched", n)
	}
	res, err := sw.SweepOnce(context.Background(), sweepNow)
	if err != nil {
		t.Fatalf("second SweepOnce: %v", err)
	}
	if res.Sessions != 3 {
		t.Fatalf("second sweep Sessions = %d, want 3", res.Sessions)
	}
}

func tickN(t *testing.T, sw *identity.SessionSweeper, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := sw.Tick(context.Background()); err != nil {
			t.Fatalf("Tick %d returned %v, want nil", i+1, err)
		}
	}
}

// Scenario: Session retention: sweep error is logged and retried.
func TestSweeperSurvivesError(t *testing.T) {
	mem := storetest.NewInMemorySessionRepository()
	rec := &recordingRepo{SessionRepository: mem, failSessions: 1}
	seedDeadSessionsAt(t, mem, 4, "user-run", "10.0.0.1", "dev", time.Now())
	buf := &syncBuf{}
	sw := newSweeper(rec, jsonLogger(buf))

	tickN(t, sw, 1)
	if n := liveSessionCount(t, mem, "user-run"); n != 4 {
		t.Fatalf("rows after failed tick = %d, want 4 untouched", n)
	}
	if logs := buf.String(); !strings.Contains(logs, `"level":"WARN"`) {
		t.Fatalf("no warning logged for the failed tick:\n%s", logs)
	}
	tickN(t, sw, 1)

	if n := liveSessionCount(t, mem, "user-run"); n != 0 {
		t.Fatalf("rows after retry = %d, want 0 (next tick purges normally)", n)
	}
	logs := buf.String()
	if !strings.Contains(logs, `"level":"WARN"`) {
		t.Fatalf("no warning logged for the failed sweep:\n%s", logs)
	}
	if strings.Contains(logs, `"level":"ERROR"`) {
		t.Fatalf("transient sweep error must be a warning, not ERROR:\n%s", logs)
	}
}

// Scenario: Session retention: purge does not log personal data.
func TestSweeperLogsCountsOnly(t *testing.T) {
	mem := storetest.NewInMemorySessionRepository()
	rec := &recordingRepo{SessionRepository: mem, failSessions: 1}
	const (
		userID = "9f1c2d3e-aaaa-bbbb-cccc-userid000001"
		ip     = "203.0.113.77"
		device = "Pixel-9-SECRETDEVICE"
		hash   = "deadbeefcafebabe0123456789"
	)
	tickNow := time.Now()
	old := tickNow.Add(-sweepSessionRetention - 24*time.Hour)
	for i := 0; i < 3; i++ {
		if _, err := mem.CreateSession(context.Background(), store.Session{
			UserID: userID, IP: ip, Device: device, Revoked: true,
			CreatedAt: old, LastSeen: old, RefreshHash: hash, PrevRefreshHash: hash,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mem.RecordLogin(context.Background(), store.LoginEvent{
		UserID: userID, IP: ip, UserAgent: device, CreatedAt: tickNow.Add(-sweepHistoryRetention - time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	buf := &syncBuf{}
	sw := newSweeper(rec, jsonLogger(buf))

	tickN(t, sw, 3)

	logs := buf.String()
	for _, key := range []string{"sessions_purged", "login_events_purged"} {
		if !strings.Contains(logs, key) {
			t.Fatalf("log lacks count key %q:\n%s", key, logs)
		}
	}
	for _, secret := range []string{userID, ip, device, hash, "SECRETDEVICE", "203.0.113"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log output leaks personal data %q:\n%s", secret, logs)
		}
	}
}

// Scenario: Session retention: login history without rows or with a failed purge.
func TestSweepOnceLoginHistoryErrorIsolated(t *testing.T) {
	// Empty login history table: nothing purged, no error.
	empty := storetest.NewInMemorySessionRepository()
	res, err := newSweeper(empty, jsonLogger(&syncBuf{})).SweepOnce(context.Background(), sweepNow)
	if err != nil {
		t.Fatalf("empty table SweepOnce: %v", err)
	}
	if res.LoginEvents != 0 {
		t.Fatalf("empty table LoginEvents = %d, want 0", res.LoginEvents)
	}

	// Failing login history purge: the session purge of the same pass already ran.
	mem := storetest.NewInMemorySessionRepository()
	rec := &recordingRepo{SessionRepository: mem, failHistory: true}
	seedDeadSessions(t, mem, 3, "user-hist-err", "10.0.0.1", "dev")
	res, err = newSweeper(rec, jsonLogger(&syncBuf{})).SweepOnce(context.Background(), sweepNow)
	if err == nil {
		t.Fatal("SweepOnce: want the login history purge error, got nil")
	}
	if res.Sessions != 3 {
		t.Fatalf("result Sessions = %d, want 3 (sessions purged in the same pass)", res.Sessions)
	}
	if res.LoginEvents != 0 {
		t.Fatalf("result LoginEvents = %d, want 0", res.LoginEvents)
	}
	if n := liveSessionCount(t, mem, "user-hist-err"); n != 0 {
		t.Fatalf("sessions left = %d, want 0 despite the history failure", n)
	}
}
