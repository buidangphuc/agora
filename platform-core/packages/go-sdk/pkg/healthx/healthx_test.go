package healthx_test

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
)

const (
	interval = 20 * time.Millisecond
	deadline = 5 * time.Second
)

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func status(t *testing.T, h *healthx.Health) grpc_health_v1.HealthCheckResponse_ServingStatus {
	t.Helper()
	resp, err := h.Server().Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	return resp.GetStatus()
}

func newHealth(t *testing.T) *healthx.Health {
	t.Helper()
	h, err := healthx.New(interval, quiet())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func TestNewRejectsNonPositiveInterval(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		if h, err := healthx.New(d, quiet()); err == nil || h != nil {
			t.Errorf("New(%v) = %v, %v; want nil and an error", d, h, err)
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(deadline)
	for !cond() {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func start(t *testing.T, h *healthx.Health) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run returned %v on cancel, want nil", err)
		}
	})
}

func TestStartsServing(t *testing.T) {
	h := newHealth(t)
	if got := status(t, h); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("initial status = %v, want SERVING", got)
	}
}

func TestCheckerFailureFlipsStatus(t *testing.T) {
	var failing atomic.Bool
	h := newHealth(t)
	h.Register("db", func(context.Context) error {
		if failing.Load() {
			return errors.New("down")
		}
		return nil
	})
	start(t, h)

	failing.Store(true)
	waitFor(t, "NOT_SERVING", func() bool {
		return status(t, h) == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})

	failing.Store(false)
	waitFor(t, "SERVING again", func() bool {
		return status(t, h) == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func TestDrainIsSticky(t *testing.T) {
	h := newHealth(t)
	var probes atomic.Int64
	h.Register("ok", func(context.Context) error { probes.Add(1); return nil })
	start(t, h)

	waitFor(t, "first probe", func() bool { return probes.Load() > 0 })
	h.Drain()

	// Let several further probe rounds pass; none may flip the status back.
	seen := probes.Load()
	waitFor(t, "more probes after drain", func() bool { return probes.Load() >= seen+3 })
	if got := status(t, h); got != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("status after drain = %v, want NOT_SERVING", got)
	}
}

func TestHangingCheckerFailsWithoutBlocking(t *testing.T) {
	h := newHealth(t)
	var cancelled atomic.Bool
	var healthy atomic.Int64
	h.Register("hang", func(ctx context.Context) error {
		<-ctx.Done()
		cancelled.Store(true)
		return ctx.Err()
	})
	h.Register("ok", func(context.Context) error { healthy.Add(1); return nil })
	start(t, h)

	waitFor(t, "NOT_SERVING", func() bool {
		return status(t, h) == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	waitFor(t, "hanging check cancelled", cancelled.Load)

	// The healthy checker keeps running every interval despite the hanging one.
	seen := healthy.Load()
	waitFor(t, "healthy checker still polled", func() bool { return healthy.Load() >= seen+3 })
}

func TestCheckIgnoringContextStillFails(t *testing.T) {
	h := newHealth(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	h.Register("stuck", func(context.Context) error { <-release; return nil })
	start(t, h)

	waitFor(t, "NOT_SERVING", func() bool {
		return status(t, h) == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
}
