package ga4relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testSecret = "s3cr3t-api-secret-XYZ"

// sleepRecorder is the injected sleep: records requested delays, never blocks.
type sleepRecorder struct {
	mu sync.Mutex
	d  []time.Duration
}

func (s *sleepRecorder) Sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d = append(s.d, d)
	return nil
}

func (s *sleepRecorder) got() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.d...)
}

type seqServer struct {
	*httptest.Server
	mu     sync.Mutex
	hits   int
	bodies []string
	urls   []string
	ctypes []string
}

// newSeqServer replies with codes[i] to the i-th request (last code repeats).
func newSeqServer(t *testing.T, codes ...int) *seqServer {
	t.Helper()
	s := &seqServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		i := s.hits
		s.hits++
		s.bodies = append(s.bodies, string(b))
		s.urls = append(s.urls, r.URL.String())
		s.ctypes = append(s.ctypes, r.Header.Get("Content-Type"))
		s.mu.Unlock()
		if i >= len(codes) {
			i = len(codes) - 1
		}
		w.WriteHeader(codes[i])
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *seqServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

func newTestClient(endpoint string, sl *sleepRecorder, logs io.Writer) *Client {
	return NewClient(ClientConfig{
		Endpoint:      endpoint,
		MeasurementID: "G-TEST123",
		APISecret:     testSecret,
		Logger:        slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Sleep:         sl.Sleep,
	})
}

func sendErr(t *testing.T, err error) *SendError {
	t.Helper()
	var se *SendError
	if !errors.As(err, &se) {
		t.Fatalf("want *SendError, got %T: %v", err, err)
	}
	return se
}

func durs(s ...int) []time.Duration {
	out := make([]time.Duration, len(s))
	for i, v := range s {
		out[i] = time.Duration(v) * time.Second
	}
	return out
}

func equalDurs(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestClient_TransientFailureThenSuccess(t *testing.T) {
	// Scenario: Transient failure then success
	srv := newSeqServer(t, 503, 503, 204)
	sl := &sleepRecorder{}
	c := newTestClient(srv.URL, sl, io.Discard)
	body := []byte(`{"client_id":"1.2","events":[{"name":"purchase"}]}`)
	if err := c.Send(context.Background(), body); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if srv.count() != 3 {
		t.Fatalf("requests = %d, want 3", srv.count())
	}
	for i, b := range srv.bodies {
		if b != string(body) {
			t.Errorf("attempt %d body = %q, want identical payload", i, b)
		}
		if srv.ctypes[i] != "application/json" {
			t.Errorf("attempt %d content-type = %q", i, srv.ctypes[i])
		}
		if !strings.Contains(srv.urls[i], "measurement_id=G-TEST123") || !strings.Contains(srv.urls[i], "api_secret="+testSecret) {
			t.Errorf("attempt %d url %q lacks measurement_id/api_secret", i, srv.urls[i])
		}
	}
	if got := sl.got(); !equalDurs(got, durs(1, 2)) {
		t.Errorf("sleeps = %v, want [1s 2s]", got)
	}
}

func TestClient_PermanentClientErrorNotRetried(t *testing.T) {
	// Scenario: Permanent client error is not retried
	for _, code := range []int{400, 401, 403, 404} {
		srv := newSeqServer(t, code)
		sl := &sleepRecorder{}
		c := newTestClient(srv.URL, sl, io.Discard)
		err := c.Send(context.Background(), []byte(`{}`))
		if err == nil {
			t.Fatalf("status %d: want error", code)
		}
		se := sendErr(t, err)
		if se.Reason != "4xx" || se.Status != code || se.Attempts != 1 {
			t.Errorf("status %d: got reason=%q status=%d attempts=%d", code, se.Reason, se.Status, se.Attempts)
		}
		if srv.count() != 1 {
			t.Errorf("status %d: requests = %d, want 1", code, srv.count())
		}
		if len(sl.got()) != 0 {
			t.Errorf("status %d: slept %v, want none", code, sl.got())
		}
	}
}

func TestClient_RateLimitedIsRetried(t *testing.T) {
	// Scenario: Rate limited is retried
	srv := newSeqServer(t, 429, 204)
	sl := &sleepRecorder{}
	c := newTestClient(srv.URL, sl, io.Discard)
	if err := c.Send(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if srv.count() != 2 {
		t.Fatalf("requests = %d, want 2", srv.count())
	}
	if got := sl.got(); len(got) != 1 || got[0] < time.Second {
		t.Errorf("sleeps = %v, want one delay >= 1s", got)
	}
}

func TestClient_ServerErrorBackoffSchedule(t *testing.T) {
	// Scenario: Server error backoff schedule
	srv := newSeqServer(t, 500)
	sl := &sleepRecorder{}
	c := newTestClient(srv.URL, sl, io.Discard)
	if err := c.Send(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("want error after exhausting retries")
	}
	if srv.count() != 5 {
		t.Fatalf("requests = %d, want 5", srv.count())
	}
	if got := sl.got(); !equalDurs(got, durs(1, 2, 4, 8)) {
		t.Errorf("sleeps = %v, want [1s 2s 4s 8s]", got)
	}
}

func TestBackoff_CappedAt30s_ServerErrorBackoffSchedule(t *testing.T) {
	// Scenario: Server error backoff schedule (cap: none exceeds 30 s)
	want := durs(1, 2, 4, 8, 16, 30, 30, 30)
	for retry, w := range want {
		if got := Backoff(retry); got != w {
			t.Errorf("Backoff(%d) = %v, want %v", retry, got, w)
		}
	}
}

func TestClient_RetriesExhausted(t *testing.T) {
	// Scenario: Retries exhausted
	srv := newSeqServer(t, 503)
	sl := &sleepRecorder{}
	c := newTestClient(srv.URL, sl, io.Discard)
	err := c.Send(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("want error")
	}
	se := sendErr(t, err)
	if se.Reason != "exhausted" || se.Attempts != 5 || se.Status != 503 {
		t.Errorf("got reason=%q attempts=%d status=%d, want exhausted/5/503", se.Reason, se.Attempts, se.Status)
	}
	if srv.count() != 5 {
		t.Errorf("requests = %d, want 5", srv.count())
	}
}

func TestClient_RetriesExhaustedOnNetworkError(t *testing.T) {
	// Scenario: Retries exhausted (network errors count as attempts)
	srv := newSeqServer(t, 204)
	url := srv.URL
	srv.Close() // connection refused
	sl := &sleepRecorder{}
	c := newTestClient(url, sl, io.Discard)
	err := c.Send(context.Background(), []byte(`{}`))
	se := sendErr(t, err)
	if se.Reason != "exhausted" || se.Attempts != 5 {
		t.Errorf("got reason=%q attempts=%d, want exhausted/5", se.Reason, se.Attempts)
	}
	if got := sl.got(); !equalDurs(got, durs(1, 2, 4, 8)) {
		t.Errorf("sleeps = %v, want [1s 2s 4s 8s]", got)
	}
}

func TestClient_DefaultRequestTimeoutIs5s_RequestTimeoutBoundary(t *testing.T) {
	// Scenario: Request timeout boundary
	if DefaultRequestTimeout != 5*time.Second {
		t.Fatalf("DefaultRequestTimeout = %v, want 5s", DefaultRequestTimeout)
	}
}

func TestClient_TimeoutCountedAsNetworkErrorAndRetried_RequestTimeoutBoundary(t *testing.T) {
	// Scenario: Request timeout boundary
	release := make(chan struct{})
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		n := hits
		mu.Unlock()
		if n == 1 {
			select { // hang past the client timeout
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		w.WriteHeader(204)
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	sl := &sleepRecorder{}
	c := NewClient(ClientConfig{
		Endpoint: srv.URL, MeasurementID: "G-TEST123", APISecret: testSecret,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Sleep:   sl.Sleep,
		Timeout: 50 * time.Millisecond, // test override of the 5 s default
	})
	if err := c.Send(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Errorf("requests = %d, want 2 (timeout then retry)", hits)
	}
	if got := sl.got(); !equalDurs(got, durs(1)) {
		t.Errorf("sleeps = %v, want [1s]", got)
	}
}

func TestClient_ContextCancelStopsRetries(t *testing.T) {
	// Scenario: Retries exhausted (cancellation aborts, no further attempts)
	srv := newSeqServer(t, 503)
	ctx, cancel := context.WithCancel(context.Background())
	c := NewClient(ClientConfig{
		Endpoint: srv.URL, MeasurementID: "G-TEST123", APISecret: testSecret,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Sleep: func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		},
	})
	if err := c.Send(ctx, []byte(`{}`)); err == nil {
		t.Fatal("want error")
	}
	if srv.count() != 1 {
		t.Errorf("requests = %d, want 1", srv.count())
	}
}

func TestClient_SecretNeverLogged(t *testing.T) {
	// Scenario: Secret never logged
	deadSrv := newSeqServer(t, 204)
	deadURL := deadSrv.URL
	deadSrv.Close()
	okSrv5xx := newSeqServer(t, 503)
	okSrv4xx := newSeqServer(t, 403)

	for name, url := range map[string]string{"network": deadURL, "5xx": okSrv5xx.URL, "4xx": okSrv4xx.URL} {
		var logs bytes.Buffer
		sl := &sleepRecorder{}
		c := newTestClient(url, sl, &logs)
		err := c.Send(context.Background(), []byte(`{}`))
		if err == nil {
			t.Fatalf("%s: want error", name)
		}
		if strings.Contains(err.Error(), testSecret) {
			t.Errorf("%s: error string leaks secret: %v", name, err)
		}
		if strings.Contains(logs.String(), testSecret) {
			t.Errorf("%s: logs leak secret: %s", name, logs.String())
		}
		if strings.Contains(logs.String(), "api_secret=") && strings.Contains(logs.String(), testSecret) {
			t.Errorf("%s: logs leak api_secret query", name)
		}
	}
}
