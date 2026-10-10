// Package consumer runs the analytics warehouse writer: a franz-go consumer-group
// reader on `analytics.events` that maps each TrackingEvent into a driver-neutral
// record, batches records, flushes them to the WarehouseWriter, and commits
// Kafka offsets ONLY once every polled record is written or skipped (at-least-once; ADR-0002).
package consumer

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Consumer is a franz-go consumer-group client bound to one topic, with
// auto-commit disabled so offsets advance only after a durable warehouse write.
type Consumer struct {
	client pollClient
}

// pollClient is the slice of *kgo.Client the run loop needs; it lets tests drive
// Run with scripted fetches and observe commits.
type pollClient interface {
	PollFetches(ctx context.Context) kgo.Fetches
	CommitUncommittedOffsets(ctx context.Context) error
	Close()
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
//
// Offsets: CommitUncommittedOffsets commits the head of EVERYTHING polled, so it
// is called only when no polled record is still buffered in either batcher
// (commitIfDrained). Flush funcs therefore only write. Skipped records (unknown
// envelope, poison) count as handled, so a skip-only fetch also commits. One
// mutex serializes per-fetch processing with the ticker's flush+commit so the
// ticker can never commit records that were polled but not yet added.
func (c *Consumer) Run(
	ctx context.Context,
	writer warehouse.WarehouseWriter,
	batchSize int,
	flushInterval time.Duration,
	logger *slog.Logger,
) error {
	flushTracking := func(ctx context.Context, batch []*warehouse.TrackingRecord) error {
		return writer.Write(ctx, batch)
	}
	trackingBatcher := NewBatcher(batchSize, flushTracking)

	flushOrderFacts := func(ctx context.Context, batch []*warehouse.OrderFactRecord) error {
		return writer.WriteOrderFacts(ctx, batch)
	}
	orderBatcher := NewOrderFactBatcher(batchSize, flushOrderFacts)

	var mu sync.Mutex
	// commitIfDrained advances offsets only when every polled record has been
	// written or skipped. Caller must hold mu.
	commitIfDrained := func(ctx context.Context) {
		if trackingBatcher.Len() != 0 || orderBatcher.Len() != 0 {
			return
		}
		if err := c.client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("offset commit failed; will retry", slog.Any("err", err))
		}
	}

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
					mu.Lock()
					errTr := trackingBatcher.Flush(ctx)
					if errTr != nil && ctx.Err() == nil {
						logger.Warn("tracking interval flush failed; will retry", slog.Any("err", errTr))
					}
					errOf := orderBatcher.Flush(ctx)
					if errOf != nil && ctx.Err() == nil {
						logger.Warn("order facts interval flush failed; will retry", slog.Any("err", errOf))
					}
					if errTr == nil && errOf == nil {
						commitIfDrained(ctx)
					}
					mu.Unlock()
				}
			}
		}()
	}

	for {
		if ctx.Err() != nil {
			// Graceful shutdown: try to persist whatever is buffered.
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			mu.Lock()
			errTr := trackingBatcher.Flush(flushCtx)
			if errTr != nil {
				logger.Warn("final tracking flush failed", slog.Any("err", errTr))
			}
			errOf := orderBatcher.Flush(flushCtx)
			if errOf != nil {
				logger.Warn("final order facts flush failed", slog.Any("err", errOf))
			}
			if errTr == nil && errOf == nil {
				commitIfDrained(flushCtx)
			}
			mu.Unlock()
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

		mu.Lock()
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
				if cw, ok := writer.(warehouse.IngestCounterWriter); ok {
					if cerr := cw.RecordDecodeFailures(ctx, time.Now(), 1); cerr != nil && ctx.Err() == nil {
						logger.Warn("decode failure counter not updated", slog.Any("err", cerr))
					}
				}
				logger.Warn("decode record failed; skipping",
					slog.String("key", string(rec.Key)),
					slog.Any("errTr", errTr),
					slog.Any("errOf", errOf),
				)
				continue
			}

			logger.Debug("skipping unknown envelope", slog.String("key", string(rec.Key)))
		}
		if fetches.NumRecords() > 0 {
			commitIfDrained(ctx)
		}
		mu.Unlock()
	}
}

// Close shuts the client down.
func (c *Consumer) Close() { c.client.Close() }
