package lifecycle_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
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
	// The default CloseGrace reserves a fifth of the grace for the closers.
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("Run returned before the components' share of the grace period elapsed")
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

	primaryCancelled := make(chan struct{})
	finishDraining := make(chan struct{})
	depStarted := make(chan context.Context, 1)
	primary := lifecycle.Func(func(ctx context.Context) error {
		<-ctx.Done()
		close(primaryCancelled)
		<-finishDraining // draining in-flight work
		mark("primary stopped")
		return nil
	})
	dependent := lifecycle.Func(func(ctx context.Context) error {
		depStarted <- ctx
		<-ctx.Done()
		mark("dependent stopped")
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- lifecycle.StopAfter(primary, dependent).Run(ctx) }()
	depCtx := <-depStarted
	cancel()
	<-primaryCancelled
	if depCtx.Err() != nil {
		t.Error("dependent context cancelled together with the parent")
	}
	close(finishDraining)

	if err := result(t, done); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("StopAfter = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "primary stopped" || order[1] != "dependent stopped" {
		t.Fatalf("shutdown order = %v, want primary then dependent", order)
	}
}

func mustEvery(t *testing.T, d time.Duration, fn func(context.Context) error, opts ...lifecycle.EveryOption) lifecycle.Component {
	t.Helper()
	c, err := lifecycle.Every(d, fn, append(opts, lifecycle.WithLogger(quiet()))...)
	if err != nil {
		t.Fatalf("Every: %v", err)
	}
	return c
}

func TestEveryKeepsRunningAfterError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	calls := 0
	second := make(chan struct{})
	c := mustEvery(t, 5*time.Millisecond, func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 2 {
			close(second)
		}
		return errors.New("boom")
	}, lifecycle.RunAtStart())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	select {
	case <-second:
	case <-time.After(wait):
		t.Fatal("fn was not called again after its first error")
	}
	cancel()
	if err := result(t, done); err != nil {
		t.Fatalf("Run = %v, want nil after cancel", err)
	}
}

func TestEveryWaitsForFirstTickWithoutRunAtStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const interval = 50 * time.Millisecond
	called := make(chan time.Time, 1)
	c := mustEvery(t, interval, func(context.Context) error { called <- time.Now(); return nil })
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- c.Run(ctx) }()
	select {
	case at := <-called:
		if at.Sub(start) < interval {
			t.Errorf("fn ran %v after start, want >= %v without RunAtStart", at.Sub(start), interval)
		}
	case <-time.After(wait):
		t.Fatal("fn never ran")
	}
	cancel()
	if err := result(t, done); err != nil {
		t.Fatalf("Run = %v", err)
	}
}

func TestEveryRejectsNonPositiveInterval(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		c, err := lifecycle.Every(d, func(context.Context) error { return nil })
		if err == nil || c != nil {
			t.Errorf("Every(%v) = %v, %v; want nil and an error", d, c, err)
		}
	}
}

func TestShutdownWorstCaseIsOneGrace(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	const grace = 300 * time.Millisecond

	stubborn := lifecycle.Func(func(context.Context) error { <-release; return nil })
	var closerBudget time.Duration
	slowCloser := func(ctx context.Context) error {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Error("closer context has no deadline")
		}
		closerBudget = time.Until(dl)
		<-ctx.Done() // a closer that would run until told to stop
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := runAsync(ctx, lifecycle.Options{
		ShutdownGrace: grace,
		Closers:       []func(context.Context) error{slowCloser},
	}, stubborn)

	start := time.Now()
	cancel()
	_ = result(t, done)
	if total := time.Since(start); total > grace+grace/2 {
		t.Fatalf("shutdown took %v, want about one grace (%v), not two", total, grace)
	}
	if closerBudget > grace/4 {
		t.Fatalf("closer got %v after components used the grace, want the remainder", closerBudget)
	}
}

func TestHTTPShutsDownGracefully(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	started := make(chan struct{})
	shutdownBegan := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("done"))
	}), ReadHeaderTimeout: time.Second}
	srv.RegisterOnShutdown(func() { close(shutdownBegan) })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- lifecycle.HTTP(srv, wait).Run(ctx) }()

	body := make(chan string, 1)
	go func() {
		var resp *http.Response
		var err error
		for range 100 {
			resp, err = http.Get("http://" + addr)
			if err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err != nil {
			body <- "error: " + err.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		body <- string(b)
	}()

	select {
	case <-started:
	case <-time.After(wait):
		t.Fatal("request never reached the handler")
	}
	cancel()
	select {
	case <-shutdownBegan:
	case <-time.After(wait):
		t.Fatal("server shutdown never began")
	}
	select {
	case err := <-done:
		t.Fatalf("Run returned %v while a request was in flight", err)
	default:
	}
	close(release)
	if got := <-body; got != "done" {
		t.Errorf("response = %q, want done", got)
	}
	if err := result(t, done); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

func TestHTTPReturnsListenError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	srv := &http.Server{Addr: l.Addr().String(), ReadHeaderTimeout: time.Second}
	if err := lifecycle.HTTP(srv, time.Second).Run(context.Background()); err == nil {
		t.Fatal("Run = nil on an address in use, want an error")
	}
}

func TestCloseGraceReservesTimeForClosers(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	const grace, reserve = 600 * time.Millisecond, 300 * time.Millisecond

	stubborn := lifecycle.Func(func(context.Context) error { <-release; return nil })
	var closerBudget time.Duration
	closer := func(ctx context.Context) error {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Error("closer context has no deadline")
		}
		closerBudget = time.Until(dl)
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := runAsync(ctx, lifecycle.Options{
		ShutdownGrace: grace,
		CloseGrace:    reserve,
		Closers:       []func(context.Context) error{closer},
	}, stubborn)

	start := time.Now()
	cancel()
	err := result(t, done)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run = %v, want DeadlineExceeded", err)
	}
	if took := time.Since(start); took > grace+grace/2 {
		t.Fatalf("shutdown took %v, want about %v", took, grace)
	}
	if closerBudget < reserve*2/3 {
		t.Fatalf("closer got %v, want about the %v reserve", closerBudget, reserve)
	}
}

func TestDefaultCloseGraceKeepsClosersAlive(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	stubborn := lifecycle.Func(func(context.Context) error { <-release; return nil })
	var closerErr error
	ctx, cancel := context.WithCancel(context.Background())
	done := runAsync(ctx, lifecycle.Options{
		ShutdownGrace: time.Second,
		Closers:       []func(context.Context) error{func(ctx context.Context) error { closerErr = ctx.Err(); return nil }},
	}, stubborn)
	cancel()
	_ = result(t, done)
	if closerErr != nil {
		t.Fatalf("closer context already done (%v) after the components used the grace", closerErr)
	}
}
