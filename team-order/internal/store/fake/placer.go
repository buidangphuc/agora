package fake

import (
	"context"
	"fmt"
	"time"

	"github.com/buidangphuc/team-order/internal/store"
)

// InMemoryOrderPlacer is the OrderPlacer of the in-memory stores: the whole
// placement runs under both stores' locks, validating every reservation and the
// saga before anything changes.
type InMemoryOrderPlacer struct {
	orders *InMemoryOrderRepository
	sagas  *InMemorySagaRepository
}

// NewInMemoryOrderPlacer places orders into orders and binds reservations in sagas.
func NewInMemoryOrderPlacer(orders *InMemoryOrderRepository, sagas *InMemorySagaRepository) *InMemoryOrderPlacer {
	return &InMemoryOrderPlacer{orders: orders, sagas: sagas}
}

func (p *InMemoryOrderPlacer) PlaceOrders(_ context.Context, sagaID string, placed []store.PlacedOrder) ([]store.Order, error) {
	if len(placed) == 0 {
		return nil, nil
	}
	p.orders.mu.Lock()
	defer p.orders.mu.Unlock()
	st := p.sagas.store
	st.mu.Lock()
	defer st.mu.Unlock()

	sg, ok := st.sagas[sagaID]
	if !ok || sg.Status != store.SagaStatusPending {
		return nil, fmt.Errorf("%w: checkout attempt %s is no longer pending", store.ErrReservationLost, sagaID)
	}
	for _, po := range placed {
		for _, id := range po.ReservationIDs {
			res, ok := st.reservations[id]
			if !ok || res.Status != store.ReservationStatusReserved {
				return nil, fmt.Errorf("%w: reservation %s", store.ErrReservationLost, id)
			}
		}
		if _, dup := p.orders.orders[po.Order.ID]; dup && po.Order.ID != "" {
			return nil, fmt.Errorf("insert order: duplicate id %s", po.Order.ID)
		}
	}

	now := time.Now()
	out := make([]store.Order, 0, len(placed))
	for _, po := range placed {
		o := po.Order
		store.PrepareOrder(&o, now, po.PaymentDeadline, p.orders.shipByAfter)
		for _, id := range po.ReservationIDs {
			res := st.reservations[id]
			res.Status = store.ReservationStatusCommitted
			res.OrderID = o.ID
			res.UpdatedAt = now
			st.reservations[id] = res
		}
		out = append(out, o)
	}
	for _, o := range out {
		p.orders.orders[o.ID] = o
	}
	sg.Status = store.SagaStatusCompleted
	sg.UpdatedAt = now
	st.sagas[sagaID] = sg
	return out, nil
}
