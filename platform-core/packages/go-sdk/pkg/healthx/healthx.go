package healthx

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Check reports whether one dependency is reachable. A nil error means healthy.
type Check func(ctx context.Context) error

type namedCheck struct {
	name  string
	check Check
}

// Health wraps a gRPC health server whose overall ("") status follows the
// registered checks.
type Health struct {
	interval time.Duration
	logger   *slog.Logger
	server   *health.Server

	mu       sync.Mutex
	checks   []namedCheck
	draining bool
	serving  bool
}

// New returns a Health that probes every interval. It starts SERVING.
func New(interval time.Duration, logger *slog.Logger) *Health {
	if logger == nil {
		logger = slog.Default()
	}
	return &Health{
		interval: interval,
		logger:   logger,
		server:   health.NewServer(),
		serving:  true,
	}
}

// Register adds a named check. Call it before Run.
func (h *Health) Register(name string, c Check) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks = append(h.checks, namedCheck{name: name, check: c})
}

// Server returns the gRPC health server to register on a grpc.Server.
func (h *Health) Server() *health.Server { return h.server }

// Run probes immediately and then every interval until ctx is done.
func (h *Health) Run(ctx context.Context) error {
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		h.apply(h.probe(ctx))
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// Drain sets the status to NOT_SERVING permanently.
func (h *Health) Drain() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.draining = true
	h.server.Shutdown()
	if h.serving {
		h.serving = false
		h.logger.Info("health: draining, status NOT_SERVING")
	}
}

// probe runs every check concurrently, each bounded by the interval, and
// returns the names of the failed ones. A check that ignores its context is
// counted as failed once the interval has elapsed.
func (h *Health) probe(ctx context.Context) []string {
	h.mu.Lock()
	checks := append([]namedCheck(nil), h.checks...)
	h.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, h.interval)
	defer cancel()

	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(checks))
	for _, c := range checks {
		go func() { results <- result{c.name, c.check(ctx)} }()
	}

	pending := make(map[string]struct{}, len(checks))
	for _, c := range checks {
		pending[c.name] = struct{}{}
	}
	var failed []string
	for range checks {
		select {
		case r := <-results:
			delete(pending, r.name)
			if r.err != nil {
				failed = append(failed, r.name)
			}
		case <-ctx.Done():
			for name := range pending {
				failed = append(failed, name)
			}
			sort.Strings(failed)
			return failed
		}
	}
	sort.Strings(failed)
	return failed
}

func (h *Health) apply(failed []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.draining {
		return
	}
	ok := len(failed) == 0
	if ok != h.serving {
		if ok {
			h.logger.Info("health: dependencies recovered, status SERVING")
		} else {
			h.logger.Warn("health: dependency check failed, status NOT_SERVING", "failed", failed)
		}
		h.serving = ok
	}
	status := grpc_health_v1.HealthCheckResponse_SERVING
	if !ok {
		status = grpc_health_v1.HealthCheckResponse_NOT_SERVING
	}
	h.server.SetServingStatus("", status)
}
