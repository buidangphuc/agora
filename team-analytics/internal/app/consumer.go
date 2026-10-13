package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"

	"github.com/buidangphuc/team-analytics/internal/analytics"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/export"
	"github.com/buidangphuc/team-analytics/internal/ga4import"
	"github.com/buidangphuc/team-analytics/internal/observability"
	"github.com/buidangphuc/team-analytics/internal/rpc"
	"github.com/buidangphuc/team-analytics/internal/store"
	"github.com/buidangphuc/team-analytics/internal/store/duckdb"
)

// RunConsumer wires the analytics warehouse writer and runs it until ctx ends
// (main cancels it on SIGINT/SIGTERM) or a component fails.
//
// The writer is a Kafka consumer that reads analytics.events and order.events,
// batches the rows and appends them to the warehouse (DuckDB local / BigQuery
// prod) behind the WarehouseWriter seam. Its batch loop buffers records across
// polls, flushes on size or on a timer and commits only once the buffers are
// empty, which consume.BatchConsumer (one handler call and one commit per
// fetch) does not cover, so it stays a custom loop on a kafkax client. The
// listing and engagement consumers use consume.BatchConsumer. The gRPC server
// serves the health probes and, on the DuckDB adapter, the read-only
// AnalyticsQueryService.
//
// Build order: logger, telemetry, health, warehouse, server, workers. Everything
// opened goes on the stack; closers run in reverse order on shutdown and on an
// early failure. The workers stop after the server.
func RunConsumer(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)

	var stack lifecycle.Stack
	fail := func(err error) error { return errors.Join(err, stack.Close(ctx)) }

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return fail(fmt.Errorf("health: %w", err))
	}

	w, err := openWarehouse(ctx, s)
	if err != nil {
		return fail(fmt.Errorf("open warehouse: %w", err))
	}
	stack.Push(func(context.Context) error { return w.Close() })

	// The read-only query service and the GA4 import need the local DuckDB
	// adapter. They share the writer's in-process handle so reads see the
	// consumer's appends without a second, lock-conflicting connection. (The
	// BigQuery query path is a follow-up; prod dashboards read BQ directly.)
	var workers []lifecycle.Component
	reconcileGauge := observability.NewReconcileGauge()
	var importer *ga4import.Importer
	dw, isDuckDB := w.(*duckdb.Writer)
	if isDuckDB {
		health.Register("duckdb", func(ctx context.Context) error { return dw.DB().PingContext(ctx) })
		importer = ga4import.NewImporter(s.GA4, dw)
		importer.SetBigQueryProject(s.Warehouse.BigQueryProject)
	} else {
		logger.InfoContext(ctx, "GA4 import disabled: warehouse driver unsupported", "driver", s.Warehouse.Driver)
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	if isDuckDB {
		rpc.Register(srv, rpc.NewService(analytics.NewDuckDBRepository(dw.DB()),
			rpc.WithLogger(logger),
			rpc.WithTrackingThresholds(rpc.TrackingThresholds{
				StaleAfter:             time.Duration(s.Tracking.StaleAfterSeconds) * time.Second,
				LagP95Max:              time.Duration(s.Tracking.LagP95MaxSeconds) * time.Second,
				MissingListingMaxRatio: s.Tracking.MissingListingMaxRatio,
			}),
			rpc.WithAttributionWindowHours(s.Recs.AttributionWindowHours),
			rpc.WithGA4Reconcile(s.GA4.ReconcileMaxDiffRatio, reconcileGauge)))
		logger.InfoContext(ctx, "analytics query service enabled", "driver", s.Warehouse.Driver)
	}

	if addr := s.Metrics.Addr; addr != "" {
		mux := http.NewServeMux()
		mux.Handle("/", observability.MetricsHandler(reconcileGauge))
		if importer != nil {
			mux.Handle("/internal/ga4import", ga4import.TriggerHandler(importer))
		}
		workers = append(workers, serveHTTP(&http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}, s.ShutdownGrace, logger))
	}
	if importer != nil {
		workers = append(workers, untilDone(func(ctx context.Context) {
			ga4import.RunScheduler(ctx, importer, s.GA4.ImportAt, logger)
		}))
	}

	newClient := func(group string, topics []string, opts ...kgo.Opt) (*kgo.Client, error) {
		c, cerr := kafkax.NewConsumerClient(s.Kafka.Kafka, group, topics, opts...)
		if cerr != nil {
			return nil, cerr
		}
		stack.Push(func(context.Context) error { c.AllowRebalance(); c.Close(); return nil })
		return c, nil
	}

	mainClient, err := newClient(s.Kafka.ConsumerGroup, []string{s.Kafka.AnalyticsTopic, s.Kafka.OrderTopic})
	if err != nil {
		return fail(fmt.Errorf("kafka consumer: %w", err))
	}

	// listing.events -> listing_sellers: lets the seller funnel attribute tracking
	// events (which carry only a listing id) to a seller. DuckDB adapter only.
	// Its own group starts at the earliest offset so existing listings backfill.
	if lw, ok := w.(store.ListingSellerWriter); ok {
		c, cerr := newClient(s.Kafka.ListingConsumerGroup, []string{s.Kafka.ListingTopic},
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
		if cerr != nil {
			return fail(fmt.Errorf("kafka listing consumer: %w", cerr))
		}
		logger.InfoContext(ctx, "listing seller consumer starting",
			"listing_topic", s.Kafka.ListingTopic,
			"dlq_topic", s.Kafka.ListingDLQTopic,
			"group", s.Kafka.ListingConsumerGroup)
		lcons := events.NewListingConsumer(c, s.Kafka.ListingDLQTopic)
		workers = append(workers, untilDone(func(ctx context.Context) {
			if runErr := lcons.Run(ctx, lw, logger); runErr != nil {
				logger.ErrorContext(ctx, "listing seller consumer stopped", "err", runErr)
			}
		}))
	}

	// engagement.events -> engagement_facts (+ DLQ). DuckDB adapter only.
	if ew, ok := w.(store.EngagementFactWriter); ok {
		c, cerr := newClient(s.Engagement.ConsumerGroup, []string{s.Engagement.EventsTopic})
		if cerr != nil {
			return fail(fmt.Errorf("kafka engagement consumer: %w", cerr))
		}
		logger.InfoContext(ctx, "engagement consumer starting",
			"topic", s.Engagement.EventsTopic,
			"dlq_topic", s.Engagement.DLQTopic,
			"group", s.Engagement.ConsumerGroup)
		econs := events.NewEngagementConsumer(c, s.Engagement.DLQTopic)
		workers = append(workers, untilDone(func(ctx context.Context) {
			if runErr := econs.Run(ctx, ew, logger); runErr != nil {
				logger.ErrorContext(ctx, "engagement consumer stopped", "err", runErr)
			}
		}))
	} else {
		logger.WarnContext(ctx, "engagement consumer not started: the warehouse driver cannot store engagement facts",
			"driver", s.Warehouse.Driver,
			"topic", s.Engagement.EventsTopic)
	}

	if isDuckDB && export.Enabled(s.Warehouse.ParquetExportPath, s.Warehouse.ParquetExportIntervalSeconds) {
		interval := time.Duration(s.Warehouse.ParquetExportIntervalSeconds) * time.Second
		logger.InfoContext(ctx, "parquet export enabled",
			"path", s.Warehouse.ParquetExportPath, "interval", interval)
		workers = append(workers, untilDone(func(ctx context.Context) {
			export.Run(ctx, dw, s.Warehouse.ParquetExportPath, interval, logger)
		}))
	}

	logger.InfoContext(ctx, "analytics consumer starting",
		"analytics_topic", s.Kafka.AnalyticsTopic,
		"order_topic", s.Kafka.OrderTopic,
		"group", s.Kafka.ConsumerGroup,
		"driver", s.Warehouse.Driver,
		"batch_max_size", s.Batch.MaxSize,
		"flush_interval_seconds", s.Batch.FlushIntervalSeconds,
	)
	cons := events.New(mainClient)
	flushInterval := time.Duration(s.Batch.FlushIntervalSeconds) * time.Second
	workers = append(workers, lifecycle.Func(func(ctx context.Context) error {
		return cons.Run(ctx, w, s.Batch.MaxSize, flushInterval, logger)
	}))

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), workers...), lifecycle.Func(health.Run))
}

// serveHTTP runs srv with lifecycle.HTTP (graceful shutdown within grace). A
// listener that fails is logged, not fatal: the metrics endpoint is not worth
// the worker, so the component waits for shutdown instead of returning.
func serveHTTP(srv *http.Server, grace time.Duration, logger *slog.Logger) lifecycle.Component {
	h := lifecycle.HTTP(srv, grace)
	return lifecycle.Func(func(ctx context.Context) error {
		if err := h.Run(ctx); err != nil {
			logger.ErrorContext(ctx, "metrics server stopped", "err", err)
			<-ctx.Done()
		}
		return nil
	})
}
