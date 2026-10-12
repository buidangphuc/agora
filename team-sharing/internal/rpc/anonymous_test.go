package rpc_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-sharing/internal/rpc"
	"github.com/buidangphuc/team-sharing/internal/sharelink"
	"github.com/buidangphuc/team-sharing/internal/store"
)

// create calls CreateShareLink through the SDK auth interceptor with the given
// incoming metadata, as the gateway-forwarded call would arrive.
func create(t *testing.T, h *rpc.Handler, md metadata.MD) (string, error) {
	t.Helper()
	ctx := metadata.NewIncomingContext(context.Background(), md)
	resp, err := auth.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		return h.CreateShareLink(ctx, &sharingv1.CreateShareLinkRequest{TargetType: "listing", TargetId: "l1"})
	})
	if err != nil {
		return "", err
	}
	return resp.(*sharingv1.CreateShareLinkResponse).GetShortCode(), nil
}

func TestCreateAnonymousStoresEmptyCreator(t *testing.T) {
	repo := store.NewInMemoryShareLinkRepo()
	h := rpc.NewHandler(sharelink.NewService(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))

	cases := map[string]struct {
		md   metadata.MD
		want string
	}{
		"gateway anonymous principal": {metadata.Pairs("x-principal-id", "anonymous", "x-principal-type", "anonymous"), ""},
		"no metadata":                 {metadata.MD{}, ""},
		"legacy x-user-id ignored":    {metadata.Pairs("x-user-id", "attacker"), ""},
		"forwarded user":              {metadata.Pairs("x-principal-id", "user-1", "x-principal-type", "user"), "user-1"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			code, err := create(t, h, c.md)
			if err != nil {
				t.Fatalf("create must not be rejected: %v", err)
			}
			link, err := repo.Resolve(context.Background(), code)
			if err != nil {
				t.Fatal(err)
			}
			if link.CreatedBy != c.want {
				t.Fatalf("created_by = %q, want %q", link.CreatedBy, c.want)
			}
		})
	}
}
