package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/export"
	"github.com/buidangphuc/team-analytics/internal/ga4import"
	"github.com/buidangphuc/team-analytics/internal/observability"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/rpc"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

const metricsReadHeaderTimeout = 5 * time.Second

func RunConsumer(ctx context.Context, s config.Settings) (err error) {
	logger := logx.New(s.Runtime, consumerService)
	slog.SetDefault(logger)

	mocks, err := mockdata.New(s.MockData, "ga4export")
	if err != nil {
		return err
	}
	mocks.LogActive(logger)

	fp, err := kafkax.NewFingerprinter(s.LogKeyFP.Secret)
	if err != nil {
		return fmt.Errorf("fingerprinter: %w", err)
	}

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, consumerService)
	if err != nil {
		return fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	w, err := openWarehouse(ctx, s)
	if err != nil {
		return fmt.Errorf("open warehouse: %w", err)
	}
	stack.Push(func(context.Context) error { return w.Close() })

	health, err := healthx.New(healthInterval(), logger)
	if err != nil {
		return err
	}
	dw, isDuckDB := w.(*duckdb.Writer)
	if isDuckDB {
		health.Register("duckdb", func(ctx context.Context) error { return pingDuckDB(ctx, dw.DB()) })
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	gauge := observability.NewReconcileGauge()
	if isDuckDB {
		h, hErr := rpc.NewHandler(query.NewDuckDBRepository(dw.DB()), logger,
			rpc.WithTrackingThresholds(rpc.TrackingThresholds{
				StaleAfter:             time.Duration(s.Tracking.StaleAfterSeconds) * time.Second,
				LagP95Max:              time.Duration(s.Tracking.LagP95MaxSeconds) * time.Second,
				MissingListingMaxRatio: s.Tracking.MissingListingMaxRatio,
			}),
			rpc.WithAttributionWindowHours(s.Recs.AttributionWindowHours),
			rpc.WithGA4Reconcile(s.GA4.ReconcileMaxDiffRatio, gauge),
		)
		if hErr != nil {
			return hErr
		}
		rpc.Register(srv, h)
	}

	producer, err := kafkax.NewProducer(s.Kafka.Kafka)
	if err != nil {
		return fmt.Errorf("dlq producer: %w", err)
	}
	stack.Push(func(context.Context) error { producer.Close(); return nil })
	redacting := events.NewRedactingDLQ(producer, fp, logger)
	if hooks.DLQProducer != nil {
		hooks.DLQProducer("consumer", redacting)
	}

	var workers []lifecycle.Component

	mainSpec := events.MainOptions(s)
	mainClient, err := newGroupClient(s, &stack, mainSpec.Group, mainSpec.Topics, kgo.FetchMaxWait(mainSpec.FetchMaxWait))
	if err != nil {
		return err
	}
	mainCons, err := newBatchConsumer(mainClient, events.NewBatchHandler(w, fp, logger), redacting, mainSpec.Options, fp, logger)
	if err != nil {
		return err
	}
	workers = append(workers, mainCons)

	if lw, ok := w.(warehouse.ListingSellerWriter); ok {
		spec := events.ListingSpec(s)
		client, cErr := newGroupClient(s, &stack, spec.Group, spec.Topics, kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
		if cErr != nil {
			return cErr
		}
		cons, cErr := newBatchConsumer(client, events.NewListingHandler(lw, fp, logger), producer, spec.Options, fp, logger)
		if cErr != nil {
			return cErr
		}
		workers = append(workers, cons)
	} else {
		logger.WarnContext(ctx, "listing consumer not started: the warehouse driver cannot store listing sellers", "driver", s.Warehouse.Driver)
	}

	if ew, ok := w.(warehouse.EngagementFactWriter); ok {
		spec := events.EngagementOptions(s)
		client, cErr := newGroupClient(s, &stack, spec.Group, spec.Topics)
		if cErr != nil {
			return cErr
		}
		opts := spec.Options
		opts.KeyFP, opts.Logger = fp, logger
		cons, cErr := consume.NewConsumer(client, events.NewEngagementHandler(ew, fp, logger), producer, opts)
		if cErr != nil {
			return fmt.Errorf("engagement consumer: %w", cErr)
		}
		workers = append(workers, cons)
	} else {
		logger.WarnContext(ctx, "engagement consumer not started: the warehouse driver cannot store engagement facts", "driver", s.Warehouse.Driver)
	}

	mux := http.NewServeMux()
	mux.Handle("/", observability.MetricsHandler(gauge))
	if isDuckDB {
		imp := ga4import.NewImporter(s.GA4, mocks, dw)
		imp.SetBigQueryProject(s.Warehouse.BigQueryProject)
		mux.Handle("/internal/ga4import", ga4import.TriggerHandler(imp))
		workers = append(workers, untilDone("ga4 import scheduler", logger, func(ctx context.Context) error {
			imp.RunScheduler(ctx, logger)
			return nil
		}))
	}
	if s.Metrics.Addr != "" {
		workers = append(workers, lifecycle.HTTP(&http.Server{
			Addr:              s.Metrics.Addr,
			Handler:           mux,
			ReadHeaderTimeout: metricsReadHeaderTimeout,
		}, s.ShutdownGrace))
	}
	if isDuckDB && export.Enabled(s.Warehouse.ParquetExportPath, s.Warehouse.ParquetExportIntervalSeconds) {
		interval := time.Duration(s.Warehouse.ParquetExportIntervalSeconds) * time.Second
		workers = append(workers, untilDone("parquet export", logger, func(ctx context.Context) error {
			export.Run(ctx, dw, s.Warehouse.ParquetExportPath, interval, logger)
			return nil
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
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), workers...), health)
}

func newBatchConsumer(client *kgo.Client, h consume.BatchHandler, dlq consume.DLQProducer, opts consume.BatchOptions, fp *kafkax.Fingerprinter, logger *slog.Logger) (*consume.BatchConsumer, error) {
	opts.KeyFP, opts.Logger = fp, logger
	cons, err := consume.NewBatchConsumer(client, h, dlq, opts)
	if err != nil {
		return nil, fmt.Errorf("batch consumer for %s: %w", opts.DLQTopic, err)
	}
	return cons, nil
}
