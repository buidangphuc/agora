package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/rpc"
)

const driverText = "duckdb: IO Error /data/analytics.duckdb"

// Scenario: A seller reads their own funnel
func TestSellerReadsOwnFunnel(t *testing.T) {
	repo := &spyRepo{funnel: query.Funnel{Impressions: 7, Views: 5, Adds: 3, Orders: 2}}
	resp, err := mustHandler(repo).GetSellerFunnel(asPrincipal("s1", "user", "seller"), &analyticsv1.GetSellerFunnelRequest{SellerId: "s1"})
	if err != nil {
		t.Fatalf("GetSellerFunnel: %v", err)
	}
	if resp.GetImpressions() != 7 || resp.GetViews() != 5 || resp.GetAdds() != 3 || resp.GetOrders() != 2 {
		t.Errorf("funnel = %+v, want 7/5/3/2", resp)
	}
	if repo.calls != 1 {
		t.Errorf("repository calls = %d, want 1", repo.calls)
	}
}

// Scenario: A seller cannot read another seller's revenue
func TestSellerCannotReadAnotherSellersRevenue(t *testing.T) {
	repo := &spyRepo{}
	_, err := mustHandler(repo).GetRevenueBreakdown(asPrincipal("s1", "user", "seller"), &analyticsv1.GetRevenueBreakdownRequest{SellerId: "s2"})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", status.Code(err))
	}
	if repo.calls != 0 {
		t.Errorf("repository calls = %d, want 0", repo.calls)
	}
}

// Scenario: An anonymous caller is unauthenticated
func TestAnonymousCallerIsUnauthenticated(t *testing.T) {
	repo := &spyRepo{}
	h := mustHandler(repo)
	req := &analyticsv1.GetDemandForecastRequest{SellerId: "s1", ListingId: "l1"}
	for name, ctx := range map[string]context.Context{
		"anonymous principal": asPrincipal("anonymous", "anonymous", ""),
		"no principal":        context.Background(),
	} {
		if _, err := h.GetDemandForecast(ctx, req); status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: code = %v, want Unauthenticated", name, status.Code(err))
		}
	}
	if repo.calls != 0 {
		t.Errorf("repository calls = %d, want 0", repo.calls)
	}
}

// Scenario: An admin reads any seller
func TestAdminReadsAnySeller(t *testing.T) {
	repo := &spyRepo{funnel: query.Funnel{Views: 4}}
	resp, err := mustHandler(repo).GetSellerFunnel(asPrincipal("a1", "user", "admin"), &analyticsv1.GetSellerFunnelRequest{SellerId: "s9"})
	if err != nil {
		t.Fatalf("GetSellerFunnel: %v", err)
	}
	if resp.GetViews() != 4 {
		t.Errorf("views = %d, want 4", resp.GetViews())
	}
	if repo.lastSeller != "s9" {
		t.Errorf("repository read seller %q, want s9", repo.lastSeller)
	}
}

// Scenario: A service principal without admin is denied
func TestServicePrincipalWithoutAdminIsDenied(t *testing.T) {
	repo := &spyRepo{}
	_, err := mustHandler(repo).GetSellerFunnel(asPrincipal("svc-1", "service", "analytics.read"), &analyticsv1.GetSellerFunnelRequest{SellerId: "s1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", status.Code(err))
	}
	if repo.calls != 0 {
		t.Errorf("repository calls = %d, want 0", repo.calls)
	}
}

// Scenario: An admin RPC requires the exact admin scope
func TestAdminRPCsRequireExactAdminScope(t *testing.T) {
	repo := &capRepo{}
	h := mustHandler(repo)
	ctx := asPrincipal("u-1", "user", "admin:read")
	calls := map[string]func() error{
		"GetPlatformOrderSummary": func() error {
			_, err := h.GetPlatformOrderSummary(ctx, &analyticsv1.GetPlatformOrderSummaryRequest{})
			return err
		},
		"ListRecentOrders": func() error {
			_, err := h.ListRecentOrders(ctx, &analyticsv1.ListRecentOrdersRequest{})
			return err
		},
		"GetTrackingQualityReport": func() error {
			_, err := h.GetTrackingQualityReport(ctx, &analyticsv1.GetTrackingQualityReportRequest{})
			return err
		},
		"GetRecommendationPerformance": func() error {
			_, err := h.GetRecommendationPerformance(ctx, &analyticsv1.GetRecommendationPerformanceRequest{})
			return err
		},
		"GetChannelFunnel": func() error {
			_, err := h.GetChannelFunnel(ctx, &analyticsv1.GetChannelFunnelRequest{})
			return err
		},
	}
	for name, call := range calls {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s: code = %v, want PermissionDenied", name, got)
		}
	}
	if repo.calls != 0 {
		t.Errorf("repository calls = %d, want 0", repo.calls)
	}
}

