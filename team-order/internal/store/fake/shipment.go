package fake

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-order/internal/store"
)

// InMemoryShipmentRepository
type InMemoryShipmentRepository struct {
	mu          sync.RWMutex
	shipments   map[string]store.Shipment
	checkpoints map[string][]store.ShipmentCheckpoint
}

func NewInMemoryShipmentRepository(_ ...store.ShipmentRepoOption) *InMemoryShipmentRepository {
	return &InMemoryShipmentRepository{
		shipments:   make(map[string]store.Shipment),
		checkpoints: make(map[string][]store.ShipmentCheckpoint),
	}
}

func (r *InMemoryShipmentRepository) CreateShipment(_ context.Context, s store.Shipment) (store.Shipment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == 0 {
		s.Status = store.ShipmentStatusPending
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()

	for i := range s.Checkpoints {
		cp := &s.Checkpoints[i]
		if cp.ID == "" {
			cp.ID = uuid.NewString()
		}
		cp.ShipmentID = s.ID
		if cp.Timestamp.IsZero() {
			cp.Timestamp = time.Now()
		}
		cp.CreatedAt = time.Now()
	}

	r.shipments[s.ID] = s
	r.checkpoints[s.ID] = append([]store.ShipmentCheckpoint{}, s.Checkpoints...)
	return s, nil
}

func (r *InMemoryShipmentRepository) GetShipment(_ context.Context, id string) (store.Shipment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.shipments[id]
	if !ok {
		return store.Shipment{}, store.ErrShipmentNotFound
	}
	s.Checkpoints = append([]store.ShipmentCheckpoint{}, r.checkpoints[id]...)
	return s, nil
}

func (r *InMemoryShipmentRepository) GetShipmentByTrackingCode(_ context.Context, trackingCode string) (store.Shipment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.shipments {
		if s.TrackingCode == trackingCode {
			s.Checkpoints = append([]store.ShipmentCheckpoint{}, r.checkpoints[s.ID]...)
			return s, nil
		}
	}
	return store.Shipment{}, store.ErrShipmentNotFound
}

func (r *InMemoryShipmentRepository) GetShipmentByOrderID(_ context.Context, orderID string) (store.Shipment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.shipments {
		if s.OrderID == orderID {
			s.Checkpoints = append([]store.ShipmentCheckpoint{}, r.checkpoints[s.ID]...)
			return s, nil
		}
	}
	return store.Shipment{}, store.ErrShipmentNotFound
}

func (r *InMemoryShipmentRepository) AddShipmentCheckpoint(_ context.Context, cp store.ShipmentCheckpoint) (store.ShipmentCheckpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.shipments[cp.ShipmentID]; !ok {
		return store.ShipmentCheckpoint{}, store.ErrShipmentNotFound
	}

	if cp.ID == "" {
		cp.ID = uuid.NewString()
	}
	if cp.Timestamp.IsZero() {
		cp.Timestamp = time.Now()
	}
	cp.CreatedAt = time.Now()

	r.checkpoints[cp.ShipmentID] = append(r.checkpoints[cp.ShipmentID], cp)
	return cp, nil
}

func (r *InMemoryShipmentRepository) UpdateShipmentStatus(_ context.Context, id string, status store.ShipmentStatus) (store.Shipment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.shipments[id]
	if !ok {
		return store.Shipment{}, store.ErrShipmentNotFound
	}
	s.Status = status
	s.UpdatedAt = time.Now()
	r.shipments[id] = s
	s.Checkpoints = append([]store.ShipmentCheckpoint{}, r.checkpoints[id]...)
	return s, nil
}
