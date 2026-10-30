package tombstone_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-search/internal/tombstone"
)

type fakePurger struct {
	cutoffs []time.Time
	err     error
}

func (f *fakePurger) PurgeTombstones(_ context.Context, olderThan time.Time) (int64, error) {
	f.cutoffs = append(f.cutoffs, olderThan)
	if f.err != nil {
		return 0, f.err
	}
	return 3, nil
}

func logger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

// Scenario "An expired tombstone is purged": each purge removes tombstones older
// than now - TTL and logs how many it removed.
func TestPurge_UsesTTLCutoff(t *testing.T) {
	clock := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	p := &fakePurger{}
	var logs bytes.Buffer

	err := tombstone.Purge(context.Background(), p, 30*time.Second, logger(&logs), func() time.Time { return clock })
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if len(p.cutoffs) != 1 {
		t.Fatalf("want 1 purge call, got %d", len(p.cutoffs))
	}
	if want := clock.Add(-30 * time.Second); !p.cutoffs[0].Equal(want) {
		t.Errorf("cutoff = %s, want %s", p.cutoffs[0], want)
	}
	if !strings.Contains(logs.String(), "purged=3") {
		t.Errorf("want the purged count logged, got:\n%s", logs.String())
	}

	clock = clock.Add(time.Hour)
	if err := tombstone.Purge(context.Background(), p, 30*time.Second, logger(&logs), func() time.Time { return clock }); err != nil {
		t.Fatalf("second Purge: %v", err)
	}
	if want := clock.Add(-30 * time.Second); !p.cutoffs[1].Equal(want) {
		t.Errorf("second cutoff = %s, want %s (cutoff follows the injected clock)", p.cutoffs[1], want)
	}
}

// Scenario "A failed purge does not stop the indexer": the failure is logged
// with its cause; if Purge reports it, the error wraps the cause so
// lifecycle.Every (which keeps ticking after an error) can log it too.
func TestPurge_FailureIsLoggedNotFatal(t *testing.T) {
	cause := errors.New("cluster down")
	p := &fakePurger{err: cause}
	var logs bytes.Buffer
	clock := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	err := tombstone.Purge(context.Background(), p, time.Minute, logger(&logs), func() time.Time { return clock })
	if err != nil && !errors.Is(err, cause) {
		t.Errorf("Purge error = %v, want nil or one wrapping the cause", err)
	}
	if !strings.Contains(logs.String(), "cluster down") {
		t.Errorf("want the failure logged with its cause, got:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), "purged=") {
		t.Errorf("a failed purge must not log a purged count:\n%s", logs.String())
	}

	p.err = nil
	if err := tombstone.Purge(context.Background(), p, time.Minute, logger(&logs), func() time.Time { return clock }); err != nil {
		t.Fatalf("Purge after recovery: %v", err)
	}
	if len(p.cutoffs) != 2 {
		t.Errorf("next call must purge again, got %d calls", len(p.cutoffs))
	}
}
