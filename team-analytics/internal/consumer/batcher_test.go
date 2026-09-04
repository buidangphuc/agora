package consumer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-analytics/internal/consumer"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/fake"
)

func rec(id string) *warehouse.TrackingRecord { return &warehouse.TrackingRecord{EventID: id} }

// TestFlushOnSize: the buffer flushes exactly when it reaches maxSize.
func TestFlushOnSize(t *testing.T) {
	var flushed [][]*warehouse.TrackingRecord
	b := consumer.NewBatcher(2, func(_ context.Context, batch []*warehouse.TrackingRecord) error {
		flushed = append(flushed, batch)
		return nil
	})
	ctx := context.Background()

	if err := b.Add(ctx, rec("e1")); err != nil {
		t.Fatalf("Add e1: %v", err)
	}
	if len(flushed) != 0 {
		t.Fatalf("flushed before reaching size: %d", len(flushed))
	}
	if err := b.Add(ctx, rec("e2")); err != nil {
		t.Fatalf("Add e2: %v", err)
	}
	if len(flushed) != 1 || len(flushed[0]) != 2 {
		t.Fatalf("expected one flush of 2, got %v", flushed)
	}
	if b.Len() != 0 {
		t.Errorf("buffer not cleared after flush: %d", b.Len())
	}
}

// TestFlushOnInterval: an explicit Flush (as the interval ticker calls) writes a
// partial batch so it is not held indefinitely.
func TestFlushOnInterval(t *testing.T) {
	var count int
	b := consumer.NewBatcher(500, func(_ context.Context, batch []*warehouse.TrackingRecord) error {
		count += len(batch)
		return nil
	})
	ctx := context.Background()
	_ = b.Add(ctx, rec("e1"))
	if count != 0 {
		t.Fatalf("flushed before interval: %d", count)
	}
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if count != 1 {
		t.Errorf("interval flush wrote %d, want 1", count)
	}
	// Flushing again is a no-op (buffer empty).
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("empty Flush: %v", err)
	}
	if count != 1 {
		t.Errorf("empty flush changed count to %d", count)
	}
}

// TestFlushErrorRetainsBuffer: a failed flush must not drop records — they stay
// buffered and are written on the next successful flush.
func TestFlushErrorRetainsBuffer(t *testing.T) {
	failing := true
	var written int
	b := consumer.NewBatcher(1, func(_ context.Context, batch []*warehouse.TrackingRecord) error {
		if failing {
			return errors.New("warehouse down")
		}
		written += len(batch)
		return nil
	})
	ctx := context.Background()

	if err := b.Add(ctx, rec("e1")); err == nil {
		t.Fatal("expected flush error to propagate")
	}
	if b.Len() != 1 {
		t.Fatalf("records dropped on flush failure: len=%d", b.Len())
	}
	failing = false
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("retry flush: %v", err)
	}
	if written != 1 || b.Len() != 0 {
		t.Errorf("retained record not written on retry: written=%d len=%d", written, b.Len())
	}
}

// TestOffsetCommittedOnlyAfterWrite models the consumer's flush closure
// (writer.Write → commit) and asserts the commit never runs when the write
// fails — the at-least-once guarantee the spec requires.
func TestOffsetCommittedOnlyAfterWrite(t *testing.T) {
	w := fake.New()
	var commits int
	commit := func() error { commits++; return nil }

	flush := func(ctx context.Context, batch []*warehouse.TrackingRecord) error {
		if err := w.Write(ctx, batch); err != nil {
			return err
		}
		return commit()
	}
	b := consumer.NewBatcher(1, flush)
	ctx := context.Background()

	// Warehouse failing → write errors → commit must NOT run.
	w.FailWith = errors.New("boom")
	if err := b.Add(ctx, rec("e1")); err == nil {
		t.Fatal("expected write error")
	}
	if commits != 0 {
		t.Fatalf("offset committed despite failed write: commits=%d", commits)
	}

	// Warehouse healthy → write succeeds → commit runs once.
	w.FailWith = nil
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("flush after recovery: %v", err)
	}
	if commits != 1 {
		t.Errorf("expected exactly one commit after successful write, got %d", commits)
	}
	if len(w.Rows()) != 1 {
		t.Errorf("expected 1 row durably written, got %d", len(w.Rows()))
	}
}
