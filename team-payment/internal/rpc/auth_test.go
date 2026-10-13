package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
)

func TestAuthInterceptor(t *testing.T) {
	t.Run("ContextWithPrincipal", func(t *testing.T) {
		p := &commonv1.Principal{
			Id:     "user-1",
			Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
			Scopes: []string{"payment:read"},
		}
		ctx := withPrincipal(context.Background(), p)

		got, ok := auth.FromContext(ctx)
		if !ok || got.ID != "user-1" {
			t.Errorf("expected principal user-1, got %v", got)
		}

		reqP, err := auth.RequirePrincipal(ctx)
		if err != nil || reqP.ID != "user-1" {
			t.Errorf("expected require principal user-1")
		}
	})

	t.Run("RequirePrincipal missing fails", func(t *testing.T) {
		_, err := auth.RequirePrincipal(context.Background())
		if err == nil {
			t.Errorf("expected unauthenticated error")
		}
	})

	t.Run("AuthUnaryInterceptor with metadata", func(t *testing.T) {
		md := metadata.New(map[string]string{
			"x-principal-id":     "user-meta",
			"x-principal-type":   "user",
			"x-principal-scopes": "payment:write,payment:read",
		})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		interceptorFunc := auth.UnaryServerInterceptor()
		handlerCalled := false
		_, err := interceptorFunc(ctx, "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
			handlerCalled = true
			p, ok := auth.FromContext(ctx)
			if !ok || p.ID != "user-meta" {
				t.Errorf("expected user-meta from metadata")
			}
			return "res", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !handlerCalled {
			t.Errorf("expected handler to be called")
		}
	})
}
