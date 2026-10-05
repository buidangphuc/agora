package query_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/interceptor"
	"github.com/buidangphuc/team-analytics/internal/query"
)

// asPrincipal runs the real principal interceptor over gateway-style metadata.
func asPrincipal(id, ptype, scopes string) context.Context {
	md := metadata.Pairs("x-principal-id", id, "x-principal-type", ptype, "x-principal-scopes", scopes)
	in := metadata.NewIncomingContext(context.Background(), md)
	var out context.Context
	_, _ = interceptor.UnaryServerInterceptor()(in, nil, &grpc.UnaryServerInfo{},
		func(c context.Context, _ any) (any, error) { out = c; return nil, nil })
	return out
}

func TestSellerRPCAccess(t *testing.T) {
	svc := query.NewService(newRepo())
	calls := map[string]func(ctx context.Context, seller string) error{
		"funnel": func(ctx context.Context, s string) error {
			_, err := svc.GetSellerFunnel(ctx, &analyticsv1.GetSellerFunnelRequest{SellerId: s})
			return err
		},
		"revenue": func(ctx context.Context, s string) error {
			_, err := svc.GetRevenueBreakdown(ctx, &analyticsv1.GetRevenueBreakdownRequest{SellerId: s})
			return err
		},
		"forecast": func(ctx context.Context, s string) error {
			_, err := svc.GetDemandForecast(ctx, &analyticsv1.GetDemandForecastRequest{SellerId: s, ListingId: "lst-a"})
			return err
		},
	}
	cases := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"owner", asPrincipal("seller-1", "user", "seller"), codes.OK},
		{"other seller", asPrincipal("seller-2", "user", "seller"), codes.PermissionDenied},
		{"no principal", context.Background(), codes.Unauthenticated},
		{"anonymous principal", asPrincipal("anonymous", "anonymous", "catalog.read"), codes.Unauthenticated},
		{"admin", asPrincipal("admin-1", "user", "admin,seller"), codes.OK},
		{"service without admin", asPrincipal("svc-x", "service", "order.read"), codes.PermissionDenied},
		{"service id equal to seller is not ownership", asPrincipal("seller-1", "service", ""), codes.PermissionDenied},
	}
	for rpc, call := range calls {
		for _, c := range cases {
			if got := status.Code(call(c.ctx, "seller-1")); got != c.want {
				t.Errorf("%s/%s: code = %v, want %v", rpc, c.name, got, c.want)
			}
		}
	}
}

func TestSellerRPCAccess_DeniedBeforeValidation(t *testing.T) {
	svc := query.NewService(nil)
	_, err := svc.GetSellerFunnel(asPrincipal("seller-2", "user", ""), &analyticsv1.GetSellerFunnelRequest{SellerId: "seller-1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("code = %v, want PermissionDenied before repo check", status.Code(err))
	}
}
