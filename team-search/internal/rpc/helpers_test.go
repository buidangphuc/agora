package rpc_test

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	"github.com/buidangphuc/team-search/internal/store"
)

// principalCtx builds a context the way the gateway does (x-principal-* metadata
// through the service's unary interceptor) with an arbitrary principal type. An
// empty id forwards no principal at all; an empty typ forwards no type header.
func principalCtx(id, typ string, scopes ...string) context.Context {
	ctx := context.Background()
	if id == "" {
		return ctx
	}
	pairs := []string{"x-principal-id", id, "x-principal-scopes", strings.Join(scopes, ",")}
	if typ != "" {
		pairs = append(pairs, "x-principal-type", typ)
	}
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pairs...))
	var out context.Context
	_, _ = auth.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{},
		func(c context.Context, _ any) (any, error) { out = c; return nil, nil })
	return out
}

// spyRepo wraps a repository and counts every call, so a denied RPC can be shown
// not to have touched storage.
type spyRepo struct {
	inner store.SavedSearchRepository
	calls int
}

func (s *spyRepo) Create(ctx context.Context, x store.SavedSearch) (store.SavedSearch, error) {
	s.calls++
	return s.inner.Create(ctx, x)
}

func (s *spyRepo) List(ctx context.Context, u string, l, o int) ([]store.SavedSearch, int64, error) {
	s.calls++
	return s.inner.List(ctx, u, l, o)
}

func (s *spyRepo) Get(ctx context.Context, id, u string) (store.SavedSearch, error) {
	s.calls++
	return s.inner.Get(ctx, id, u)
}

func (s *spyRepo) Delete(ctx context.Context, id, u string) error {
	s.calls++
	return s.inner.Delete(ctx, id, u)
}
