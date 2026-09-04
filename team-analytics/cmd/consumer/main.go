// Command consumer is the analytics warehouse writer: a Kafka consumer that
// reads `analytics.events`, unmarshals the TrackingEvent out of each
// EventEnvelope, and appends it to the warehouse (DuckDB local / BigQuery prod)
// behind the WarehouseWriter seam. It serves NO business RPC — only a gRPC
// health server for k8s probes (ADR-0002 / AGENTS.md §6c worker).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/bootstrap"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/consumer"
	"github.com/buidangphuc/team-analytics/internal/grpcserver"
	"github.com/buidangphuc/team-analytics/internal/observability"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

func main() {
	if err := run(); err != nil {
		slog.Error("analytics consumer exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	logger := observability.NewLogger(settings)

	if !settings.Kafka.Enabled {
		return errors.New("KAFKA_ENABLED must be true to run the analytics consumer")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	res, err := bootstrap.OpenResources(ctx, settings)
	if err != nil {
		return fmt.Errorf("open resources: %w", err)
	}
	defer func() { _ = bootstrap.CloseResources(context.Background(), res) }()

	// gRPC server: always health (probes); plus the read-only seller-analytics
	// query service when the warehouse is the local DuckDB adapter, sharing the
	// writer's in-process handle so reads see the consumer's appends without a
	// second, lock-conflicting connection. (The BigQuery query path is a
	// follow-up; prod dashboards read BQ directly.)
	var queryServer analyticsv1.AnalyticsQueryServiceServer
	if w, ok := res.Writer.(*duckdb.Writer); ok {
		queryServer = query.NewService(query.NewDuckDBRepository(w.DB()))
		logger.Info("analytics query service enabled", slog.String("driver", settings.Warehouse.Driver))
	}
	srv := grpcserver.Build(settings, res.Health, queryServer)
	addr := net.JoinHostPort(settings.Server.Host, fmt.Sprintf("%d", settings.Server.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen health %s: %w", addr, err)
	}
	go func() {
		logger.Info("health server listening", slog.String("addr", addr))
		if serveErr := srv.Serve(lis); serveErr != nil {
			logger.Error("health server stopped", slog.Any("err", serveErr))
		}
	}()
	defer srv.GracefulStop()

	cons, err := consumer.New(settings.KafkaBrokers(), settings.Kafka.ConsumerGroup, settings.Kafka.AnalyticsTopic)
	if err != nil {
		return fmt.Errorf("kafka consumer: %w", err)
	}
	defer cons.Close()

	logger.Info("analytics consumer starting",
		slog.String("topic", settings.Kafka.AnalyticsTopic),
		slog.String("group", settings.Kafka.ConsumerGroup),
		slog.String("driver", settings.Warehouse.Driver),
		slog.Int("batch_max_size", settings.Batch.MaxSize),
		slog.Int("flush_interval_seconds", settings.Batch.FlushIntervalSeconds),
	)

	flushInterval := time.Duration(settings.Batch.FlushIntervalSeconds) * time.Second
	return cons.Run(ctx, res.Writer, settings.Batch.MaxSize, flushInterval, logger)
}
