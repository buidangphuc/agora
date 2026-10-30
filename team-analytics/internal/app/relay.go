package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/ga4relay"
)

func RunGA4Relay(ctx context.Context, s config.Settings) (err error) {
	logger := logx.New(s.Runtime, relayService)
	slog.SetDefault(logger)

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, relayService)
	if err != nil {
		return fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthInterval(), logger)
	if err != nil {
		return err
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})

	var workers []lifecycle.Component
	if s.GA4RelayEnabled() {
		w, wErr := buildRelayWorkers(ctx, s, &stack, health)
		if wErr != nil {
			return wErr
		}
		workers = w
		logger.InfoContext(ctx, "ga4 relay starting", "group", s.GA4.RelayConsumerGroup, "dlq_topic", s.GA4.RelayDLQTopic)
	} else {
		logger.InfoContext(ctx, "ga4 relay disabled")
	}

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
		CloseGrace:    closeGrace,
		DrainDelay:    s.DrainDelay,
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), workers...), health)
}

func buildRelayWorkers(ctx context.Context, s config.Settings, stack *lifecycle.Stack, health *healthx.Health) ([]lifecycle.Component, error) {
	logger := logx.New(s.Runtime, relayService)

	fp, err := kafkax.NewFingerprinter(s.LogKeyFP.Secret)
	if err != nil {
		return nil, fmt.Errorf("fingerprinter: %w", err)
	}

	db, err := sql.Open("duckdb", s.Warehouse.DuckDBPath)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	stack.Push(func(context.Context) error { return db.Close() })
	health.Register("duckdb", func(ctx context.Context) error { return pingDuckDB(ctx, db) })

	ledger, err := ga4relay.NewSQLLedger(ctx, db, time.Now)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", err)
	}
	mappings, err := ga4relay.NewSQLMappingStore(ctx, db, time.Now)
	if err != nil {
		return nil, fmt.Errorf("mapping store: %w", err)
	}
	purgers := []ga4relay.Purger{ledger.(ga4relay.Purger), mappings.(ga4relay.Purger)}
	if hooks.WrapLedger != nil {
		ledger = hooks.WrapLedger(ledger)
	}

	producer, err := kafkax.NewProducer(s.Kafka.Kafka)
	if err != nil {
		return nil, fmt.Errorf("dlq producer: %w", err)
	}
	stack.Push(func(context.Context) error { producer.Close(); return nil })
	redacting := events.NewRedactingDLQ(producer, fp, logger)
	if hooks.DLQProducer != nil {
		hooks.DLQProducer("relay", redacting)
	}

	relay := ga4relay.NewRelay(ga4relay.RelayConfig{
		Mappings: mappings,
		Ledger:   ledger,
		Sender: ga4relay.NewClient(ga4relay.ClientConfig{
			Endpoint: s.GA4.Endpoint, MeasurementID: s.GA4.MeasurementID, APISecret: s.GA4.APISecret, Logger: logger,
		}),
		Now:    time.Now,
		Logger: logger,
		KeyFP:  fp,
	})
	if hooks.Relay != nil {
		hooks.Relay(relay)
	}
	mapping := ga4relay.NewMappingHandler(mappings)

	client, err := newGroupClient(s, stack, s.GA4.RelayConsumerGroup, []string{s.Kafka.AnalyticsTopic, s.Kafka.OrderTopic})
	if err != nil {
		return nil, err
	}
	opts := ga4relay.ConsumerOptions(s)
	opts.KeyFP, opts.Logger = fp, logger
	cons, err := consume.NewConsumer(client, func(ctx context.Context, rec consume.Record) error {
		switch rec.Topic {
		case s.Kafka.AnalyticsTopic:
			return mapping.Handle(ctx, rec)
		case s.Kafka.OrderTopic:
			return relay.Handle(ctx, rec)
		}
		return nil
	}, redacting, opts)
	if err != nil {
		return nil, fmt.Errorf("relay consumer: %w", err)
	}

	purge, err := lifecycle.Every(purgeInterval, func(ctx context.Context) error {
		for _, p := range purgers {
			n, pErr := p.Purge(ctx)
			if pErr != nil {
				return fmt.Errorf("ga4 relay retention purge: %w", pErr)
			}
			logger.InfoContext(ctx, "ga4 relay retention purge", "deleted", n)
		}
		return nil
	}, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return nil, err
	}
	return []lifecycle.Component{cons, purge}, nil
}
