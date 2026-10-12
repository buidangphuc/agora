package main

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/ga4relay"
)

// Required seam (implementer provides):
//
//	type deps struct {
//	    probeListener net.Listener                  // probe server listener; nil => listen on GA4.RelayProbePort
//	    newConsumer   func(ctx context.Context) error // enabled-mode Kafka consumer factory; MUST NOT be called when disabled
//	    httpClient    *http.Client                  // outbound client for GA4; MUST NOT be used when disabled
//	}
//	func run(ctx context.Context, s *config.Settings, logger *slog.Logger, d deps) error // blocks until ctx done
//	func newProbeMux(ready func() bool) http.Handler

func TestProbeMux_NoInboundEndpointBeyondProbes_OtherPaths404(t *testing.T) {
	h := newProbeMux(func() bool { return true })
	for _, p := range []string{"/", "/send", "/metrics-admin", "/metrics", "/healthz/x"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/send", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /send = %d, want 404", rec.Code)
	}
}

func TestProbeMux_NoInboundEndpointBeyondProbes_ProbesServed(t *testing.T) {
	ready := true
	h := newProbeMux(func() bool { return ready })
	get := func(p string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		return rec.Code
	}
	if c := get("/healthz"); c != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", c)
	}
	if c := get("/readyz"); c != http.StatusOK {
		t.Errorf("/readyz ready = %d, want 200", c)
	}
	ready = false
	if c := get("/readyz"); c != http.StatusServiceUnavailable {
		t.Errorf("/readyz not ready = %d, want 503", c)
	}
}

func TestRun_SecretUnset_DisabledLogsOnceReadyNoConsumerNoOutbound(t *testing.T) {
	s := &config.Settings{}
	s.GA4.RelayProbePort = 8085
	if s.GA4RelayEnabled() {
		t.Fatal("precondition: relay must be disabled with no secret")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var consumers, outbound atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		outbound.Add(1)
		return nil, http.ErrNotSupported
	})}
	var buf syncBuf
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, s, logger, deps{
			probeListener: ln,
			newConsumer:   func(context.Context) error { consumers.Add(1); return nil },
			httpClient:    client,
		})
	}()

	base := "http://" + ln.Addr().String()
	deadline := time.Now().Add(5 * time.Second)
	var code int
	for time.Now().Before(deadline) {
		resp, gerr := http.Get(base + "/readyz")
		if gerr == nil {
			code = resp.StatusCode
			resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if code != http.StatusOK {
		t.Fatalf("/readyz in disabled mode = %d, want 200", code)
	}
	resp, err := http.Get(base + "/send")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("/send = %d, want 404", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v on graceful shutdown, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after ctx cancel")
	}
	if n := strings.Count(buf.String(), "ga4 relay disabled"); n != 1 {
		t.Errorf("'ga4 relay disabled' logged %d times, want exactly 1:\n%s", n, buf.String())
	}
	if consumers.Load() != 0 {
		t.Error("consumer factory called in disabled mode; must join no consumer group")
	}
	if outbound.Load() != 0 {
		t.Error("outbound HTTP made in disabled mode")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// Required seam (implementer provides):
//
//	func runPurger(ctx context.Context, interval time.Duration, logger *slog.Logger, purgers ...ga4relay.Purger)
//
// Calls every purger immediately, then every interval; returns when ctx is cancelled.
// On error it logs (counts only, no ids) and keeps going. Blocks until ctx done.

var _ ga4relay.Purger = (*fakePurger)(nil)

type fakePurger struct {
	calls chan struct{}
	n     int64
	err   error
}

func (f *fakePurger) Purge(context.Context) (int64, error) {
	f.calls <- struct{}{}
	return f.n, f.err
}

func waitCall(t *testing.T, p *fakePurger) {
	t.Helper()
	select {
	case <-p.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("purge not called")
	}
}

// Scenario: retention purge - runs at startup (before any tick), then per tick, stops on cancel.
func TestRunPurger_PurgesAtStartupThenPerTick_StopsOnCancel(t *testing.T) {
	a := &fakePurger{calls: make(chan struct{}, 100)}
	b := &fakePurger{calls: make(chan struct{}, 100)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runPurger(ctx, 20*time.Millisecond, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), a, b)
		close(done)
	}()
	waitCall(t, a)
	waitCall(t, b)
	waitCall(t, a) // at least one tick
	waitCall(t, b)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runPurger did not stop on ctx cancel")
	}
}

// Scenario: retention purge - an error is logged without ids and does not stop the loop or other purgers.
func TestRunPurger_ErrorLoggedAndContinues(t *testing.T) {
	bad := &fakePurger{calls: make(chan struct{}, 100), err: context.DeadlineExceeded}
	good := &fakePurger{calls: make(chan struct{}, 100), n: 7}
	var buf syncBuf
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runPurger(ctx, 20*time.Millisecond, slog.New(slog.NewTextHandler(&buf, nil)), bad, good)
		close(done)
	}()
	waitCall(t, bad)
	waitCall(t, good)
	waitCall(t, bad) // loop continued after error
	waitCall(t, good)
	cancel()
	<-done
	if !strings.Contains(buf.String(), "deadline exceeded") {
		t.Errorf("error not logged: %q", buf.String())
	}
}
