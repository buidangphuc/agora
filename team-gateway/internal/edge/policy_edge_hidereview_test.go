package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	engagementv1 "github.com/buidangphuc/team-gateway/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const hideReviewPath = "/platform.engagement.v1.EngagementService/HideReview"

type hideReviewUpstream struct {
	engagementv1.EngagementServiceClient
	calls atomic.Int64
	err   error
}

func (h *hideReviewUpstream) HideReview(context.Context, *engagementv1.HideReviewRequest, ...grpc.CallOption) (*engagementv1.HideReviewResponse, error) {
	h.calls.Add(1)
	if h.err != nil {
		return nil, h.err
	}
	return &engagementv1.HideReviewResponse{}, nil
}

func callHideReview(t *testing.T, up *hideReviewUpstream, auth func(f *cockpitFixture) string) (int, string) {
	t.Helper()
	f := newCockpitFixture(t)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Engagement: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+hideReviewPath, strings.NewReader(`{"review_id":"r-1"}`))
	req.Header.Set("Content-Type", "application/json")
	if a := auth(f); a != "" {
		req.Header.Set("Authorization", a)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Scenario "Anonymous caller is refused at the edge".
func TestHideReviewAnonymousUnauthenticated(t *testing.T) {
	up := &hideReviewUpstream{}
	code, body := callHideReview(t, up, func(*cockpitFixture) string { return "" })
	if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
		t.Fatalf("anonymous HideReview = %d %s, want 401 unauthenticated", code, body)
	}
	if n := up.calls.Load(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

// Scenario "Non-admin is refused at the edge".
func TestHideReviewNonAdminPermissionDenied(t *testing.T) {
	up := &hideReviewUpstream{}
	code, body := callHideReview(t, up, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "engagement:write")
	})
	if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") {
		t.Fatalf("buyer HideReview = %d %s, want 403 permission_denied", code, body)
	}
	if n := up.calls.Load(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

// Scenario "Admin call is forwarded once": write path, no retry on Unavailable.
func TestHideReviewForwardedOnceNoRetry(t *testing.T) {
	up := &hideReviewUpstream{err: status.Error(codes.Unavailable, "down")}
	code, body := callHideReview(t, up, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "admin")
	})
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "unavailable") {
		t.Fatalf("admin HideReview = %d %s, want 503 unavailable", code, body)
	}
	if n := up.calls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want exactly 1", n)
	}
}

// Scenario "HideReview is not left unrouted": an admin call is served by the
// forwarder (not the 501 Unimplemented default) and reaches the upstream.
func TestUnroutedGuardCoversHideReview(t *testing.T) {
	up := &hideReviewUpstream{}
	code, body := callHideReview(t, up, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "admin")
	})
	if code != http.StatusOK {
		t.Fatalf("admin HideReview = %d %s, want forwarded 200 (not 501)", code, body)
	}
	if n := up.calls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}
