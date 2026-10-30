package rpc_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

func wireFunnelCall(t *testing.T, scopes string, req *analyticsv1.GetChannelFunnelRequest) (*analyticsv1.GetChannelFunnelResponse, error) {
	t.Helper()
	w, err := duckdb.Open(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	conn := startServer(t, sdkconfig.GRPC{}, mustHandler(query.NewDuckDBRepository(w.DB())))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-principal-id", "u-1", "x-principal-type", "user", "x-principal-scopes", scopes)
	return analyticsv1.NewAnalyticsQueryServiceClient(conn).GetChannelFunnel(ctx, req)
}

func wireFunnelWindow() *analyticsv1.GetChannelFunnelRequest {
	now := time.Now().UTC()
	return &analyticsv1.GetChannelFunnelRequest{From: timestamppb.New(now.Add(-time.Hour)), To: timestamppb.New(now)}
}

// Scenario: Only admins can read the channel funnel (direct call over the wire)
func TestChannelFunnel_OnlyAdminsCanReadTheChannelFunnel_Wire(t *testing.T) {
	resp, err := wireFunnelCall(t, "seller", wireFunnelWindow())
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("non-admin code = %v, want PermissionDenied", status.Code(err))
	}
	if len(resp.GetRows()) != 0 {
		t.Errorf("rows leaked to non-admin: %v", resp.GetRows())
	}
	// Scenario: A window with no events returns no rows (admin reaches the handler)
	resp, err = wireFunnelCall(t, "admin", wireFunnelWindow())
	if err != nil || len(resp.GetRows()) != 0 {
		t.Errorf("admin on empty warehouse: rows=%v err=%v, want success with no rows", resp.GetRows(), err)
	}
}

// Scenario: An invalid window is rejected (over the wire)
func TestChannelFunnel_AnInvalidWindowIsRejected_Wire(t *testing.T) {
	now := time.Now().UTC()
	_, err := wireFunnelCall(t, "admin", &analyticsv1.GetChannelFunnelRequest{From: timestamppb.New(now), To: timestamppb.New(now.Add(-time.Hour))})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}
