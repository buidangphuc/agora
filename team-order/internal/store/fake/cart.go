package fake

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-order/internal/store"
)

// InMemoryCartRepository is for unit tests.
type InMemoryCartRepository struct {
	mu    sync.RWMutex
	items map[string]store.CartItem
}

func NewInMemoryCartRepository() *InMemoryCartRepository {
	return &InMemoryCartRepository{items: make(map[string]store.CartItem)}
}

func (r *InMemoryCartRepository) GetCart(_ context.Context, userID string) ([]store.CartItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []store.CartItem
	for _, item := range r.items {
		if item.UserID == userID {
			res = append(res, item)
		}
	}
	return res, nil
}

func (r *InMemoryCartRepository) AddItem(ctx context.Context, item store.CartItem) ([]store.CartItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	// check existing
	found := false
	for id, existing := range r.items {
		if existing.UserID == item.UserID && existing.ListingID == item.ListingID && existing.VariantID == item.VariantID {
			existing.Quantity += item.Quantity
			existing.UnitPrice = item.UnitPrice
			existing.Title = item.Title
			existing.VariantName = item.VariantName
			existing.ImageURL = item.ImageURL
			existing.SellerID = item.SellerID
			r.items[id] = existing
			found = true
			break
		}
	}
	if !found {
		r.items[item.ID] = item
	}
	r.mu.Unlock()
	res, err := r.GetCart(ctx, item.UserID)
	r.mu.Lock()
	return res, err
}

func (r *InMemoryCartRepository) UpdateItem(ctx context.Context, userID, itemID string, quantity int32) ([]store.CartItem, error) {
	r.mu.Lock()
	item, ok := r.items[itemID]
	if !ok || item.UserID != userID {
		r.mu.Unlock()
		return nil, store.ErrCartItemNotFound
	}
	if quantity <= 0 {
		delete(r.items, itemID)
	} else {
		item.Quantity = quantity
		r.items[itemID] = item
	}
	r.mu.Unlock()
	return r.GetCart(ctx, userID)
}

func (r *InMemoryCartRepository) RemoveItem(ctx context.Context, userID, itemID string) ([]store.CartItem, error) {
	r.mu.Lock()
	item, ok := r.items[itemID]
	if !ok || item.UserID != userID {
		r.mu.Unlock()
		return nil, store.ErrCartItemNotFound
	}
	delete(r.items, itemID)
	r.mu.Unlock()
	return r.GetCart(ctx, userID)
}

func (r *InMemoryCartRepository) ClearCart(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, item := range r.items {
		if item.UserID == userID {
			delete(r.items, id)
		}
	}
	return nil
}

func (r *InMemoryCartRepository) RemoveItems(_ context.Context, userID string, itemIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	idSet := make(map[string]struct{}, len(itemIDs))
	for _, id := range itemIDs {
		idSet[id] = struct{}{}
	}
	for id, item := range r.items {
		if item.UserID == userID {
			if _, match := idSet[id]; match {
				delete(r.items, id)
			}
		}
	}
	return nil
}

func (r *InMemoryCartRepository) UpdateItemPrice(_ context.Context, userID, itemID string, unitPrice int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[itemID]
	if !ok || item.UserID != userID {
		return store.ErrCartItemNotFound
	}
	item.UnitPrice = unitPrice
	r.items[itemID] = item
	return nil
}
