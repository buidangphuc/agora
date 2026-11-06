package storefake

import (
	"context"
	"sync"
	"time"

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

func (r *InMemoryReservationRepository) countLocked(voucherID, buyerID string, now time.Time) int {
	n := 0
	for _, res := range r.reservations {
		if res.VoucherID != voucherID || res.BuyerID != buyerID {
			continue
		}
		if res.Status == store.ReservationCommitted || (res.Status == store.ReservationReserved && res.ExpiresAt.After(now)) {
			n++
		}
	}
	return n
}

func (r *InMemoryReservationRepository) CountActiveByBuyer(_ context.Context, voucherID, buyerID string, now time.Time) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.countLocked(voucherID, buyerID, now), nil
}

func (r *InMemoryReservationRepository) CreateWithinLimit(_ context.Context, res store.Reservation, perUserLimit int, now time.Time) (store.Reservation, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.reservations[res.ReservationID]; ok {
		return existing, false, nil
	}
	if r.countLocked(res.VoucherID, res.BuyerID, now) >= perUserLimit {
		return store.Reservation{}, false, store.ErrPerUserLimit
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
