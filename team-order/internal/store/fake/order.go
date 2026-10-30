package fake

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-order/internal/store"
)

// InMemoryOrderRepository for unit tests
type InMemoryOrderRepository struct {
	mu           sync.RWMutex
	orders       map[string]store.Order
	returnSource store.ReturnRepository

	autoCompleteAfter, returnWindow, shipByAfter time.Duration
}

func NewInMemoryOrderRepository(opts ...store.OrderRepoOption) *InMemoryOrderRepository {
	cfg := store.ResolveOrderOptions(opts...)
	return &InMemoryOrderRepository{
		orders:            make(map[string]store.Order),
		autoCompleteAfter: cfg.AutoCompleteAfter, returnWindow: cfg.ReturnWindow, shipByAfter: cfg.ShipByAfter,
	}
}

func (r *InMemoryOrderRepository) CreateOrder(_ context.Context, order store.Order) (store.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if order.ID == "" {
		order.ID = uuid.NewString()
	}
	if order.Currency == "" {
		order.Currency = "VND"
	}
	if order.Status == 0 {
		order.Status = store.OrderStatusPending
	}
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()
	for i := range order.Items {
		if order.Items[i].ID == "" {
			order.Items[i].ID = uuid.NewString()
		}
		order.Items[i].OrderID = order.ID
	}
	r.orders[order.ID] = order
	return order, nil
}

func (r *InMemoryOrderRepository) GetOrder(_ context.Context, id string) (store.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.orders[id]
	if !ok {
		return store.Order{}, store.ErrOrderNotFound
	}
	return o, nil
}

func (r *InMemoryOrderRepository) ListBuyerOrders(_ context.Context, buyerID string, statusFilter int32) ([]store.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []store.Order
	for _, o := range r.orders {
		if o.BuyerID == buyerID {
			if statusFilter == 0 || int32(o.Status) == statusFilter {
				res = append(res, o)
			}
		}
	}
	return res, nil
}

func (r *InMemoryOrderRepository) ListSellerOrders(_ context.Context, sellerID string, statusFilter int32) ([]store.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []store.Order
	for _, o := range r.orders {
		if o.SellerID == sellerID {
			if statusFilter == 0 || int32(o.Status) == statusFilter {
				res = append(res, o)
			}
		}
	}
	return res, nil
}

func (r *InMemoryOrderRepository) ListOrdersByCheckout(_ context.Context, checkoutID string) ([]store.Order, error) {
	if checkoutID == "" {
		return nil, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []store.Order
	for _, o := range r.orders {
		if o.CheckoutID == checkoutID {
			res = append(res, o)
		}
	}
	return res, nil
}

func (r *InMemoryOrderRepository) UpdateOrderStatusFrom(_ context.Context, id string, to store.OrderStatus, allowedFrom []store.OrderStatus, trackingNumber string, opts ...store.StatusOption) (store.Order, error) {
	cfg := store.ResolveStatusOptions(opts...)
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return store.Order{}, store.ErrOrderNotFound
	}
	allowed := false
	for _, st := range allowedFrom {
		if o.Status == st {
			allowed = true
			break
		}
	}
	if !allowed {
		return store.Order{}, store.ErrStatusConflict
	}
	o.Status = to
	if trackingNumber != "" {
		o.TrackingNumber = trackingNumber
	}
	o.UpdatedAt = time.Now()
	if to == store.OrderStatusCancelled {
		o.CancelReason = cfg.CancelReason
	}
	if to == store.OrderStatusPaid {
		paidAt := o.UpdatedAt
		o.PaidAt = &paidAt
		o.ShipBy = store.StampAfter(paidAt, r.shipByAfter)
	}
	if to == store.OrderStatusShipped {
		shippedAt := o.UpdatedAt
		o.ShippedAt = &shippedAt
		o.AutoCompleteAt = store.StampAfter(shippedAt, r.autoCompleteAfter)
	}
	if to == store.OrderStatusCompleted {
		completedAt := o.UpdatedAt
		o.CompletedAt = &completedAt
		o.AutoCompleteAt = nil
		o.ReturnWindowEndsAt = store.StampAfter(completedAt, r.returnWindow)
		o.CompletedBy = cfg.CompletedBy
	}
	r.orders[id] = o
	return o, nil
}

