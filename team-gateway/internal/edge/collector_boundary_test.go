package edge

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
)

// principalCapturingPublisher records the principal passed with each publish.
type principalCapturingPublisher struct {
	principals []*commonv1.Principal
	n          int
}

func (p *principalCapturingPublisher) PublishTrackingEvent(_ context.Context, _ *analyticsv1.TrackingEvent, pr *commonv1.Principal, _, _ string) error {
	p.n++
	p.principals = append(p.principals, pr)
	return nil
}
func (p *principalCapturingPublisher) Close() {}

func batchBody(n int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = `{"type":"view","listingId":"item-` + strings.Repeat("x", i%5) + `","sessionId":"s","anonymousId":"a"}`
	}
	return "[" + strings.Join(items, ",") + "]"
}

// Scenario: Batch of exactly 100 accepted (sibling: 101 rejected in collector_test.go).
func TestBoundaryExactly100BatchAccepted(t *testing.T) {
	pub := &mockAnalyticsPublisher{}
	rec := postTrack(t, &Edge{}, pub, batchBody(100))
	if rec.Code < 200 || rec.Code > 299 {
		t.Fatalf("expected 2xx, got %d", rec.Code)
	}
	if len(pub.events) != 100 {
		t.Fatalf("expected 100 published, got %d", len(pub.events))
	}
}

// Scenario: Anonymous caller allowed.
func TestBoundaryAnonymousCallerAllowed(t *testing.T) {
	pub := &principalCapturingPublisher{}
	h := HandleTrack(&Edge{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/track", strings.NewReader(`{"type":"view","listingId":"l1"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code > 299 {
		t.Fatalf("expected 2xx, got %d", rec.Code)
	}
	if pub.n != 1 {
		t.Fatalf("expected 1 published, got %d", pub.n)
	}
	if ty := pub.principals[0].GetType(); ty != commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS {
		t.Errorf("expected PRINCIPAL_TYPE_ANONYMOUS, got %v", ty)
	}
}

// Scenario: Oversized body rejected without publishing (64 KiB + 1).
func TestBoundaryOversizedBodyRejectedWithoutPublishing(t *testing.T) {
	pub := &mockAnalyticsPublisher{}
	const limit = 64 << 10
	valid := `{"type":"view","listingId":"l1"}`
	body := valid + strings.Repeat(" ", limit+1-len(valid))
	if len(body) != limit+1 {
		t.Fatalf("setup: body %d", len(body))
	}
	rec := postTrack(t, &Edge{}, pub, body)
	if rec.Code < 400 || rec.Code > 499 {
		t.Fatalf("expected 4xx, got %d", rec.Code)
	}
	if len(pub.events) != 0 {
		t.Fatalf("expected 0 published, got %d", len(pub.events))
	}
}

// Scenario: Handler p99 under budget (1000 requests x 100 events, fake publisher).
func TestBoundaryHandlerP99UnderBudget(t *testing.T) {
	const reqs = 1000
	pub := &mockAnalyticsPublisher{}
	h := HandleTrack(&Edge{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := []byte(batchBody(100))
	lat := make([]time.Duration, reqs)
	for i := range lat {
		req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		start := time.Now()
		h.ServeHTTP(rec, req)
		lat[i] = time.Since(start)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("req %d: code %d", i, rec.Code)
		}
	}
	if len(pub.events) != reqs*100 {
		t.Fatalf("expected %d published, got %d", reqs*100, len(pub.events))
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	p99 := lat[reqs*99/100-1]
	if p99 >= 50*time.Millisecond {
		t.Fatalf("p99 %v >= 50ms", p99)
	}
}

func BenchmarkBoundaryTrack100Events(b *testing.B) {
	h := HandleTrack(&Edge{}, &mockAnalyticsPublisher{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := []byte(batchBody(100))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// Scenario: Stamping adds no latency beyond budget (N=1000 batches of 100 events, valid consent cookie).
// Timed p99 over a fake publisher; wall-clock bound is a loose 50 ms budget, so it is stable in CI.
func TestConsentStamp_StampingAddsNoLatencyBeyondBudget(t *testing.T) {
	const reqs = 1000
	pub := &mockAnalyticsPublisher{}
	h := HandleTrack(&Edge{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := []byte(batchBody(100))
	lat := make([]time.Duration, reqs)
	for i := range lat {
		req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
		req.Header.Set("Cookie", "agora_consent=v=1&a=1&m=0&p=1&ts=1760000000")
		rec := httptest.NewRecorder()
		start := time.Now()
		h.ServeHTTP(rec, req)
		lat[i] = time.Since(start)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("req %d: code %d", i, rec.Code)
		}
	}
	if len(pub.events) != reqs*100 {
		t.Fatalf("expected %d published, got %d", reqs*100, len(pub.events))
	}
	// Behavioural guard so the test is red until stamping exists.
	for _, ev := range pub.events[:100] {
		if ev.GetConsent().GetVersion() != 1 || !ev.GetConsent().GetAnalytics() {
			t.Fatalf("event not stamped from cookie")
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	if p99 := lat[reqs*99/100-1]; p99 >= 50*time.Millisecond {
		t.Fatalf("p99 %v >= 50ms", p99)
	}
}
