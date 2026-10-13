package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
)

// Callers resolve through the SDK auth interceptor, as a gateway-forwarded call
// would arrive. The gateway's anonymous principal, and a missing principal, are
// both anonymous and are rejected; a verified user is attributed by id.
func TestInterceptorTreatsGatewayAnonymousAsAnonymous(t *testing.T) {
	cases := []struct {
		name     string
		md       metadata.MD
		wantAnon bool
		wantID   string // checked for non-anonymous callers only
	}{
		{"user", metadata.Pairs("x-principal-id", "u-1", "x-principal-type", "user"), false, "u-1"},
		{"gateway anonymous", metadata.Pairs("x-principal-id", "anonymous", "x-principal-type", "anonymous"), true, ""},
		{"anonymous type with an id", metadata.Pairs("x-principal-id", "u-1", "x-principal-type", "anonymous"), true, ""},
		{"anonymous id without a type", metadata.Pairs("x-principal-id", "anonymous"), true, ""},
		{"no metadata", metadata.MD{}, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), c.md)
			var gotAnon bool
			var gotID string
			_, err := auth.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
				p, ok := auth.FromContext(ctx)
				gotAnon, gotID = !ok || p.Anonymous(), p.ID
				return newHandler(t).CreateReferralCode(ctx, &referralv1.CreateReferralCodeRequest{})
			})
			if gotAnon != c.wantAnon {
				t.Fatalf("anonymous = %v, want %v", gotAnon, c.wantAnon)
			}
			if c.wantAnon {
				if status.Code(err) != codes.Unauthenticated {
					t.Fatalf("anonymous caller: expected Unauthenticated, got %v", err)
				}
				return
			}
			if err != nil || gotID != c.wantID {
				t.Fatalf("user caller: id = %q, err = %v, want id %q and no error", gotID, err, c.wantID)
			}
		})
	}
}
