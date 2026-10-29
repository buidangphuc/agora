package edge_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
)

type scriptedPub struct {
	calls atomic.Int32
	err   error
	block bool
	stop  chan struct{}
}

func (p *scriptedPub) PublishTrackingEvent(ctx context.Context, _ *analyticsv1.TrackingEvent, _ *commonv1.Principal, _, _ string) error {
	p.calls.Add(1)
	if p.block {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.stop:
			return errors.New("released by test")
		}
	}
	return p.err
}

func (p *scriptedPub) Close() {}

func postBeacon(t *testing.T, e *edge.Edge, pub *scriptedPub, body string, logs *syncBuf) *httptest.ResponseRecorder {
	t.Helper()
	h := edge.HandleTrack(e, pub, slog.New(slog.NewJSONHandler(logs, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/track", strings.NewReader(body))
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.ServeHTTP(rec, req); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(pub.stop)
		t.Fatal("beacon not answered within 1s")
	}
	return rec
}

const viewBeacon = `{"type":"view","listingId":"lst-1","sessionId":"s-1"}`

// Scenario: A produce failure still answers 202.
func TestTrackAcceptsBeaconWhenPublishFails(t *testing.T) {
	logs := &syncBuf{}
	pub := &scriptedPub{err: errors.New("broker down"), stop: make(chan struct{})}
	rec := postBeacon(t, edge.NewEdge(nil, nil, time.Second, 0, 1000, 1000), pub, viewBeacon, logs)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if pub.calls.Load() != 1 {
		t.Fatalf("publisher calls = %d, want 1", pub.calls.Load())
	}
	out := logs.String()
	if !strings.Contains(out, `"level":"WARN"`) || !strings.Contains(out, `"msg":"publish tracking event"`) {
		t.Fatalf("want a warn line 'publish tracking event', got:\n%s", out)
	}
}

// Scenario: A stuck broker cannot hold the beacon.
func TestTrackPublishIsBoundedByTimeout(t *testing.T) {
	logs := &syncBuf{}
	pub := &scriptedPub{block: true, stop: make(chan struct{})}
	e := edge.NewEdge(nil, nil, time.Second, 0, 1000, 1000).WithTrackPublishTimeout(200 * time.Millisecond)
	rec := postBeacon(t, e, pub, viewBeacon, logs)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if !strings.Contains(logs.String(), "context deadline exceeded") {
		t.Fatalf("publisher context error not logged:\n%s", logs.String())
	}
}

// Scenario: A malformed beacon is rejected without a publish.
func TestMalformedBeaconIsRejectedWithoutPublish(t *testing.T) {
	logs := &syncBuf{}
	pub := &scriptedPub{stop: make(chan struct{})}
	rec := postBeacon(t, edge.NewEdge(nil, nil, time.Second, 0, 1000, 1000), pub, `{"type":"teleport","sessionId":"s"}`, logs)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if pub.calls.Load() != 0 {
		t.Fatalf("publisher called %d times, want 0", pub.calls.Load())
	}
}
