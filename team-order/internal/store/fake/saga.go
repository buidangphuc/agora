package fake

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-order/internal/store"
)

// ReservationStore is the backing state shared by InMemorySagaRepository
// instances. Sharing one store across two repositories models a process restart
// against the same durable database (used by the M8 restart test).
type ReservationStore struct {
	mu           sync.RWMutex
	sagas        map[string]store.Saga
	reservations map[string]store.Reservation
}

// NewReservationStore builds an empty shared backing store.
func NewReservationStore() *ReservationStore {
	return &ReservationStore{
		sagas:        make(map[string]store.Saga),
		reservations: make(map[string]store.Reservation),
	}
}

type InMemorySagaRepository struct {
	store  *ReservationStore
	orders store.OrderReader // set by BindOrders; answers FindHeldByCancelledOrders
}

// BindOrders gives the in-memory saga repository the order store it reads order
// statuses from (Postgres joins). NewOrderService binds it automatically.
func (r *InMemorySagaRepository) BindOrders(o store.OrderReader) { r.orders = o }

// NewInMemorySagaRepository builds an in-memory saga repo over its own store.
func NewInMemorySagaRepository() *InMemorySagaRepository {
	return &InMemorySagaRepository{store: NewReservationStore()}
}

// NewInMemorySagaRepositoryWithStore builds a repo over an externally-owned store
// so a second instance can read state a prior instance wrote (restart semantics).
func NewInMemorySagaRepositoryWithStore(store *ReservationStore) *InMemorySagaRepository {
	if store == nil {
		store = NewReservationStore()
	}
	return &InMemorySagaRepository{store: store}
}

func (r *InMemorySagaRepository) CreateSaga(_ context.Context, s store.Saga) (store.Saga, bool, error) {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if s.IdempotencyKey != "" {
		for _, existing := range r.store.sagas {
			if existing.BuyerID == s.BuyerID && existing.IdempotencyKey == s.IdempotencyKey {
				return existing, false, nil
			}
		}
	}
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == 0 {
		s.Status = store.SagaStatusPending
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = s.CreatedAt
	r.store.sagas[s.ID] = s
	return s, true, nil
}

func (r *InMemorySagaRepository) GetSaga(_ context.Context, id string) (store.Saga, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	s, ok := r.store.sagas[id]
	if !ok {
		return store.Saga{}, store.ErrSagaNotFound
	}
	return s, nil
}

func (r *InMemorySagaRepository) UpdateSagaStatus(_ context.Context, id string, status store.SagaStatus) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	s, ok := r.store.sagas[id]
	if !ok {
		return fmt.Errorf("saga %q not found", id)
	}
	s.Status = status
	if status == store.SagaStatusCompensated || status == store.SagaStatusFailed {
		s.IdempotencyKey = "" // free the key (mirrors the Postgres statement)
	}
	s.UpdatedAt = time.Now()
	r.store.sagas[id] = s
	return nil
}

func (r *InMemorySagaRepository) CreateReservation(_ context.Context, res store.Reservation) (store.Reservation, error) {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if res.ID == "" {
		res.ID = uuid.NewString()
	}
	// Idempotent on the stable reservation_id (AD5): keep the existing row.
	if existing, ok := r.store.reservations[res.ID]; ok {
		return existing, nil
	}
	if res.Status == 0 {
		res.Status = store.ReservationStatusPending
	}
	if res.ExpiresAt.IsZero() {
		res.ExpiresAt = time.Now().Add(15 * time.Minute)
	}
	res.CreatedAt = time.Now()
	res.UpdatedAt = time.Now()
	r.store.reservations[res.ID] = res
	return res, nil
}

func (r *InMemorySagaRepository) GetReservation(_ context.Context, id string) (store.Reservation, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	res, ok := r.store.reservations[id]
	if !ok {
		return store.Reservation{}, store.ErrReservationNotFound
	}
	return res, nil
}

func (r *InMemorySagaRepository) UpdateReservationStatus(_ context.Context, id string, status store.ReservationStatus) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	res, ok := r.store.reservations[id]
	if !ok {
		return store.ErrReservationNotFound
	}
	res.Status = status
	res.UpdatedAt = time.Now()
	r.store.reservations[id] = res
	return nil
}

func (r *InMemorySagaRepository) CommitReservation(_ context.Context, id, orderID string) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	res, ok := r.store.reservations[id]
	if !ok {
		return store.ErrReservationNotFound
	}
	res.Status = store.ReservationStatusCommitted
	res.OrderID = orderID
	res.UpdatedAt = time.Now()
	r.store.reservations[id] = res
	return nil
}

func (r *InMemorySagaRepository) ListReservationsBySaga(_ context.Context, sagaID string) ([]store.Reservation, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	var out []store.Reservation
	for _, res := range r.store.reservations {
		if res.SagaID == sagaID {
			out = append(out, res)
		}
	}
	return out, nil
}

func (r *InMemorySagaRepository) ListReservationsByOrder(_ context.Context, orderID string) ([]store.Reservation, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	var out []store.Reservation
	for _, res := range r.store.reservations {
		if res.OrderID == orderID {
			out = append(out, res)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *InMemorySagaRepository) ListCommittedReservations(_ context.Context, afterID string, limit int) ([]store.Reservation, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	var out []store.Reservation
	for _, res := range r.store.reservations {
		if res.Status == store.ReservationStatusCommitted && res.ID > afterID {
			out = append(out, res)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemorySagaRepository) FindReleasable(_ context.Context, now time.Time, limit int) ([]store.Reservation, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	var out []store.Reservation
	for _, res := range r.store.reservations {
		if res.Status != store.ReservationStatusReserved && res.Status != store.ReservationStatusReleaseFailed {
			continue
		}
		if res.ExpiresAt.After(now) {
			continue
		}
		out = append(out, res)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *InMemorySagaRepository) FindHeldByCancelledOrders(ctx context.Context, cancelledBefore time.Time, limit int) ([]store.Reservation, error) {
	if r.orders == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	r.store.mu.RLock()
	var candidates []store.Reservation
	for _, res := range r.store.reservations {
		if res.OrderID != "" && (res.Status == store.ReservationStatusCommitted || res.Status == store.ReservationStatusReleaseFailed) {
			candidates = append(candidates, res)
		}
	}
	r.store.mu.RUnlock() // never hold the store lock while reading orders

	var out []store.Reservation
	for _, res := range candidates {
		o, err := r.orders.GetOrder(ctx, res.OrderID)
		if err != nil || o.Status != store.OrderStatusCancelled || o.UpdatedAt.After(cancelledBefore) {
			continue
		}
		out = append(out, res)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *InMemorySagaRepository) FindStalePendingSagas(_ context.Context, createdBefore time.Time, limit int) ([]store.Saga, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	var out []store.Saga
	for _, sg := range r.store.sagas {
		if sg.Status == store.SagaStatusPending && !sg.CreatedAt.After(createdBefore) {
			out = append(out, sg)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
