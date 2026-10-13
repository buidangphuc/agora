package order_test

import (
	"context"
	"sync"
	"testing"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

// recordingSagaRepo records the ids of the sagas it creates so a test can inspect
// one checkout attempt's reservations.
type recordingSagaRepo struct {
	store.SagaRepository
	mu  sync.Mutex
	ids []string
}

func (r *recordingSagaRepo) CreateSaga(ctx context.Context, s store.Saga) (store.Saga, bool, error) {
	got, created, err := r.SagaRepository.CreateSaga(ctx, s)
	if err == nil && created {
		r.mu.Lock()
		r.ids = append(r.ids, got.ID)
		r.mu.Unlock()
	}
	return got, created, err
}

func (r *recordingSagaRepo) sagaIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

func (r *recordingSagaRepo) lastSagaID() string {
	ids := r.sagaIDs()
	if len(ids) == 0 {
		return ""
	}
	return ids[len(ids)-1]
}

func reserveReq(res store.Reservation) *listingv1.ReserveStockRequest {
	return &listingv1.ReserveStockRequest{ListingId: res.ListingID, VariantId: res.VariantID, Quantity: res.Quantity, ReservationId: res.ID}
}

// cancelWithoutRelease moves an order to Cancelled without releasing anything:
// a cancel that crashed right after its claim.
func cancelWithoutRelease(t *testing.T, orders store.OrderRepository, id string) {
	t.Helper()
	if _, err := orders.UpdateOrderStatusFrom(context.Background(), id, store.OrderStatusCancelled, []store.OrderStatus{store.OrderStatusPending, store.OrderStatusPaid}, ""); err != nil {
		t.Fatal(err)
	}
}

func releaseReq(id string) *listingv1.ReleaseStockRequest {
	return &listingv1.ReleaseStockRequest{ReservationId: id}
}
