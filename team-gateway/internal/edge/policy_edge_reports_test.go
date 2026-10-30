package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	engagementv1 "github.com/buidangphuc/team-gateway/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const (
	createReportPath  = "/platform.engagement.v1.EngagementService/CreateReport"
	listReportsPath   = "/platform.engagement.v1.EngagementService/ListReports"
	resolveReportPath = "/platform.engagement.v1.EngagementService/ResolveReport"
)

type reportUpstream struct {
	engagementv1.EngagementServiceClient
	createCalls, listCalls, resolveCalls atomic.Int64
	err                                  error

	mu       sync.Mutex
	lastMD   metadata.MD
	lastBody *engagementv1.CreateReportRequest
}

func (r *reportUpstream) CreateReport(ctx context.Context, in *engagementv1.CreateReportRequest, _ ...grpc.CallOption) (*engagementv1.CreateReportResponse, error) {
	r.createCalls.Add(1)
	md, _ := metadata.FromOutgoingContext(ctx)
	r.mu.Lock()
	r.lastMD, r.lastBody = md, in
	r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return &engagementv1.CreateReportResponse{}, nil
}

func (r *reportUpstream) ListReports(context.Context, *engagementv1.ListReportsRequest, ...grpc.CallOption) (*engagementv1.ListReportsResponse, error) {
	r.listCalls.Add(1)
	if r.err != nil {
		return nil, r.err
	}
	return &engagementv1.ListReportsResponse{}, nil
}

func (r *reportUpstream) ResolveReport(context.Context, *engagementv1.ResolveReportRequest, ...grpc.CallOption) (*engagementv1.ResolveReportResponse, error) {
	r.resolveCalls.Add(1)
	if r.err != nil {
		return nil, r.err
	}
	return &engagementv1.ResolveReportResponse{}, nil
}

func callReportRPC(t *testing.T, up *reportUpstream, path, body string, auth func(f *cockpitFixture) string) (int, string) {
	t.Helper()
	f := newCockpitFixture(t)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Engagement: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
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

func anonAuth(*cockpitFixture) string { return "" }

const createBody = `{"target_type":"REPORT_TARGET_TYPE_LISTING","target_id":"11111111-1111-1111-1111-111111111111","reason_code":"spam"}`

// Scenario "Listing reports: Anonymous caller cannot list reports at the edge".
func TestListReportsAnonymousUnauthenticated(t *testing.T) {
	up := &reportUpstream{}
	code, body := callReportRPC(t, up, listReportsPath, `{}`, anonAuth)
	if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
		t.Fatalf("anonymous ListReports = %d %s, want 401 unauthenticated", code, body)
	}
	if n := up.listCalls.Load(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

// Scenario "Listing reports: Non-admin is refused at the edge".
func TestReportAdminProceduresNonAdminPermissionDenied(t *testing.T) {
	for _, c := range []struct{ name, path, body string }{
		{"ListReports", listReportsPath, `{}`},
		{"ResolveReport", resolveReportPath, `{"report_id":"r-1","status":"REPORT_STATUS_ACTIONED","resolution_note":"ok"}`},
	} {
		up := &reportUpstream{}
		code, body := callReportRPC(t, up, c.path, c.body, func(f *cockpitFixture) string {
			return "Bearer " + f.token(t, "engagement:write")
		})
		if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") {
			t.Fatalf("buyer %s = %d %s, want 403 permission_denied", c.name, code, body)
		}
		if n := up.listCalls.Load() + up.resolveCalls.Load(); n != 0 {
			t.Fatalf("%s upstream calls = %d, want 0", c.name, n)
		}
	}
}

// Scenario "Listing reports: Admin close is forwarded once".
func TestResolveReportForwardedOnceNoRetry(t *testing.T) {
	up := &reportUpstream{err: status.Error(codes.Unavailable, "down")}
	code, body := callReportRPC(t, up, resolveReportPath, `{"report_id":"r-1","status":"REPORT_STATUS_ACTIONED","resolution_note":"ok"}`, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "admin")
	})
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "unavailable") {
		t.Fatalf("admin ResolveReport = %d %s, want 503 unavailable", code, body)
	}
	if n := up.resolveCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want exactly 1", n)
	}
}

// Scenario "Listing reports: CreateReport is forwarded once with the caller identity".
func TestCreateReportForwardedOnceWithPrincipal(t *testing.T) {
	up := &reportUpstream{err: status.Error(codes.Unavailable, "down")}
	code, body := callReportRPC(t, up, createReportPath, createBody, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "engagement:write")
	})
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "unavailable") {
		t.Fatalf("buyer CreateReport = %d %s, want 503 unavailable", code, body)
	}
	if n := up.createCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want exactly 1", n)
	}
	up.mu.Lock()
	defer up.mu.Unlock()
	if got := up.lastMD.Get("x-principal-id"); len(got) != 1 || got[0] != "user-1" {
		t.Fatalf("x-principal-id = %v, want [user-1]", got)
	}
	if got := up.lastMD.Get("x-principal-type"); len(got) != 1 || got[0] != "user" {
		t.Fatalf("x-principal-type = %v, want [user]", got)
	}
	if got := up.lastMD.Get("x-principal-scopes"); len(got) != 1 || !strings.Contains(got[0], "engagement:write") {
		t.Fatalf("x-principal-scopes = %v, want to contain engagement:write", got)
	}
}

// Scenario "Listing reports: Anonymous CreateReport is not rejected by the edge policy".
func TestCreateReportAnonymousAnsweredByService(t *testing.T) {
	up := &reportUpstream{err: status.Error(codes.Unauthenticated, "sign in")}
	code, body := callReportRPC(t, up, createReportPath, createBody, anonAuth)
	if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
		t.Fatalf("anonymous CreateReport = %d %s, want 401 unauthenticated", code, body)
	}
	if n := up.createCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1 (service answers, edge does not reject)", n)
	}
}

// Scenario "Listing reports: Report RPCs are not left unrouted".
func TestUnroutedGuardCoversReportRPCs(t *testing.T) {
	admin := func(f *cockpitFixture) string { return "Bearer " + f.token(t, "admin") }
	for _, c := range []struct {
		name, path, body string
		calls            func(*reportUpstream) int64
	}{
		{"CreateReport", createReportPath, createBody, func(u *reportUpstream) int64 { return u.createCalls.Load() }},
		{"ListReports", listReportsPath, `{}`, func(u *reportUpstream) int64 { return u.listCalls.Load() }},
		{"ResolveReport", resolveReportPath, `{"report_id":"r-1","status":"REPORT_STATUS_DISMISSED","resolution_note":"ok"}`, func(u *reportUpstream) int64 { return u.resolveCalls.Load() }},
	} {
		up := &reportUpstream{}
		code, body := callReportRPC(t, up, c.path, c.body, admin)
		if code != http.StatusOK {
			t.Fatalf("admin %s = %d %s, want forwarded 200 (not 501)", c.name, code, body)
		}
		if n := c.calls(up); n != 1 {
			t.Fatalf("%s upstream calls = %d, want 1", c.name, n)
		}
	}
}
