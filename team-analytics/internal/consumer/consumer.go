// Package consumer runs the analytics warehouse writer: a franz-go consumer-group
// reader on `analytics.events` that maps each TrackingEvent into a driver-neutral
// record, batches records, flushes them to the WarehouseWriter, and commits
// Kafka offsets ONLY after a successful flush (at-least-once; ADR-0002).
package consumer

import (
	"context"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Consumer is a franz-go consumer-group client bound to one topic, with
// auto-commit disabled so offsets advance only after a durable warehouse write.
type Consumer struct {
	client *kgo.Client
}

// New dials the brokers and joins the consumer group for topics. Auto-commit is
// disabled: the flush path commits explicitly after each successful write.
func New(brokers []string, group string, topics ...string) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, err
	}
	return &Consumer{client: client}, nil
}

// Run polls records, maps + batches them, and flushes to writer. It returns when
// ctx is cancelled, doing a best-effort final flush so a partial batch is not
// lost on graceful shutdown.
func (c *Consumer) Run(
	ctx context.Context,
	writer warehouse.WarehouseWriter,
	batchSize int,
	flushInterval time.Duration,
	logger *slog.Logger,
) error {
	flushTracking := func(ctx context.Context, batch []*warehouse.TrackingRecord) error {
		if err := writer.Write(ctx, batch); err != nil {
			return err
		}
		// Durable write succeeded → it is now safe to advance offsets.
		return c.client.CommitUncommittedOffsets(ctx)
	}
	trackingBatcher := NewBatcher(batchSize, flushTracking)

	flushOrderFacts := func(ctx context.Context, batch []*warehouse.OrderFactRecord) error {
		if err := writer.WriteOrderFacts(ctx, batch); err != nil {
			return err
		}
		return c.client.CommitUncommittedOffsets(ctx)
	}
	orderBatcher := NewOrderFactBatcher(batchSize, flushOrderFacts)

	// Interval flush: bound how long a partial batch lingers (design.md).
	if flushInterval > 0 {
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := trackingBatcher.Flush(ctx); err != nil && ctx.Err() == nil {
						logger.Warn("tracking interval flush failed; will retry", slog.Any("err", err))
					}
					if err := orderBatcher.Flush(ctx); err != nil && ctx.Err() == nil {
						logger.Warn("order facts interval flush failed; will retry", slog.Any("err", err))
					}
				}
			}
		}()
	}

	for {
		if ctx.Err() != nil {
			// Graceful shutdown: try to persist whatever is buffered.
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := trackingBatcher.Flush(flushCtx); err != nil {
				logger.Warn("final tracking flush failed", slog.Any("err", err))
			}
			if err := orderBatcher.Flush(flushCtx); err != nil {
				logger.Warn("final order facts flush failed", slog.Any("err", err))
			}
			cancel()
			return nil
		}

		fetches := c.client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			// ctx cancellation surfaces here as a fetch error on shutdown; the
			// loop top handles the graceful flush + return.
			if ctx.Err() != nil {
				continue
			}
			logger.Warn("kafka fetch error", slog.Any("errs", errs))
			continue
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()

			tr, okTr, errTr := RecordFromEnvelope(rec.Value)
			if okTr {
				if err := trackingBatcher.Add(ctx, tr); err != nil && ctx.Err() == nil {
					logger.Warn("tracking batch flush failed; records retained for retry", slog.Any("err", err))
				}
				continue
			}

			of, okOf, errOf := OrderFactsFromEnvelope(rec.Value)
			if okOf {
				if err := orderBatcher.Add(ctx, of...); err != nil && ctx.Err() == nil {
					logger.Warn("order facts batch flush failed; records retained for retry", slog.Any("err", err))
				}
				continue
			}

			if errTr != nil && errOf != nil {
				// Poison record: log and move on (offset advances with the batch).
				logger.Warn("decode record failed; skipping",
					slog.String("key", string(rec.Key)),
					slog.Any("errTr", errTr),
					slog.Any("errOf", errOf),
				)
				continue
			}

			logger.Debug("skipping unknown envelope", slog.String("key", string(rec.Key)))
		}
	}
}

// Close shuts the client down.
func (c *Consumer) Close() { c.client.Close() }

