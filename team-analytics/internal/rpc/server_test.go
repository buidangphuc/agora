package rpc_test

import (
	"context"
	"testing"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query/querytest"
)

// TestHealthServesOnEphemeralPort is the in-process health check: the worker's
// health-only gRPC server answers grpc.health.v1.Health/Check with SERVING —
// which is what the k8s probes call.
func TestHealthServesOnEphemeralPort(t *testing.T) {
	conn := startServer(t, sdkconfig.GRPC{}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("health status = %v, want SERVING", resp.GetStatus())
	}
}

// TestAnalyticsQueryServiceRegistered proves the new AnalyticsQueryService is
// registered and reachable over the wire when a servicer is supplied to Build
// (mirroring how Health is wired), returning a real aggregation from a
// memory-backed repository.
func TestAnalyticsQueryServiceRegistered(t *testing.T) {
	now := time.Now().UTC()
	repo := querytest.NewMemoryRepository(
		querytest.Event{SellerID: "seller-1", EventType: "impression", OccurredAt: now},
		querytest.Event{SellerID: "seller-1", EventType: "view", OccurredAt: now},
		querytest.Event{SellerID: "seller-1", EventType: "add_to_cart", OccurredAt: now},
		querytest.Event{SellerID: "seller-1", OrderID: "o-1", Revenue: 500, OccurredAt: now},
	)
	conn := startServer(t, sdkconfig.GRPC{}, mustHandler(repo))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-principal-id", "seller-1", "x-principal-type", "user")
	resp, err := analyticsv1.NewAnalyticsQueryServiceClient(conn).
		GetSellerFunnel(ctx, &analyticsv1.GetSellerFunnelRequest{SellerId: "seller-1"})
	if err != nil {
		t.Fatalf("GetSellerFunnel: %v", err)
	}
	if resp.GetImpressions() != 1 || resp.GetViews() != 1 || resp.GetAdds() != 1 || resp.GetOrders() != 1 {
		t.Errorf("funnel = %+v, want impressions/views/adds/orders all 1", resp)
	}
}
