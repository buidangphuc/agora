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

// New dials the brokers and joins the consumer group for topic. Auto-commit is
// disabled: the flush path commits explicitly after each successful write.
func New(brokers []string, group, topic string) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
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
//
// Offset discipline: the flush closure commits uncommitted offsets ONLY after
// writer.Write succeeds. Because the Batcher always flushes its entire buffer
// (every polled-but-uncommitted record) before the commit runs, a commit never
// races ahead of a durable write — a crash mid-batch reprocesses rather than
// loses events.
func (c *Consumer) Run(
	ctx context.Context,
	writer warehouse.WarehouseWriter,
	batchSize int,
	flushInterval time.Duration,
	logger *slog.Logger,
) error {
	flush := func(ctx context.Context, batch []*warehouse.TrackingRecord) error {
		if err := writer.Write(ctx, batch); err != nil {
			return err
		}
		// Durable write succeeded → it is now safe to advance offsets.
		return c.client.CommitUncommittedOffsets(ctx)
	}
	batcher := NewBatcher(batchSize, flush)

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
					if err := batcher.Flush(ctx); err != nil && ctx.Err() == nil {
						logger.Warn("interval flush failed; will retry", slog.Any("err", err))
					}
				}
			}
		}()
	}

	for {
		if ctx.Err() != nil {
			// Graceful shutdown: try to persist whatever is buffered.
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := batcher.Flush(flushCtx); err != nil {
				logger.Warn("final flush failed", slog.Any("err", err))
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
			tr, ok, err := RecordFromEnvelope(rec.Value)
			if err != nil {
				// Poison record: log and move on (offset advances with the batch).
				logger.Warn("decode record failed; skipping",
					slog.String("key", string(rec.Key)),
					slog.Any("err", err),
				)
				continue
			}
			if !ok {
				logger.Debug("skipping non-tracking envelope", slog.String("key", string(rec.Key)))
				continue
			}
			if err := batcher.Add(ctx, tr); err != nil && ctx.Err() == nil {
				logger.Warn("batch flush failed; records retained for retry", slog.Any("err", err))
			}
		}
	}
}

// Close shuts the client down.
func (c *Consumer) Close() { c.client.Close() }
