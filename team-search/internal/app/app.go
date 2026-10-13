// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs. Each
// binary has its own entry point: RunServer (the gRPC query API) and RunIndexer
// (the Kafka consumer that keeps the OpenSearch read-model in sync). Both load
// the same Settings.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/events"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/rpc"
	"github.com/buidangphuc/team-search/internal/store"
	"github.com/buidangphuc/team-search/internal/taxonomy"
	"github.com/buidangphuc/team-search/internal/tombstone"
)

const service = "team-search"

// RunServer wires the gRPC query API and runs it until ctx ends (main cancels it
// on SIGINT/SIGTERM) or a component fails. Build order: logger, telemetry,
// health, database, OpenSearch, server. Everything opened goes on the stack;
// closers run in reverse order on shutdown and on an early failure.
//
// Migrations are not applied here: the compose job team-search-migrate does it.
func RunServer(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return errors.Join(fmt.Errorf("health: %w", err), stack.Close(ctx))
	}

	// Saved searches live in Postgres. The database opens before OpenSearch so a
	// deployed environment without it fails boot first.
	saved, err := selectSavedSearchRepository(ctx, s, logger, health, &stack, postgres.Open)
	if err != nil {
		return errors.Join(err, stack.Close(ctx))
	}

	rm, err := openReadModel(ctx, s, &stack)
	if err != nil {
		return errors.Join(err, stack.Close(ctx))
	}
	health.Register("opensearch", rm.index.Ping)

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.NewSearchHandlerWithEngine(rm.index, rm.engine, saved))

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, grpcx.Serve(srv, s.GRPC), lifecycle.Func(health.Run))
}

// RunIndexer wires the Kafka indexer and runs it until ctx ends or the consumer
// fails. Build order: logger, telemetry, OpenSearch, consumer. The consumer
// reads listing.events (ADR-0002/0005); a record that fails its retries is parked
// on listing.events.dlq. The tombstone purge loop (D6) runs beside it.
func RunIndexer(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	rm, err := openReadModel(ctx, s, &stack)
	if err != nil {
		return errors.Join(err, stack.Close(ctx))
	}

	cons, err := events.NewConsumer(s.Kafka, s.Events.ConsumerGroup, s.Events.ListingTopic,
		events.NewListingEventHandler(rm.index, rm.embed, rm.classifier), consume.Options{Logger: logger})
	if err != nil {
		return errors.Join(err, stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { cons.Close(); return nil })

	ttl, warnTTL := s.TombstoneTTL()
	interval, warnInterval := s.TombstonePurgeInterval()
	for _, w := range []string{warnTTL, warnInterval} {
		if w != "" {
			logger.WarnContext(ctx, w)
		}
	}
	logger.InfoContext(ctx, "tombstone purge configured",
		"tombstone_ttl", ttl.String(), "tombstone_purge_interval", interval.String())
	purge, err := tombstone.Loop{Purger: rm.index, TTL: ttl, Interval: interval, Logger: logger}.Component()
	if err != nil {
		return errors.Join(fmt.Errorf("tombstone purge: %w", err), stack.Close(ctx))
	}

	logger.InfoContext(ctx, "indexer consuming",
		"topic", s.Events.ListingTopic,
		"group", s.Events.ConsumerGroup,
		"dlq_topic", s.Events.ListingTopic+events.DLQSuffix,
		"opensearch_index", s.OpenSearch.Index)

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.Func(cons.Run), purge)
}

// readModel is what both binaries open: the OpenSearch index and the retrieval
// clients around it.
type readModel struct {
	index      *index.OpenSearchIndex
	embed      retrieval.EmbedClient
	classifier taxonomy.Classifier // nil when UPSTREAM_AI_ADDR is empty
	engine     *retrieval.Engine
}

// openReadModel builds the OpenSearch index client, ensures the index exists and
// constructs the retrieval clients and engine. OpenSearch is this service's
// store (ADR-0005); its client is plain HTTP, so there is nothing to close.
func openReadModel(ctx context.Context, s config.Settings, stack *lifecycle.Stack) (*readModel, error) {
	idx, err := index.New(s.OpenSearch.URL, s.OpenSearch.Index)
	if err != nil {
		return nil, err
	}
	if err := idx.EnsureIndex(ctx); err != nil {
		return nil, fmt.Errorf("ensure index: %w", err)
	}

	embed := retrieval.NewHTTPEmbedClient(s.Retrieval.ModelServerURL, 2*time.Second)
	var rerank retrieval.RerankClient
	if s.Retrieval.EnableReranker {
		rerank = retrieval.NewHTTPRerankClient(s.Retrieval.ModelServerURL, 2*time.Second)
	}

	var classifier taxonomy.Classifier
	if s.Taxonomy.AIAddr != "" {
		c, conn, err := taxonomy.DialGRPCClassifier(s.Taxonomy.AIAddr)
		if err != nil {
			return nil, err
		}
		stack.Push(func(context.Context) error { return conn.Close() })
		classifier = c
	}

	return &readModel{
		index:      idx,
		embed:      embed,
		classifier: classifier,
		engine:     retrieval.NewEngine(idx, embed, rerank, s.Retrieval),
	}, nil
}

// poolOpener opens (and verifies) a Postgres pool.
type poolOpener func(ctx context.Context, c sdkconfig.Postgres) (*pgxpool.Pool, error)

// selectSavedSearchRepository returns the Postgres-backed repository and
// registers its health checker. An unreachable database fails boot, except with
// ENV=local (or test), where the in-memory store keeps the service usable and its
// saved searches live only for the process.
func selectSavedSearchRepository(ctx context.Context, s config.Settings, logger *slog.Logger, health *healthx.Health, stack *lifecycle.Stack, open poolOpener) (store.SavedSearchRepository, error) {
	pool, err := open(ctx, s.Postgres)
	switch {
	case err == nil:
		db := stdlib.OpenDBFromPool(pool)
		health.Register("postgres", postgres.Checker(pool))
		stack.Push(func(context.Context) error { _ = db.Close(); pool.Close(); return nil })
		logger.InfoContext(ctx, "saved searches backed by postgres")
		return store.NewPostgresSavedSearchRepository(db), nil
	case s.IsLocal():
		logger.WarnContext(ctx, "database unreachable, saved searches use an in-memory store and are lost on restart", "err", err)
		return store.NewInMemorySavedSearchRepository(), nil
	default:
		return nil, fmt.Errorf("open database: %w", err)
	}
}
