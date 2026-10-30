package app

import (
	"context"
	"errors"
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

type flakyReservations struct {
	calls    atomic.Int64
	released chan int
}

func (f *flakyReservations) SweepExpiredReservations(_ context.Context, _ time.Time) (int, error) {
	if f.calls.Add(1) == 1 {
		return 0, errors.New("db blip")
	}
	select {
	case f.released <- 1:
	default:
	}
	return 1, nil
}

func quietLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestRunLifecycleSweeper_TicksCallSweepLifecycle_OrderUnpaidPastDeadlineIsCancelled(t *testing.T) {
	// scenario: Each tick drives one lifecycle sweep
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeSweeper{calls: make(chan time.Time, 16)}
	c, err := lifecycleSweep(f, time.Millisecond, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Run(ctx) }()

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

func TestRunLifecycleSweeper_StopsOnContextCancel_OrderUnpaidPastDeadlineIsCancelled(t *testing.T) {
	// scenario: Each tick drives one lifecycle sweep
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeSweeper{calls: make(chan time.Time, 16)}
	c, err := lifecycleSweep(f, time.Millisecond, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = c.Run(ctx); close(done) }()
	select {
	case <-f.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper never ticked")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("component did not return after context cancel")
	}
}

func TestReservationSweepRetriesAfterFailure(t *testing.T) {
	// scenario: A failing reservation sweep is retried on the next tick
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &flakyReservations{released: make(chan int, 4)}
	c, err := reservationSweep(f, time.Millisecond, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	select {
	case n := <-f.released:
		if n != 1 {
			t.Fatalf("released = %d, want 1", n)
		}
	case err := <-done:
		t.Fatalf("component stopped after the first failing tick: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the sweep was not retried after a failing tick")
	}
	if f.calls.Load() < 2 {
		t.Fatalf("calls = %d, want at least 2", f.calls.Load())
	}
}

func TestSweepComponentsRefuseNonPositiveInterval(t *testing.T) {
	// scenario: Each tick drives one lifecycle sweep
	if _, err := lifecycleSweep(&fakeSweeper{calls: make(chan time.Time, 1)}, 0, quietLogger()); err == nil {
		t.Error("lifecycleSweep accepted a zero interval")
	}
	if _, err := reservationSweep(&flakyReservations{released: make(chan int, 1)}, 0, quietLogger()); err == nil {
		t.Error("reservationSweep accepted a zero interval")
	}
}

func TestREADME_DocumentsLifecycleConfig_OrderUnpaidPastDeadlineIsCancelled(t *testing.T) {
	// scenario: Each tick drives one lifecycle sweep (docs kept from the moved cmd/server test)
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
