package order_test

import (
	"context"

	"github.com/buidangphuc/team-order/internal/store"
)

func (m *mockOrderServiceRepo) HasCompletedOrder(_ context.Context, buyerID string) (bool, error) {
	for _, o := range m.orders {
		if o.BuyerID == buyerID && o.Status == store.OrderStatusCompleted {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeOrderRepo) HasCompletedOrder(context.Context, string) (bool, error) {
	return false, nil
}
