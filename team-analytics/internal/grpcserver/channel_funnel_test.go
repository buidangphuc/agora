package grpcserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/grpcserver"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

func funnelCall(t *testing.T, scopes string, req *analyticsv1.GetChannelFunnelRequest) (*analyticsv1.GetChannelFunnelResponse, error) {
	t.Helper()
	w, err := duckdb.Open(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpcserver.Build(&config.Settings{}, nil, query.NewService(query.NewDuckDBRepository(w.DB())))
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-principal-id", "u-1", "x-principal-type", "user", "x-principal-scopes", scopes)
	return analyticsv1.NewAnalyticsQueryServiceClient(conn).GetChannelFunnel(ctx, req)
}

func funnelWindow() *analyticsv1.GetChannelFunnelRequest {
	now := time.Now().UTC()
	return &analyticsv1.GetChannelFunnelRequest{From: timestamppb.New(now.Add(-time.Hour)), To: timestamppb.New(now)}
}

// Scenario: Only admins can read the channel funnel (direct call over the wire)
func TestChannelFunnel_OnlyAdminsCanReadTheChannelFunnel_Wire(t *testing.T) {
	resp, err := funnelCall(t, "seller", funnelWindow())
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("non-admin code = %v, want PermissionDenied", status.Code(err))
	}
	if len(resp.GetRows()) != 0 {
		t.Errorf("rows leaked to non-admin: %v", resp.GetRows())
	}
	// Scenario: A window with no events returns no rows (admin reaches the handler)
	resp, err = funnelCall(t, "admin", funnelWindow())
	if err != nil || len(resp.GetRows()) != 0 {
		t.Errorf("admin on empty warehouse: rows=%v err=%v, want success with no rows", resp.GetRows(), err)
	}
}

// Scenario: An invalid window is rejected (over the wire)
func TestChannelFunnel_AnInvalidWindowIsRejected_Wire(t *testing.T) {
	now := time.Now().UTC()
	_, err := funnelCall(t, "admin", &analyticsv1.GetChannelFunnelRequest{From: timestamppb.New(now), To: timestamppb.New(now.Add(-time.Hour))})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}
