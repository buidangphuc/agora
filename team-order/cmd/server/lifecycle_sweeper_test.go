package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSweeper struct {
	calls chan time.Time
	n     atomic.Int64
}

func (f *fakeSweeper) SweepLifecycle(_ context.Context, now time.Time) {
	f.n.Add(1)
	select {
	case f.calls <- now:
	default:
	}
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Scenario: Order expiry: Order unpaid past its deadline is cancelled and its stock released
// (wiring: the sweeper drives SweepLifecycle on every tick).
func TestRunLifecycleSweeper_TicksCallSweepLifecycle_OrderUnpaidPastDeadlineIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeSweeper{calls: make(chan time.Time, 16)}
	go runLifecycleSweeper(ctx, f, time.Millisecond, quietLogger())

	for i := 0; i < 3; i++ {
		select {
		case now := <-f.calls:
			if now.IsZero() {
				t.Fatalf("tick %d: SweepLifecycle got zero time", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("tick %d: SweepLifecycle was never called", i)
		}
	}
}

// Scenario: Order expiry: Order unpaid past its deadline is cancelled and its stock released
// (wiring: cancelling the context stops the sweeper).
func TestRunLifecycleSweeper_StopsOnContextCancel_OrderUnpaidPastDeadlineIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeSweeper{calls: make(chan time.Time, 16)}
	done := make(chan struct{})
	go func() {
		runLifecycleSweeper(ctx, f, time.Millisecond, quietLogger())
		close(done)
	}()
	select {
	case <-f.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper never ticked")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runLifecycleSweeper did not return after context cancel")
	}
}

// Scenario: Order expiry: Order unpaid past its deadline is cancelled and its stock released
// (docs: README documents the new env vars, events and reason code).
func TestREADME_DocumentsLifecycleConfig_OrderUnpaidPastDeadlineIsCancelled(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(b)
	for _, want := range []string{
		"PAYMENT_DEADLINE", "PAYMENT_REMINDER_BEFORE", "LIFECYCLE_SWEEP_INTERVAL",
		"OrderPlaced", "OrderPaymentReminder", "PAYMENT_EXPIRED",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md does not mention %q", want)
		}
	}
}
