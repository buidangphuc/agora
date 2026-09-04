// Package fake provides an in-memory WarehouseWriter for black-box tests: it
// records every batch it is asked to write and can be made to fail on demand, so
// tests can assert the consumer's map→batch→flush→offset-commit behavior without
// a real DuckDB/BigQuery backend.
package fake

import (
	"context"
	"sync"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Writer is a WarehouseWriter that appends batches into an in-memory slice.
type Writer struct {
	mu sync.Mutex
	// FailWith, when non-nil, is returned by Write and no rows are recorded —
	// simulating a flush failure so the caller must NOT advance offsets.
	FailWith error
	rows     []*warehouse.TrackingRecord
	batches  int
	closed   bool
}

// New returns an empty fake writer.
func New() *Writer { return &Writer{} }

// Write records the batch (or returns FailWith without recording).
func (w *Writer) Write(_ context.Context, batch []*warehouse.TrackingRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.FailWith != nil {
		return w.FailWith
	}
	w.rows = append(w.rows, batch...)
	w.batches++
	return nil
}

// Close marks the writer closed.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

// Rows returns a copy of all successfully written rows, in write order.
func (w *Writer) Rows() []*warehouse.TrackingRecord {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*warehouse.TrackingRecord, len(w.rows))
	copy(out, w.rows)
	return out
}

// Batches returns how many successful Write calls happened (i.e. flush count).
func (w *Writer) Batches() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.batches
}

// Closed reports whether Close was called.
func (w *Writer) Closed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

// compile-time assertion that the fake satisfies the seam.
var _ warehouse.WarehouseWriter = (*Writer)(nil)
