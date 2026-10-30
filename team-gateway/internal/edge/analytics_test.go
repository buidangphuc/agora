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
	"google.golang.org/grpc/metadata"

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