// Scenario: A repository error does not leak driver text
func TestRepositoryErrorDoesNotLeak(t *testing.T) {
	admin := asPrincipal("a1", "user", "admin")
	for name, call := range map[string]func(h *rpc.Handler) error{
		"GetSellerFunnel": func(h *rpc.Handler) error {
			_, err := h.GetSellerFunnel(admin, &analyticsv1.GetSellerFunnelRequest{SellerId: "s1"})
			return err
		},
		"GetRevenueBreakdown": func(h *rpc.Handler) error {
			_, err := h.GetRevenueBreakdown(admin, &analyticsv1.GetRevenueBreakdownRequest{SellerId: "s1"})
			return err
		},
		"GetPlatformOrderSummary": func(h *rpc.Handler) error {
			_, err := h.GetPlatformOrderSummary(admin, &analyticsv1.GetPlatformOrderSummaryRequest{})
			return err
		},
		"ListRecentOrders": func(h *rpc.Handler) error {
			_, err := h.ListRecentOrders(admin, &analyticsv1.ListRecentOrdersRequest{})
			return err
		},
	} {
		err := call(mustHandler(&spyRepo{err: fmt.Errorf("query: %w", errors.New(driverText))}))
		st, _ := status.FromError(err)
		if st.Code() != codes.Internal {
			t.Errorf("%s: code = %v, want Internal", name, st.Code())
		}
		if st.Message() != "internal error" {
			t.Errorf("%s: message = %q, want %q", name, st.Message(), "internal error")
		}
		if strings.Contains(st.Message(), "duckdb") || strings.Contains(st.Message(), "/data") {
			t.Errorf("%s: driver text leaked: %q", name, st.Message())
		}
	}
}

// Scenario: A cancelled caller gets Canceled
func TestCancelledContextMapsToCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(asPrincipal("s1", "user", "seller"))
	defer cancel()
	repo := &spyRepo{onCall: func(c context.Context) error {
		cancel()
		return c.Err()
	}}
	_, err := mustHandler(repo).GetSellerFunnel(ctx, &analyticsv1.GetSellerFunnelRequest{SellerId: "s1"})
	if status.Code(err) != codes.Canceled {
		t.Errorf("code = %v, want Canceled", status.Code(err))
	}
	if repo.calls != 1 {
		t.Errorf("repository calls = %d, want 1", repo.calls)
	}
}

// Scenario: An unsupported capability is unavailable
func TestUnsupportedCapabilityIsUnavailable(t *testing.T) {
	admin := asPrincipal("a1", "user", "admin")
	now := time.Now().UTC()
	req := &analyticsv1.GetChannelFunnelRequest{From: ts(now.Add(-time.Hour)), To: ts(now)}
	for name, repoErr := range map[string]error{
		"bare":    query.ErrUnsupported,
		"wrapped": fmt.Errorf("channel funnel: %w", query.ErrUnsupported),
	} {
		repo := &capRepo{channelErr: repoErr}
		_, err := mustHandler(repo).GetChannelFunnel(admin, req)
		st, _ := status.FromError(err)
		if st.Code() != codes.Unavailable {
			t.Errorf("%s: code = %v, want Unavailable", name, st.Code())
		}
		if st.Message() != "not available on this warehouse" {
			t.Errorf("%s: message = %q, want %q", name, st.Message(), "not available on this warehouse")
		}
	}
}

// Scenario: An invalid window is rejected before the repository
func TestInvalidWindowIsRejectedBeforeRepository(t *testing.T) {
	admin := asPrincipal("a1", "user", "admin")
	repo := &capRepo{}
	h := mustHandler(repo)
	if _, err := h.GetTrackingQualityReport(admin, &analyticsv1.GetTrackingQualityReportRequest{WindowHours: 169}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("window_hours=169: code = %v, want InvalidArgument", status.Code(err))
	}
	now := time.Now().UTC()
	if _, err := h.GetChannelFunnel(admin, &analyticsv1.GetChannelFunnelRequest{From: ts(now), To: ts(now.Add(-time.Hour))}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("from after to: code = %v, want InvalidArgument", status.Code(err))
	}
	if repo.calls != 0 {
		t.Errorf("repository calls = %d, want 0", repo.calls)
	}
}

// Scenario: A server built from default settings does not serve reflection
func TestReflectionOffByDefault(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50059")
	t.Setenv("GRPC_REFLECTION_ENABLED", "")
	if err := os.Unsetenv("GRPC_REFLECTION_ENABLED"); err != nil {
		t.Fatal(err)
	}
	s, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	conn := startServer(t, s.GRPC, mustHandler(&spyRepo{}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := rpb.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	if err == nil {
		err = stream.Send(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_ListServices{ListServices: "*"}})
		if err == nil {
			_, err = stream.Recv()
		}
	}
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("reflection call err = %v, want Unimplemented", err)
	}
}
