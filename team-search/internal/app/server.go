package app

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/rpc"
	"github.com/buidangphuc/team-search/internal/store"
	"github.com/buidangphuc/team-search/migrations"
)

const (
	serviceName  = "team-search"
	closeGrace   = 2 * time.Second
	modelTimeout = 2 * time.Second
)

func Run(ctx context.Context, s config.Settings) (err error) {
	logger := logx.New(s.Runtime, serviceName)

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, serviceName)
	if err != nil {
		return fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return err
	}

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	idx, err := index.New(s.OpenSearch.URL, s.OpenSearch.Index)
	if err != nil {
		return fmt.Errorf("opensearch: %w", err)
	}
	if err := idx.EnsureIndex(ctx); err != nil {
		return fmt.Errorf("ensure index: %w", err)
	}
	health.Register("opensearch", idx.Ping)

	embed := retrieval.NewHTTPEmbedClient(s.Retrieval.ModelServerURL, modelTimeout)
	var rerank retrieval.RerankClient
	if s.Retrieval.EnableReranker {
		rerank = retrieval.NewHTTPRerankClient(s.Retrieval.ModelServerURL, modelTimeout)
	}
	engine := retrieval.NewEngine(idx, embed, rerank, s.Retrieval)

	h, err := rpc.NewHandler(idx, engine, store.NewPostgresSavedSearchRepository(pool), logger)
	if err != nil {
		return err
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	searchv1.RegisterSearchServiceServer(srv, h)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, health, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC)))
}

func Migrate(ctx context.Context, s config.MigrateSettings) error {
	if err := migrate.Up(ctx, s.Postgres.URL, fs.FS(migrations.FS)); err != nil {
		host := "unknown"
		if u, perr := url.Parse(s.Postgres.URL); perr == nil && u.Host != "" {
			host = u.Host
		}
		return fmt.Errorf("migrate %s: %w", host, err)
	}
	return nil
}
