package upstream

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
)

// outgoingMD runs the interceptor for method with an incoming user principal and
// returns the metadata that would be sent to team-domain.
func outgoingMD(t *testing.T, method string, incoming metadata.MD) metadata.MD {
	t.Helper()
	ctx := metadata.NewIncomingContext(context.Background(), incoming)
	var got metadata.MD
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		got, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}
	if err := forwardMetadataInterceptor()(ctx, method, nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	return got
}

func userMD() metadata.MD {
	return metadata.Pairs(
		"x-principal-id", "buyer-1",
		"x-principal-type", "user",
		"x-principal-scopes", "listing.read",
		"x-request-id", "req-1",
	)
}

func TestStockCallsAlwaysUseServicePrincipal(t *testing.T) {
	for _, method := range []string{
		listingv1.ListingService_ReserveStock_FullMethodName,
		listingv1.ListingService_ReleaseStock_FullMethodName,
	} {
		t.Run(method, func(t *testing.T) {
			md := outgoingMD(t, method, userMD())
			if v := md.Get("x-principal-id"); len(v) != 1 || v[0] != "service-team-order" {
				t.Fatalf("principal id = %v, want service-team-order", v)
			}
			if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "service" {
				t.Fatalf("principal type = %v, want service", v)
			}
			if v := md.Get("x-principal-scopes"); len(v) != 1 || !hasScope(v[0], "listing.write") {
				t.Fatalf("scopes = %v, want listing.write", v)
			}
			if v := md.Get("x-request-id"); len(v) != 1 || v[0] != "req-1" {
				t.Fatalf("request id not preserved: %v", v)
			}
		})
	}
}

func TestStockCallsWithoutIncomingUseServicePrincipal(t *testing.T) {
	md := outgoingMD(t, listingv1.ListingService_ReleaseStock_FullMethodName, nil)
	if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "service" {
		t.Fatalf("principal type = %v, want service", v)
	}
}

func TestOtherDomainCallsKeepForwardedUser(t *testing.T) {
	md := outgoingMD(t, listingv1.ListingService_GetListing_FullMethodName, userMD())
	if v := md.Get("x-principal-id"); len(v) != 1 || v[0] != "buyer-1" {
		t.Fatalf("GetListing principal id = %v, want forwarded buyer-1", v)
	}
	if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "user" {
		t.Fatalf("GetListing principal type = %v, want user", v)
	}
}

func hasScope(csv, want string) bool {
	start := 0
	for i := 0; i <= len(csv); i++ {
		if i == len(csv) || csv[i] == ',' {
			if csv[start:i] == want {
				return true
			}
			start = i + 1
		}
	}
	return false
}
