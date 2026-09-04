package grpcserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/grpcserver"
	"github.com/buidangphuc/team-analytics/internal/query"
)

// TestHealthServesOnEphemeralPort is the in-process health check: the worker's
// health-only gRPC server answers grpc.health.v1.Health/Check with SERVING —
// which is what the k8s probes call.
func TestHealthServesOnEphemeralPort(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpcserver.Build(&config.Settings{}, nil, nil)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

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
	repo := query.NewMemoryRepository(
		query.Event{SellerID: "seller-1", EventType: "impression", OccurredAt: now},
		query.Event{SellerID: "seller-1", EventType: "view", OccurredAt: now},
		query.Event{SellerID: "seller-1", EventType: "add_to_cart", OccurredAt: now},
		query.Event{SellerID: "seller-1", OrderID: "o-1", Revenue: 500, OccurredAt: now},
	)
	svc := query.NewService(repo)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpcserver.Build(&config.Settings{}, nil, svc)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := analyticsv1.NewAnalyticsQueryServiceClient(conn).
		GetSellerFunnel(ctx, &analyticsv1.GetSellerFunnelRequest{SellerId: "seller-1"})
	if err != nil {
		t.Fatalf("GetSellerFunnel: %v", err)
	}
	if resp.GetImpressions() != 1 || resp.GetViews() != 1 || resp.GetAdds() != 1 || resp.GetOrders() != 1 {
		t.Errorf("funnel = %+v, want impressions/views/adds/orders all 1", resp)
	}
}
