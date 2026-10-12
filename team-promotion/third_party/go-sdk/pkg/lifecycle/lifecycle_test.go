package lifecycle_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
)

const wait = 5 * time.Second

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func runAsync(ctx context.Context, o lifecycle.Options, cs ...lifecycle.Component) <-chan error {
	o.Logger = quiet()
	done := make(chan error, 1)
	go func() { done <- lifecycle.Run(ctx, o, cs...) }()
	return done
}

func result(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(wait):
		t.Fatal("Run did not return in time")
		return nil
	}
}

func TestFailingComponentCancelsOthers(t *testing.T) {
	boom := errors.New("boom")
	started := make(chan struct{})
	otherCancelled := make(chan struct{})

	other := lifecycle.Func(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(otherCancelled)
		return ctx.Err()
	})
	failing := lifecycle.Func(func(context.Context) error {
		<-started
		return boom
	})

	err := result(t, runAsync(context.Background(), lifecycle.Options{ShutdownGrace: wait}, other, failing))
	if !errors.Is(err, boom) {
		t.Fatalf("Run = %v, want %v", err, boom)
	}
	select {
	case <-otherCancelled:
	default:
		t.Fatal("other component was not cancelled before Run returned")
	}
}

func TestShutdownOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string
	rec := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, s)
	}

	comp := lifecycle.Func(func(ctx context.Context) error {
		<-ctx.Done()
		rec("component-stopped")
		return ctx.Err()
	})
	closer := func(name string) func(context.Context) error {
		return func(context.Context) error { rec(name); return nil }
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := runAsync(ctx, lifecycle.Options{
		ShutdownGrace: wait,
		OnShutdown:    []func(){func() { rec("hook") }},
		Closers:       []func(context.Context) error{closer("a"), closer("b")},
	}, comp)
	cancel()

	if err := result(t, done); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	want := []string{"hook", "component-stopped", "b", "a"}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestShutdownDeadline(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	stubborn := lifecycle.Func(func(context.Context) error {
		<-release
		return nil
	})
	var closed bool
	ctx, cancel := context.WithCancel(context.Background())
	done := runAsync(ctx, lifecycle.Options{
		ShutdownGrace: 50 * time.Millisecond,
		Closers:       []func(context.Context) error{func(context.Context) error { closed = true; return nil }},
	}, stubborn)

	start := time.Now()
	cancel()
	err := result(t, done)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("Run returned before the grace period elapsed")
	}
	if !closed {
		t.Fatal("closer did not run after the deadline")
	}
}

func TestComponentReturningNilStopsOthers(t *testing.T) {
	stopped := make(chan struct{})
	other := lifecycle.Func(func(ctx context.Context) error {
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	quick := lifecycle.Func(func(context.Context) error { return nil })

	if err := result(t, runAsync(context.Background(), lifecycle.Options{ShutdownGrace: wait}, other, quick)); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	<-stopped
}

func TestStackClosesReverseOnce(t *testing.T) {
	var order []string
	boom := errors.New("b failed")
	var s lifecycle.Stack
	s.Push(func(context.Context) error { order = append(order, "a"); return nil })
	s.Push(func(context.Context) error { order = append(order, "b"); return boom })

	if err := s.Close(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("first Close = %v, want b's error", err)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
	if len(order) != 2 || order[0] != "b" || order[1] != "a" {
		t.Fatalf("order = %v, want [b a]", order)
	}
}

func TestDrainDelayHoldsComponents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var hookAt time.Time
	cancelledAt := make(chan time.Time, 1)
	comp := lifecycle.Func(func(ctx context.Context) error {
		<-ctx.Done()
		cancelledAt <- time.Now()
		return nil
	})
	done := runAsync(ctx, lifecycle.Options{
		DrainDelay: 100 * time.Millisecond,
		OnShutdown: []func(){func() { hookAt = time.Now() }},
	}, comp)

	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := result(t, done); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gap := (<-cancelledAt).Sub(hookAt); gap < 100*time.Millisecond {
		t.Fatalf("components cancelled %v after the hook, want >= 100ms", gap)
	}
}

func TestStopAfterOrdersShutdown(t *testing.T) {
	var mu sync.Mutex
	var order []string
	mark := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }

	primary := lifecycle.Func(func(ctx context.Context) error {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond) // draining in-flight work
		mark("primary stopped")
		return nil
	})
	dependent := lifecycle.Func(func(ctx context.Context) error {
		if ctx.Err() != nil {
			t.Error("dependent context cancelled together with the parent")
		}
		<-ctx.Done()
		mark("dependent stopped")
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- lifecycle.StopAfter(primary, dependent).Run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := result(t, done); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("StopAfter = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "primary stopped" || order[1] != "dependent stopped" {
		t.Fatalf("shutdown order = %v, want primary then dependent", order)
	}
}