// SetReturnSource lets the in-memory order store evaluate the blocking-return
// rule of ListAutoCompletable (the Postgres store does it in SQL).
func (r *InMemoryOrderRepository) SetReturnSource(src store.ReturnRepository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.returnSource = src
}

// ListAutoCompletable: SHIPPED orders with auto_complete_at <= now and no blocking
// return (PENDING, or APPROVED when the order has paid_at), oldest deadline first.
func (r *InMemoryOrderRepository) ListAutoCompletable(ctx context.Context, now time.Time, limit int) ([]store.Order, error) {
	r.mu.RLock()
	var due []store.Order
	for _, o := range r.orders {
		if o.Status == store.OrderStatusShipped && o.AutoCompleteAt != nil && !o.AutoCompleteAt.After(now) {
			due = append(due, o)
		}
	}
	src := r.returnSource
	r.mu.RUnlock()
	sort.Slice(due, func(i, j int) bool {
		if !due[i].AutoCompleteAt.Equal(*due[j].AutoCompleteAt) {
			return due[i].AutoCompleteAt.Before(*due[j].AutoCompleteAt)
		}
		return due[i].ID < due[j].ID
	})
	var res []store.Order
	for _, o := range due {
		if limit > 0 && len(res) >= limit {
			break
		}
		if src != nil {
			rets, err := src.ListReturnsByOrder(ctx, o.ID)
			if err != nil {
				return nil, err
			}
			blocked := false
			for _, ret := range rets {
				if ret.Status == store.ReturnStatusPending || (ret.Status == store.ReturnStatusApproved && o.PaidAt != nil) {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}
		}
		res = append(res, o)
	}
	return res, nil
}

func (r *InMemoryOrderRepository) FindReminderDue(_ context.Context, now time.Time, before time.Duration, limit int) ([]store.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []store.Order
	for _, o := range r.orders {
		if o.Status != store.OrderStatusPending || o.PaymentMethod == store.CodPaymentMethod || o.PaymentDueAt == nil ||
			o.PaymentReminderSentAt != nil {
			continue
		}
		due := *o.PaymentDueAt
		if due.Add(-before).After(now) || !due.After(now) {
			continue
		}
		res = append(res, o)
	}
	sort.Slice(res, func(i, j int) bool {
		if !res[i].PaymentDueAt.Equal(*res[j].PaymentDueAt) {
			return res[i].PaymentDueAt.Before(*res[j].PaymentDueAt)
		}
		return res[i].ID < res[j].ID
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *InMemoryOrderRepository) FindExpired(_ context.Context, now time.Time, limit int) ([]store.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []store.Order
	for _, o := range r.orders {
		if o.Status != store.OrderStatusPending || o.PaymentMethod == store.CodPaymentMethod || o.PaymentDueAt == nil ||
			!o.PaymentDueAt.Before(now) {
			continue
		}
		res = append(res, o)
	}
	sort.Slice(res, func(i, j int) bool {
		if !res[i].PaymentDueAt.Equal(*res[j].PaymentDueAt) {
			return res[i].PaymentDueAt.Before(*res[j].PaymentDueAt)
		}
		return res[i].ID < res[j].ID
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *InMemoryOrderRepository) MarkReminderSent(_ context.Context, id string, now time.Time) (store.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return store.Order{}, store.ErrOrderNotFound
	}
	if o.Status != store.OrderStatusPending || o.PaymentReminderSentAt != nil {
		return store.Order{}, store.ErrStatusConflict
	}
	sent := now
	o.PaymentReminderSentAt = &sent
	r.orders[id] = o
	return o, nil
}
