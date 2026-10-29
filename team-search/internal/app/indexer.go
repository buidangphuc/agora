package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/events"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/taxonomy"
	"github.com/buidangphuc/team-search/internal/tombstone"
)

const (
	indexerServiceName = "team-search-indexer"
	aiPrincipalID      = "service-team-search"
	aiClassifyScope    = "ai.classify"
	classifyTimeout    = 2 * time.Second
)

func RunIndexer(ctx context.Context, s config.IndexerSettings) (err error) {
	logger := logx.New(s.Runtime, indexerServiceName)

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, indexerServiceName)
	if err != nil {
		return fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	idx, err := index.New(s.OpenSearch.URL, s.OpenSearch.Index)
	if err != nil {
		return fmt.Errorf("opensearch: %w", err)
	}
	if err := idx.EnsureIndex(ctx); err != nil {
		return fmt.Errorf("ensure index: %w", err)
	}

	embed := retrieval.NewHTTPEmbedClient(s.Retrieval.ModelServerURL, modelTimeout)

	aiConn, err := upstream.NewClient(s.Taxonomy.AIAddr,
		upstream.AsService(aiPrincipalID, aiClassifyScope),
		upstream.WithTimeout(classifyTimeout),
	)
	if err != nil {
		return fmt.Errorf("dial team-ai at %s: %w", s.Taxonomy.AIAddr, err)
	}
	stack.Push(func(context.Context) error { return aiConn.Close() })
	classifier := taxonomy.NewGRPCClassifier(aiConn)

	dlq, err := kafkax.NewProducer(s.Kafka)
	if err != nil {
		return fmt.Errorf("dlq producer: %w", err)
	}
	stack.Push(func(context.Context) error { dlq.Close(); return nil })

	fp, err := kafkax.NewFingerprinter(s.LogKeyFP.Secret)
	if err != nil {
		return fmt.Errorf("fingerprinter: %w", err)
	}

	client, err := kafkax.NewConsumerClient(s.Kafka, s.Events.ConsumerGroup, []string{s.Events.ListingTopic})
	if err != nil {
		return fmt.Errorf("kafka consumer client: %w", err)
	}
	stack.Push(func(context.Context) error { client.Close(); return nil })

	handler := events.NewListingEventHandler(idx, embed, classifier)
	cons, err := newIndexerConsumer(client, handler, dlq, s.Events.ListingTopic, fp, logger)
	if err != nil {
		return err
	}

	purge, err := lifecycle.Every(s.Tombstone.PurgeInterval, func(ctx context.Context) error {
		return tombstone.Purge(ctx, idx, s.Tombstone.TTL, logger, nil)
	}, lifecycle.WithLogger(logger))
	if err != nil {
		return err
	}

	logger.InfoContext(ctx, "indexer consuming",
		"topic", s.Events.ListingTopic,
		"group", s.Events.ConsumerGroup,
		"opensearch_index", s.OpenSearch.Index,
	)
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace,
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	}, cons, purge)
}

func newIndexerConsumer(client *kgo.Client, h consume.Handler, dlq consume.DLQProducer, topic string, fp *kafkax.Fingerprinter, logger *slog.Logger) (*consume.Consumer, error) {
	return consume.NewConsumer(client, h, dlq, consume.Options{
		DLQTopic: topic + ".dlq",
		KeyFP:    fp,
		Logger:   logger,
	})
}
