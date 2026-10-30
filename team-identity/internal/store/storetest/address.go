package storetest

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-identity/internal/store"
)

// InMemoryAddressRepository is a deterministic fake store for tests.
type InMemoryAddressRepository struct {
	mu   sync.RWMutex
	byID map[string]store.Address
}

func NewInMemoryAddressRepository() *InMemoryAddressRepository {
	return &InMemoryAddressRepository{byID: make(map[string]store.Address)}
}

func (r *InMemoryAddressRepository) List(_ context.Context, userID string) ([]store.Address, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []store.Address
	for _, a := range r.byID {
		if a.UserID == userID {
			items = append(items, a)
		}
	}
	return items, nil
}

func (r *InMemoryAddressRepository) Get(_ context.Context, id, userID string) (store.Address, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byID[id]
	if !ok || a.UserID != userID {
		return store.Address{}, store.ErrAddressNotFound
	}
	return a, nil
}

func (r *InMemoryAddressRepository) Create(_ context.Context, a store.Address) (store.Address, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.IsDefault {
		for id, item := range r.byID {
			if item.UserID == a.UserID {
				item.IsDefault = false
				r.byID[id] = item
			}
		}
	}
	r.byID[a.ID] = a
	return a, nil
}

func (r *InMemoryAddressRepository) Update(_ context.Context, a store.Address) (store.Address, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.byID[a.ID]
	if !ok || existing.UserID != a.UserID {
		return store.Address{}, store.ErrAddressNotFound
	}
	if a.IsDefault {
		for id, item := range r.byID {
			if item.UserID == a.UserID {
				item.IsDefault = false
				r.byID[id] = item
			}
		}
	}
	r.byID[a.ID] = a
	return a, nil
}

func (r *InMemoryAddressRepository) Delete(_ context.Context, id, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok || a.UserID != userID {
		return store.ErrAddressNotFound
	}
	delete(r.byID, id)
	return nil
}

func (r *InMemoryAddressRepository) SetDefault(_ context.Context, id, userID string) (store.Address, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok || a.UserID != userID {
		return store.Address{}, store.ErrAddressNotFound
	}
	for k, item := range r.byID {
		if item.UserID == userID {
			item.IsDefault = false
			r.byID[k] = item
		}
	}
	a.IsDefault = true
	r.byID[id] = a
	return a, nil
}
