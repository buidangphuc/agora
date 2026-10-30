package fake

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/team-order/internal/store"
)

// InMemoryReturnRepository
type InMemoryReturnRepository struct {
	mu               sync.RWMutex
	returns          map[string]store.OrderReturn
	autoApproveAfter time.Duration
}

func NewInMemoryReturnRepository(opts ...store.ReturnRepoOption) *InMemoryReturnRepository {
	return &InMemoryReturnRepository{
		returns:          make(map[string]store.OrderReturn),
		autoApproveAfter: store.ResolveReturnAutoApproveAfter(opts...),
	}
}

func (r *InMemoryReturnRepository) CreateReturn(_ context.Context, ret store.OrderReturn) (store.OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	store.PrepareReturn(&ret, r.autoApproveAfter)
	r.returns[ret.ID] = ret
	return ret, nil
}

func (r *InMemoryReturnRepository) CreateReturnCapped(_ context.Context, ret store.OrderReturn, orderTotal int64) (store.OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var taken int64
	for _, existing := range r.returns {
		if existing.OrderID == ret.OrderID && existing.Status != store.ReturnStatusRejected {
			taken += existing.RefundAmount
		}
	}
	amount, err := store.ReturnRemainder(ret.RefundAmount, orderTotal, taken)
	if err != nil {
		return store.OrderReturn{}, err
	}
	ret.RefundAmount = amount
	store.PrepareReturn(&ret, r.autoApproveAfter)
	r.returns[ret.ID] = ret
	return ret, nil
}

func (r *InMemoryReturnRepository) GetReturn(_ context.Context, id string) (store.OrderReturn, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ret, ok := r.returns[id]
	if !ok {
		return store.OrderReturn{}, store.ErrReturnNotFound
	}
	return ret, nil
}

func (r *InMemoryReturnRepository) GetReturnByOrderID(_ context.Context, orderID string) (store.OrderReturn, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ret := range r.returns {
		if ret.OrderID == orderID {
			return ret, nil
		}
	}
	return store.OrderReturn{}, store.ErrReturnNotFound
}

func (r *InMemoryReturnRepository) filter(keep func(store.OrderReturn) bool) []store.OrderReturn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []store.OrderReturn
	for _, ret := range r.returns {
		if keep(ret) {
			res = append(res, ret)
		}
	}
	return res
}

func (r *InMemoryReturnRepository) ListReturnsByBuyer(_ context.Context, buyerID string) ([]store.OrderReturn, error) {
	return r.filter(func(ret store.OrderReturn) bool { return ret.BuyerID == buyerID }), nil
}

func (r *InMemoryReturnRepository) ListReturnsBySeller(_ context.Context, sellerID string) ([]store.OrderReturn, error) {
	return r.filter(func(ret store.OrderReturn) bool { return ret.SellerID == sellerID }), nil
}

func (r *InMemoryReturnRepository) ListReturnsByOrder(_ context.Context, orderID string) ([]store.OrderReturn, error) {
	res := r.filter(func(ret store.OrderReturn) bool { return ret.OrderID == orderID })
	sort.Slice(res, func(i, j int) bool {
		if !res[i].CreatedAt.Equal(res[j].CreatedAt) {
			return res[i].CreatedAt.After(res[j].CreatedAt)
		}
		return res[i].ID > res[j].ID
	})
	return res, nil
}

func (r *InMemoryReturnRepository) TransitionReturn(_ context.Context, id string, from, to store.ReturnStatus) (store.OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ret, ok := r.returns[id]
	if !ok {
		return store.OrderReturn{}, store.ErrReturnNotFound
	}
	if ret.Status != from {
		return store.OrderReturn{}, store.ErrReturnStatusConflict
	}
	ret.Status = to
	ret.UpdatedAt = time.Now()
	r.returns[id] = ret
	return ret, nil
}

// ListAutoApprovable: PENDING returns with auto_approve_at <= now, oldest first.
func (r *InMemoryReturnRepository) ListAutoApprovable(_ context.Context, now time.Time, limit int) ([]store.OrderReturn, error) {
	res := r.filter(func(ret store.OrderReturn) bool {
		return ret.Status == store.ReturnStatusPending && ret.AutoApproveAt != nil && !ret.AutoApproveAt.After(now)
	})
	sort.Slice(res, func(i, j int) bool {
		if !res[i].AutoApproveAt.Equal(*res[j].AutoApproveAt) {
			return res[i].AutoApproveAt.Before(*res[j].AutoApproveAt)
		}
		return res[i].ID < res[j].ID
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *InMemoryReturnRepository) ReapproveReturn(_ context.Context, id string, orderTotal int64) (store.OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ret, ok := r.returns[id]
	if !ok {
		return store.OrderReturn{}, store.ErrReturnNotFound
	}
	if ret.Status != store.ReturnStatusRejected {
		return store.OrderReturn{}, store.ErrReturnStatusConflict
	}
	var taken int64
	for _, existing := range r.returns {
		if existing.OrderID == ret.OrderID && existing.Status != store.ReturnStatusRejected {
			taken += existing.RefundAmount
		}
	}
	if _, err := store.ReturnRemainder(ret.RefundAmount, orderTotal, taken); err != nil {
		return store.OrderReturn{}, err
	}
	ret.Status = store.ReturnStatusApproved
	ret.UpdatedAt = time.Now()
	r.returns[id] = ret
	return ret, nil
}
