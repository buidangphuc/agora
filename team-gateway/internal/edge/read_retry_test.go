package edge_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/buidangphuc/team-gateway/generated/platform/ai/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/ai/v1/aiv1connect"
	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type scriptedSearch struct {
	searchv1.SearchServiceClient
	calls   atomic.Int32
	script  []error
	started chan struct{}
	block   bool
}

func (s *scriptedSearch) SearchListings(ctx context.Context, _ *searchv1.SearchListingsRequest, _ ...grpc.CallOption) (*searchv1.SearchListingsResponse, error) {
	n := int(s.calls.Add(1))
	if s.block {
		select {
		case s.started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, status.Error(codes.Unavailable, "connection closed")
	}
	if n <= len(s.script) {
		return &searchv1.SearchListingsResponse{}, s.script[n-1]
	}
	return &searchv1.SearchListingsResponse{}, nil
}

type downOrder struct {
	orderv1.OrderServiceClient
	calls atomic.Int32
}

func (f *downOrder) CreateOrder(context.Context, *orderv1.CreateOrderRequest, ...grpc.CallOption) (*orderv1.CreateOrderResponse, error) {
	f.calls.Add(1)
	return nil, status.Error(codes.Unavailable, "down")
}

type downAI struct {
	aiv1.AIServiceClient
	calls atomic.Int32
}

func (f *downAI) MagicListing(context.Context, *aiv1.MagicListingRequest, ...grpc.CallOption) (*aiv1.MagicListingResponse, error) {
	f.calls.Add(1)
	return nil, status.Error(codes.Unavailable, "down")
}

type retryFixture struct {
	srv  *httptest.Server
	logs *syncBuf
}

func newRetryFixture(t *testing.T, retryMax int, clients *upstream.Clients) *retryFixture {
	t.Helper()
	logs := &syncBuf{}
	e := edge.NewEdge(nil, []string{"search:read"}, 2*time.Second, retryMax, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(clients, e, nil, edge.CockpitConfig{}, slog.New(slog.NewJSONHandler(logs, nil))))
	t.Cleanup(srv.Close)
	return &retryFixture{srv: srv, logs: logs}
}

func (f *retryFixture) edgeRequestLine(t *testing.T, rid string) map[string]any {
	t.Helper()
	for _, l := range strings.Split(strings.TrimSpace(f.logs.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["msg"] == "edge.request" && m["request_id"] == rid {
			return m
		}
	}
	t.Fatalf("no edge.request line for %q in:\n%s", rid, f.logs.String())
	return nil
}

func searchCall(ctx context.Context, f *retryFixture, rid string) error {
	c := searchv1connect.NewSearchServiceClient(f.srv.Client(), f.srv.URL)
	req := connect.NewRequest(&searchv1.SearchListingsRequest{})
	req.Header().Set("X-Request-Id", rid)
	_, err := c.SearchListings(ctx, req)
	return err
}

func unavailable() error { return status.Error(codes.Unavailable, "down") }

// Scenario: A read retries Unavailable up to RETRY_MAX.
func TestCallReadRetriesUnavailableUpToRetryMax(t *testing.T) {
	up := &scriptedSearch{script: []error{unavailable(), unavailable(), nil}}
	f := newRetryFixture(t, 2, &upstream.Clients{Search: up})
	if err := searchCall(context.Background(), f, "rt-1"); err != nil {
		t.Fatalf("call: %v", err)
	}
	if n := up.calls.Load(); n != 3 {
		t.Fatalf("upstream calls = %d, want 3", n)
	}
	if got := f.edgeRequestLine(t, "rt-1")["attempts"]; got != float64(3) {
		t.Fatalf("attempts = %v, want 3", got)
	}
}

// Scenario: A persistent outage stops after RETRY_MAX plus one attempts.
func TestCallReadStopsAfterRetryMaxPlusOne(t *testing.T) {
	up := &scriptedSearch{script: []error{unavailable(), unavailable(), unavailable(), unavailable(), unavailable()}}
	f := newRetryFixture(t, 2, &upstream.Clients{Search: up})
	err := searchCall(context.Background(), f, "rt-2")
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("code = %v (%v), want unavailable", connect.CodeOf(err), err)
	}
	if n := up.calls.Load(); n != 3 {
		t.Fatalf("upstream calls = %d, want exactly 3", n)
	}
}

// Scenario: A business error is not retried (NotFound, DeadlineExceeded, ResourceExhausted).
func TestCallReadRetriesOnlyUnavailable(t *testing.T) {
	for name, tc := range map[string]struct {
		code codes.Code
		want connect.Code
	}{
		"not found":          {codes.NotFound, connect.CodeNotFound},
		"deadline exceeded":  {codes.DeadlineExceeded, connect.CodeDeadlineExceeded},
		"resource exhausted": {codes.ResourceExhausted, connect.CodeResourceExhausted},
	} {
		t.Run(name, func(t *testing.T) {
			up := &scriptedSearch{script: []error{status.Error(tc.code, "x"), status.Error(tc.code, "x"), status.Error(tc.code, "x")}}
			f := newRetryFixture(t, 2, &upstream.Clients{Search: up})
			err := searchCall(context.Background(), f, "rt-3")
			if connect.CodeOf(err) != tc.want {
				t.Fatalf("code = %v (%v), want %v", connect.CodeOf(err), err, tc.want)
			}
			if n := up.calls.Load(); n != 1 {
				t.Fatalf("upstream calls = %d, want 1", n)
			}
		})
	}
}

// Scenario: A caller deadline or cancellation is not retried.
func TestCallerDeadlineAndCancelAreNotRetried(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		up := &scriptedSearch{block: true, started: make(chan struct{}, 1)}
		f := newRetryFixture(t, 2, &upstream.Clients{Search: up})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- searchCall(ctx, f, "rt-4") }()
		<-up.started
		at := time.Now()
		cancel()
		if err := <-done; err == nil {
			t.Fatal("cancelled call succeeded")
		}
		if d := time.Since(at); d > time.Second {
			t.Fatalf("returned %v after cancel, want < 1s", d)
		}
		f.srv.Close()
		if n := up.calls.Load(); n != 1 {
			t.Fatalf("upstream calls = %d, want 1", n)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		up := &scriptedSearch{block: true, started: make(chan struct{}, 1)}
		f := newRetryFixture(t, 2, &upstream.Clients{Search: up})
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		at := time.Now()
		if err := searchCall(ctx, f, "rt-5"); err == nil {
			t.Fatal("call past its deadline succeeded")
		}
		if d := time.Since(at); d > time.Second {
			t.Fatalf("returned %v after deadline, want < 1s", d)
		}
		f.srv.Close()
		if n := up.calls.Load(); n != 1 {
			t.Fatalf("upstream calls = %d, want 1", n)
		}
	})
}

