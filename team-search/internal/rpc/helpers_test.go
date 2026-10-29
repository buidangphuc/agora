package rpc_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/rpc"
	"github.com/buidangphuc/team-search/internal/store"
)

func principalCtx(id, typ string, scopes ...string) context.Context {
	ctx := context.Background()
	if id == "" {
		return ctx
	}
	return auth.WithPrincipal(ctx, auth.Principal{ID: id, Type: auth.PrincipalType(typ), Scopes: scopes})
}

func userCtx(id string, scopes ...string) context.Context {
	return principalCtx(id, "user", scopes...)
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newHandler(t *testing.T, idx index.Index, saved rpc.SavedSearches) *rpc.Handler {
	t.Helper()
	return newHandlerEngine(t, idx, nil, saved)
}

func newHandlerEngine(t *testing.T, idx index.Index, eng *retrieval.Engine, saved rpc.SavedSearches) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(idx, eng, saved, discardLogger())
	if err != nil {
		t.Fatalf("rpc.NewHandler: %v", err)
	}
	return h
}

func newSavedHandler(t *testing.T) *rpc.Handler {
	t.Helper()
	return newHandler(t, &mockIndex{}, newFakeSaved())
}

type spyRepo struct {
	inner rpc.SavedSearches
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

type failingSaved struct{ err error }

func (f failingSaved) Create(context.Context, store.SavedSearch) (store.SavedSearch, error) {
	return store.SavedSearch{}, f.err
}

func (f failingSaved) List(context.Context, string, int, int) ([]store.SavedSearch, int64, error) {
	return nil, 0, f.err
}

func (f failingSaved) Get(context.Context, string, string) (store.SavedSearch, error) {
	return store.SavedSearch{}, f.err
}

func (f failingSaved) Delete(context.Context, string, string) error { return f.err }

func pageReq(cursor string, size int32) *commonv1.PageRequest {
	return &commonv1.PageRequest{Cursor: cursor, PageSize: size}
}
