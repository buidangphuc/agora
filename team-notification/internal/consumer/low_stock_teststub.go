package consumer

// TEST-STUB: compile-only placeholders so low_stock_test.go builds before task 3
// is implemented. The implementer deletes this file and defines these symbols in
// listing.go (real LowStockStateStore, in-memory default, options, handling).

import "context"

// LowStockStateStore is the per-listing durable low-stock state.
type LowStockStateStore interface {
	Get(ctx context.Context, listingID string) (sellerID, title string, low, known bool, err error)
	Upsert(ctx context.Context, listingID, sellerID, title string, low bool) error
	SetLow(ctx context.Context, listingID string, low bool) error
	Delete(ctx context.Context, listingID string) error
}

type stubLowStockStore struct{}

func NewInMemoryLowStockStore() LowStockStateStore { return stubLowStockStore{} }

func (stubLowStockStore) Get(context.Context, string) (string, string, bool, bool, error) {
	return "", "", false, false, nil
}
func (stubLowStockStore) Upsert(context.Context, string, string, string, bool) error { return nil }
func (stubLowStockStore) SetLow(context.Context, string, bool) error                 { return nil }
func (stubLowStockStore) Delete(context.Context, string) error                       { return nil }

func WithLowStockStore(LowStockStateStore) Option { return func(*ListingConsumer) {} }

func WithLowStockThreshold(int32) Option { return func(*ListingConsumer) {} }