// Scenario: A failing upstream is called on every read (no circuit breaker).
func TestNoBreakerEveryReadReachesUpstream(t *testing.T) {
	script := make([]error, 30)
	for i := range script {
		script[i] = unavailable()
	}
	up := &scriptedSearch{script: script}
	f := newRetryFixture(t, 0, &upstream.Clients{Search: up})
	const reads = 20
	for i := 0; i < reads; i++ {
		if connect.CodeOf(searchCall(context.Background(), f, "rt-6")) != connect.CodeUnavailable {
			t.Fatalf("read %d: want unavailable", i)
		}
	}
	if n := up.calls.Load(); n != reads {
		t.Fatalf("upstream calls = %d, want %d (every read must reach the upstream)", n, reads)
	}
}

// Scenario: A write and an AI call make one attempt.
func TestWriteAndAICallsAreSingleAttempt(t *testing.T) {
	ord, ai := &downOrder{}, &downAI{}
	f := newRetryFixture(t, 2, &upstream.Clients{Order: ord, AI: ai})

	oreq := connect.NewRequest(&orderv1.CreateOrderRequest{})
	oreq.Header().Set("X-Request-Id", "rt-w")
	_, err := orderv1connect.NewOrderServiceClient(f.srv.Client(), f.srv.URL).CreateOrder(context.Background(), oreq)
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("write: code = %v (%v)", connect.CodeOf(err), err)
	}
	areq := connect.NewRequest(&aiv1.MagicListingRequest{})
	areq.Header().Set("X-Request-Id", "rt-a")
	_, err = aiv1connect.NewAIServiceClient(f.srv.Client(), f.srv.URL).MagicListing(context.Background(), areq)
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("ai: code = %v (%v)", connect.CodeOf(err), err)
	}
	if ord.calls.Load() != 1 || ai.calls.Load() != 1 {
		t.Fatalf("upstream calls: order=%d ai=%d, want 1 each", ord.calls.Load(), ai.calls.Load())
	}
	for _, rid := range []string{"rt-w", "rt-a"} {
		if got := f.edgeRequestLine(t, rid)["attempts"]; got != float64(1) {
			t.Errorf("%s: attempts = %v, want 1", rid, got)
		}
	}
}
