package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type funnelSpy struct {
	analyticsv1.AnalyticsQueryServiceClient
	calls int
	md    metadata.MD
}

func (s *funnelSpy) GetChannelFunnel(ctx context.Context, _ *analyticsv1.GetChannelFunnelRequest, _ ...grpc.CallOption) (*analyticsv1.GetChannelFunnelResponse, error) {
	s.calls++
	s.md, _ = metadata.FromOutgoingContext(ctx)
	return &analyticsv1.GetChannelFunnelResponse{Rows: []*analyticsv1.ChannelFunnelRow{
		{Source: "google", Medium: "cpc", Campaign: "c1", Impressions: 10, Views: 5, Adds: 3, BeginCheckouts: 2, Purchases: 1},
	}}, nil
}

func postFunnel(t *testing.T, f *cockpitFixture, spy *funnelSpy, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{AnalyticsQuery: spy}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform.analytics.v1.AnalyticsQueryService/GetChannelFunnel", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

// Scenario "Only admins can read the channel funnel": an admin is forwarded
// upstream and receives the upstream rows.
func TestOnlyAdminsCanReadTheChannelFunnel_AdminForwarded(t *testing.T) {
	f := newCockpitFixture(t)
	spy := &funnelSpy{}
	res, body := postFunnel(t, f, spy, map[string]string{"Authorization": "Bearer " + f.token(t, "admin")})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("admin GetChannelFunnel = %d %s, want 200", res.StatusCode, body)
	}
	if spy.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", spy.calls)
	}
	if !strings.Contains(body, `"google"`) || !strings.Contains(body, `"cpc"`) || !strings.Contains(body, `"c1"`) {
		t.Fatalf("response lacks upstream rows: %s", body)
	}
}

// Scenario "Only admins can read the channel funnel": client-supplied
// x-principal-* headers never reach upstream; the edge rebuilds them from the token.
func TestOnlyAdminsCanReadTheChannelFunnel_SpoofedPrincipalHeadersRebuilt(t *testing.T) {
	f := newCockpitFixture(t)
	spy := &funnelSpy{}
	res, body := postFunnel(t, f, spy, map[string]string{
		"Authorization":      "Bearer " + f.token(t, "admin"),
		"X-Principal-Id":     "attacker",
		"X-Principal-Scopes": "admin,superuser",
		"X-Principal-Type":   "service",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", res.StatusCode, body)
	}
	for k, want := range map[string]string{"x-principal-id": "user-1", "x-principal-scopes": "admin", "x-principal-type": "user"} {
		if got := spy.md.Get(k); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %v, want [%s]", k, got, want)
		}
	}
}

type reportSpy struct {
	analyticsv1.AnalyticsQueryServiceClient
	calls int
	err   error
}

func (s *reportSpy) GetRetentionCohorts(context.Context, *analyticsv1.GetRetentionCohortsRequest, ...grpc.CallOption) (*analyticsv1.GetRetentionCohortsResponse, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &analyticsv1.GetRetentionCohortsResponse{Cohorts: []*analyticsv1.RetentionCohort{{CohortSize: 3}}}, nil
}

func (s *reportSpy) GetExperimentReadout(context.Context, *analyticsv1.GetExperimentReadoutRequest, ...grpc.CallOption) (*analyticsv1.GetExperimentReadoutResponse, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &analyticsv1.GetExperimentReadoutResponse{WindowDays: 7}, nil
}

func (s *reportSpy) GetAiSurfaceReport(context.Context, *analyticsv1.GetAiSurfaceReportRequest, ...grpc.CallOption) (*analyticsv1.GetAiSurfaceReportResponse, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &analyticsv1.GetAiSurfaceReportResponse{Rows: []*analyticsv1.AiSurfaceRow{{Surface: "assistant", Shown: 5}}}, nil
}

var reportRPCs = []string{"GetRetentionCohorts", "GetExperimentReadout", "GetAiSurfaceReport"}

func postReport(t *testing.T, f *cockpitFixture, spy *reportSpy, rpc, auth string) (*http.Response, string) {
	t.Helper()
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{AnalyticsQuery: spy}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform.analytics.v1.AnalyticsQueryService/"+rpc, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

// Scenario: Admin reads each report through the gateway (forwarder level).
func TestExperimentReportForwarder_AdminReadsEachReport(t *testing.T) {
	f := newCockpitFixture(t)
	want := map[string]string{
		"GetRetentionCohorts":  `"cohortSize":3`,
		"GetExperimentReadout": `"windowDays":7`,
		"GetAiSurfaceReport":   `"assistant"`,
	}
	for _, rpc := range reportRPCs {
		spy := &reportSpy{}
		res, body := postReport(t, f, spy, rpc, "Bearer "+f.token(t, "admin"))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("admin %s = %d %s, want 200", rpc, res.StatusCode, body)
		}
		if spy.calls != 1 {
			t.Errorf("%s upstream calls = %d, want 1", rpc, spy.calls)
		}
		if !strings.Contains(strings.ReplaceAll(body, " ", ""), want[rpc]) {
			t.Errorf("%s body %s lacks upstream data %s", rpc, body, want[rpc])
		}
	}
}

// Scenario: Anonymous caller is refused at the edge (401).
// Scenario: Buyer or seller is refused at the edge (403, upstream untouched).
func TestExperimentReportForwarder_EdgeRefusesNonAdmins(t *testing.T) {
	f := newCockpitFixture(t)
	for _, rpc := range reportRPCs {
		for _, c := range []struct {
			name, auth string
			want       int
		}{
			{"anonymous", "", http.StatusUnauthorized},
			{"buyer", "Bearer " + f.token(t, "order.read"), http.StatusForbidden},
			{"seller", "Bearer " + f.token(t, "seller", "listing.write"), http.StatusForbidden},
		} {
			spy := &reportSpy{}
			res, body := postReport(t, f, spy, rpc, c.auth)
			if res.StatusCode != c.want {
				t.Errorf("%s %s = %d %s, want %d", c.name, rpc, res.StatusCode, body, c.want)
			}
			if spy.calls != 0 {
				t.Errorf("%s %s: upstream called %d times, want 0", c.name, rpc, spy.calls)
			}
		}
	}
}

// Scenario: Upstream failure is surfaced, not masked.
func TestExperimentReportForwarder_UnavailablePassesThrough(t *testing.T) {
	f := newCockpitFixture(t)
	for _, rpc := range reportRPCs {
		spy := &reportSpy{err: status.Error(codes.Unavailable, "warehouse down")}
		res, body := postReport(t, f, spy, rpc, "Bearer "+f.token(t, "admin"))
		if res.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s = %d %s, want 503", rpc, res.StatusCode, body)
		}
		if !strings.Contains(body, `"unavailable"`) {
			t.Errorf("%s body %s lacks code unavailable", rpc, body)
		}
		if strings.Contains(body, "cohorts") || strings.Contains(body, "windowDays") || strings.Contains(body, "rows") {
			t.Errorf("%s: partial body %s", rpc, body)
		}
	}
}
