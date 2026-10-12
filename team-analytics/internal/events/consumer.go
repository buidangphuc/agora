// Package events runs the analytics warehouse writer: a franz-go consumer-group
// reader on `analytics.events` that maps each TrackingEvent into a driver-neutral
// record, batches records, flushes them to the WarehouseWriter, and commits
// Kafka offsets ONLY once every polled record is written or skipped (at-least-once; ADR-0002).
package events

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/team-analytics/internal/store"
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

// New wraps a consumer-group client. The client must have auto-commit disabled
// (kafkax.NewConsumerClient does that): the flush path commits explicitly after
// each successful write.
func New(client *kgo.Client) *Consumer {
	return &Consumer{client: client}
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
	writer store.WarehouseWriter,
	batchSize int,
	flushInterval time.Duration,
	logger *slog.Logger,
) error {
	flushTracking := func(ctx context.Context, batch []*store.TrackingRecord) error {
		return writer.Write(ctx, batch)
	}
	trackingBatcher := NewBatcher(batchSize, flushTracking)

	flushOrderFacts := func(ctx context.Context, batch []*store.OrderFactRecord) error {
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
			logger.WarnContext(ctx, "offset commit failed; will retry", "err", err)
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
						logger.WarnContext(ctx, "tracking interval flush failed; will retry", "err", errTr)
					}
					errOf := orderBatcher.Flush(ctx)
					if errOf != nil && ctx.Err() == nil {
						logger.WarnContext(ctx, "order facts interval flush failed; will retry", "err", errOf)
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
			flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			mu.Lock()
			errTr := trackingBatcher.Flush(flushCtx)
			if errTr != nil {
				logger.WarnContext(ctx, "final tracking flush failed", "err", errTr)
			}
			errOf := orderBatcher.Flush(flushCtx)
			if errOf != nil {
				logger.WarnContext(ctx, "final order facts flush failed", "err", errOf)
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
			logger.WarnContext(ctx, "kafka fetch error", "errs", errs)
			continue
		}

		mu.Lock()
		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()

			tr, okTr, errTr := RecordFromEnvelope(rec.Value)
			if okTr {
				if err := trackingBatcher.Add(ctx, tr); err != nil && ctx.Err() == nil {
					logger.WarnContext(ctx, "tracking batch flush failed; records retained for retry", "err", err)
				}
				continue
			}

			of, okOf, errOf := OrderFactsFromEnvelope(rec.Value)
			if okOf {
				if err := orderBatcher.Add(ctx, of...); err != nil && ctx.Err() == nil {
					logger.WarnContext(ctx, "order facts batch flush failed; records retained for retry", "err", err)
				}
				continue
			}

			if errTr != nil && errOf != nil {
				// Poison record: log and move on (offset advances with the batch).
				if cw, ok := writer.(store.IngestCounterWriter); ok {
					if cerr := cw.RecordDecodeFailures(ctx, time.Now(), 1); cerr != nil && ctx.Err() == nil {
						logger.WarnContext(ctx, "decode failure counter not updated", "err", cerr)
					}
				}
				logger.WarnContext(ctx, "decode record failed; skipping",
					"key", string(rec.Key),
					"errTr", errTr,
					"errOf", errOf,
				)
				continue
			}

			logger.DebugContext(ctx, "skipping unknown envelope", "key", string(rec.Key))
		}
		if fetches.NumRecords() > 0 {
			commitIfDrained(ctx)
		}
		mu.Unlock()
	}
}

// Close shuts the client down.
func (c *Consumer) Close() { c.client.Close() }
