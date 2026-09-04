package consumer

import (
	"context"
	"sync"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// FlushFunc durably writes a batch AND, on success, advances the source of truth
// (the consumer wires this to warehouse.Write followed by a Kafka offset commit).
// Returning an error means the batch was NOT durably written: the Batcher keeps
// the buffer so nothing is dropped, and the caller must not have advanced
// offsets (at-least-once — a crash reprocesses).
type FlushFunc func(ctx context.Context, batch []*warehouse.TrackingRecord) error

// Batcher accumulates records and flushes on whichever comes first: the buffer
// reaching maxSize, or an external interval tick (Flush). It is deliberately
// transport-agnostic — it knows nothing about Kafka — so it is unit-testable
// with a plain FlushFunc.
type Batcher struct {
	mu      sync.Mutex
	buf     []*warehouse.TrackingRecord
	maxSize int
	flush   FlushFunc
}

// NewBatcher returns a Batcher that flushes when the buffer reaches maxSize
// (maxSize <= 0 is treated as 1 — flush every record).
func NewBatcher(maxSize int, flush FlushFunc) *Batcher {
	if maxSize <= 0 {
		maxSize = 1
	}
	return &Batcher{maxSize: maxSize, flush: flush}
}

// Add appends a record and flushes synchronously if the buffer is now full. A
// flush error is propagated and the buffer is retained (not dropped).
func (b *Batcher) Add(ctx context.Context, rec *warehouse.TrackingRecord) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, rec)
	if len(b.buf) >= b.maxSize {
		return b.flushLocked(ctx)
	}
	return nil
}

// Flush writes whatever is buffered (a no-op when empty). Called by the interval
// ticker and on graceful shutdown so a partial batch is not held indefinitely.
func (b *Batcher) Flush(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked(ctx)
}

// Len reports the current buffered count (for tests/metrics).
func (b *Batcher) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}

// flushLocked flushes under b.mu. On success the buffer is cleared; on failure
// the buffer is kept intact so the records are retried on the next flush and no
// offsets are advanced for them.
func (b *Batcher) flushLocked(ctx context.Context) error {
	if len(b.buf) == 0 {
		return nil
	}
	if err := b.flush(ctx, b.buf); err != nil {
		return err
	}
	// Release the flushed slice so it can be GC'd; start a fresh buffer.
	b.buf = nil
	return nil
}
