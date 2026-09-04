// Command indexer is the Kafka consumer that keeps the OpenSearch read-model in
// sync: it reads listing.events and upserts/deletes documents (ADR-0002/0005).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-search/internal/bootstrap"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/consumer"
	"github.com/buidangphuc/team-search/internal/observability"
)

func main() {
	if err := run(); err != nil {
		slog.Error("indexer exited with error", slog.Any("err", err))
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
		return errors.New("KAFKA_ENABLED must be true to run the indexer")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	res, err := bootstrap.OpenResources(ctx, settings)
	if err != nil {
		return fmt.Errorf("open resources: %w", err)
	}
	defer func() { _ = bootstrap.CloseResources(context.Background(), res) }()

	cons, err := consumer.New(settings.KafkaBrokers(), settings.Kafka.ConsumerGroup, settings.Kafka.ListingTopic)
	if err != nil {
		return fmt.Errorf("kafka consumer: %w", err)
	}
	defer cons.Close()

	logger.Info("indexer consuming",
		slog.String("topic", settings.Kafka.ListingTopic),
		slog.String("group", settings.Kafka.ConsumerGroup),
		slog.String("opensearch_index", settings.OpenSearch.Index),
	)
	return cons.Run(ctx, consumer.ListingEventHandler(res.Index), logger)
}
