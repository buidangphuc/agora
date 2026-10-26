package storefake

import (
	"context"
	"sync"

	"github.com/buidangphuc/team-promotion/internal/store"
)

// InMemoryReservationRepository is the DB-less backend used when DATABASE_ENABLED
// is false and in unit tests.
type InMemoryReservationRepository struct {
	mu           sync.RWMutex
	reservations map[string]store.Reservation // keyed by reservation_id
}

func NewInMemoryReservationRepository() *InMemoryReservationRepository {
	return &InMemoryReservationRepository{reservations: make(map[string]store.Reservation)}
}

func (r *InMemoryReservationRepository) Create(_ context.Context, res store.Reservation) (store.Reservation, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.reservations[res.ReservationID]; ok {
		return existing, false, nil
	}
	if res.ID == "" {
		res.ID = store.NewID()
	}
	if res.Status == "" {
		res.Status = store.ReservationReserved
	}
	r.reservations[res.ReservationID] = res
	return res, true, nil
}

func (r *InMemoryReservationRepository) GetByReservationID(_ context.Context, reservationID string) (store.Reservation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res, ok := r.reservations[reservationID]
	if !ok {
		return store.Reservation{}, store.ErrReservationNotFound
	}
	return res, nil
}

func (r *InMemoryReservationRepository) UpdateStatus(_ context.Context, reservationID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, ok := r.reservations[reservationID]
	if !ok {
		return store.ErrReservationNotFound
	}
	res.Status = status
	r.reservations[reservationID] = res
	return nil
}

func (r *InMemoryReservationRepository) Ping(_ context.Context) error { return nil }
