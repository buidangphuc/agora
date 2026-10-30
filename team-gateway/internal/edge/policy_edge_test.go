package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	auditv1 "github.com/buidangphuc/team-gateway/generated/platform/audit/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type queryAudit struct {
	auditv1.AuditServiceClient
	calls int
}

func (q *queryAudit) QueryAuditLog(context.Context, *auditv1.QueryAuditLogRequest, ...grpc.CallOption) (*auditv1.QueryAuditLogResponse, error) {
	q.calls++
	return &auditv1.QueryAuditLogResponse{}, nil
}

// QueryAuditLog is admin-only at the edge: an anonymous caller is refused (401)
// by the interceptor before any upstream call.
func TestQueryAuditLogAnonymousRejectedAtEdge(t *testing.T) {
	up := &queryAudit{}
	e := edge.NewEdge(nil, []string{"listing.read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Audit: up}, e, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)

	res, err := srv.Client().Post(srv.URL+"/platform.audit.v1.AuditService/QueryAuditLog", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("anonymous QueryAuditLog = %d %s, want 401", res.StatusCode, b)
	}
	if up.calls != 0 {
		t.Fatalf("upstream called %d times, want 0", up.calls)
	}
}

type countingAnalytics struct {
	analyticsv1.AnalyticsQueryServiceClient
	calls int
}

func (c *countingAnalytics) GetChannelFunnel(context.Context, *analyticsv1.GetChannelFunnelRequest, ...grpc.CallOption) (*analyticsv1.GetChannelFunnelResponse, error) {
	c.calls++
	return &analyticsv1.GetChannelFunnelResponse{}, nil
}

// Scenario "Only admins can read the channel funnel": anonymous -> 401,
// authenticated non-admin -> 403, and neither reaches the upstream.
func TestOnlyAdminsCanReadTheChannelFunnel_EdgeRejects(t *testing.T) {
	f := newCockpitFixture(t)
	for _, c := range []struct {
		name string
		auth string
		want int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"non-admin", "Bearer " + f.token(t, "order.read"), http.StatusForbidden},
	} {
		t.Run(c.name, func(t *testing.T) {
			up := &countingAnalytics{}
			srv := httptest.NewServer(edge.NewMux(&upstream.Clients{AnalyticsQuery: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
			t.Cleanup(srv.Close)
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform.analytics.v1.AnalyticsQueryService/GetChannelFunnel", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			if c.auth != "" {
				req.Header.Set("Authorization", c.auth)
			}
			res, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != c.want {
				b, _ := io.ReadAll(res.Body)
				t.Fatalf("%s GetChannelFunnel = %d %s, want %d", c.name, res.StatusCode, b, c.want)
			}
			if up.calls != 0 {
				t.Fatalf("upstream called %d times, want 0", up.calls)
			}
		})
	}
}

// Scenario: Edge: Seller token gets 403 on moderation RPCs.
func TestPolicyEdgeModerationAdminOnly(t *testing.T) {
	for _, path := range []string{moderateListingPath, listModerationQueuePath} {
		up := &moderationUpstream{}
		code, body := callModeration(t, up, func(*cockpitFixture) string { return path }, func(f *cockpitFixture) string {
			return "Bearer " + f.token(t, "listing:write")
		})
		if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") {
			t.Fatalf("seller %s = %d %s, want 403 permission_denied", path, code, body)
		}
		if n := up.moderate.Load() + up.queue.Load(); n != 0 {
			t.Fatalf("seller %s reached upstream %d times, want 0", path, n)
		}
	}
}

// User-enforcement edge: the AccountAdmin procedures are admin-gated at the edge.
func accountAdminDenied(t *testing.T, auth func(*cockpitFixture) string, want int) {
	t.Helper()
	f := newCockpitFixture(t)
	for _, path := range []string{listUsersPath, setUserStatusPath} {
		up := &accountAdminUpstream{}
		srv := httptest.NewServer(edge.NewMux(&upstream.Clients{AccountAdmin: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
		t.Cleanup(srv.Close)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(`{"user_id":"u-9","status":"USER_STATUS_BANNED","reason_code":"fraud"}`))
		req.Header.Set("Content-Type", "application/json")
		if a := auth(f); a != "" {
			req.Header.Set("Authorization", a)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("%s = %d %s, want %d", path, res.StatusCode, b, want)
		}
		if n := up.setCalls.Load() + up.listCalls.Load(); n != 0 {
			t.Errorf("%s: team-identity received %d requests, want 0", path, n)
		}
	}
}

// Scenario: Edge: Anonymous gets 401 on moderation RPCs.
func TestPolicyEdgeModerationAnonymousUnauthenticated(t *testing.T) {
	up := &moderationUpstream{}
	code, body := callModeration(t, up, func(*cockpitFixture) string { return moderateListingPath }, func(*cockpitFixture) string { return "" })
	if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
		t.Fatalf("anonymous ModerateListing = %d %s, want 401 unauthenticated", code, body)
	}
	if n := up.moderate.Load(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

// Scenario: User enforcement edge: Anonymous caller is rejected at the edge.
func TestAccountAdminAnonymousUnauthenticated(t *testing.T) {
	accountAdminDenied(t, func(*cockpitFixture) string { return "" }, http.StatusUnauthorized)
}

// Scenario: User enforcement edge: Seller without admin scope is denied.
func TestAccountAdminNonAdminPermissionDenied(t *testing.T) {
	accountAdminDenied(t, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "listing.read", "listing.write")
	}, http.StatusForbidden)
}
