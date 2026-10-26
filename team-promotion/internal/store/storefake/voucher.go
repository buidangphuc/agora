package storefake

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
)

// InMemoryVoucherRepository is the DB-less backend used when DATABASE_ENABLED=false
// and in unit tests.
type InMemoryVoucherRepository struct {
	mu       sync.RWMutex
	vouchers map[string]store.Voucher // keyed by id
}

func NewInMemoryVoucherRepository() *InMemoryVoucherRepository {
	return &InMemoryVoucherRepository{vouchers: make(map[string]store.Voucher)}
}

func (r *InMemoryVoucherRepository) Create(_ context.Context, v store.Voucher, event func(store.Voucher) (outbox.Row, error)) (store.Voucher, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v.ID == "" {
		v.ID = store.NewID()
	}
	for _, existing := range r.vouchers {
		if existing.Code == v.Code {
			return store.Voucher{}, fmt.Errorf("voucher code %q already exists", v.Code)
		}
	}
	if _, err := event(v); err != nil {
		return store.Voucher{}, err
	}
	r.vouchers[v.ID] = v
	return v, nil
}

func (r *InMemoryVoucherRepository) GetByCode(_ context.Context, code string) (store.Voucher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.vouchers {
		if v.Code == code {
			return v, nil
		}
	}
	return store.Voucher{}, store.ErrVoucherNotFound
}

func (r *InMemoryVoucherRepository) GetByID(_ context.Context, id string) (store.Voucher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.vouchers[id]
	if !ok {
		return store.Voucher{}, store.ErrVoucherNotFound
	}
	return v, nil
}

func (r *InMemoryVoucherRepository) List(_ context.Context, sellerID string, limit, offset int) ([]store.Voucher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []store.Voucher
	for _, v := range r.vouchers {
		if v.SellerID == sellerID {
			all = append(all, v)
		}
	}
	// Deterministic order (by code) so pagination is stable in the DB-less path.
	sort.Slice(all, func(i, j int) bool { return all[i].Code < all[j].Code })
	if offset >= len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (r *InMemoryVoucherRepository) IncrementUsed(_ context.Context, id string, delta int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.vouchers[id]
	if !ok {
		return store.ErrVoucherNotFound
	}
	v.Used += delta
	r.vouchers[id] = v
	return nil
}

func (r *InMemoryVoucherRepository) Ping(_ context.Context) error { return nil }
