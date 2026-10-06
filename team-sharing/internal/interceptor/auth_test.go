package interceptor_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-sharing/internal/interceptor"
)

func callerThrough(t *testing.T, md metadata.MD) (string, error) {
	t.Helper()
	ctx := metadata.NewIncomingContext(context.Background(), md)
	var id string
	var reqErr error
	_, err := interceptor.Unary()(ctx, nil, &grpc.UnaryServerInfo{}, func(c context.Context, _ any) (any, error) {
		id = interceptor.CallerID(c)
		_, reqErr = interceptor.RequirePrincipal(c)
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id, reqErr
}

func TestForwardedPrincipalIsTrusted(t *testing.T) {
	id, err := callerThrough(t, metadata.Pairs(
		"x-principal-id", "user-1", "x-principal-type", "user", "x-principal-scopes", "a, b"))
	if id != "user-1" || err != nil {
		t.Fatalf("got id=%q err=%v", id, err)
	}
}

// The old unverified x-user-id header must no longer identify anyone.
func TestLegacyUserIDHeaderIgnored(t *testing.T) {
	id, err := callerThrough(t, metadata.Pairs("x-user-id", "attacker"))
	if id != "" || status.Code(err) != codes.Unauthenticated {
		t.Fatalf("got id=%q err=%v", id, err)
	}
}

func TestGatewayAnonymousPrincipalIsAnonymous(t *testing.T) {
	id, err := callerThrough(t, metadata.Pairs(
		"x-principal-id", "anonymous", "x-principal-type", "anonymous"))
	if id != "" || status.Code(err) != codes.Unauthenticated {
		t.Fatalf("got id=%q err=%v", id, err)
	}
}

func TestNoMetadataIsAnonymous(t *testing.T) {
	id, err := callerThrough(t, metadata.MD{})
	if id != "" || status.Code(err) != codes.Unauthenticated {
		t.Fatalf("got id=%q err=%v", id, err)
	}
}
