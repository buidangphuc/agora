package storetest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	"github.com/buidangphuc/team-domain/internal/store"
)

type InMemoryBundleRepository struct {
	mu   sync.RWMutex
	byID map[string]store.Bundle
}

func NewInMemoryBundleRepository(seed ...store.Bundle) *InMemoryBundleRepository {
	byID := make(map[string]store.Bundle, len(seed))
	for _, b := range seed {
		byID[b.ID] = b
	}
	return &InMemoryBundleRepository{byID: byID}
}

func (r *InMemoryBundleRepository) Create(_ context.Context, b store.Bundle) (store.Bundle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	r.byID[b.ID] = b
	return b, nil
}

func (r *InMemoryBundleRepository) Get(_ context.Context, id string) (store.Bundle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.byID[id]
	if !ok {
		return store.Bundle{}, store.ErrBundleNotFound
	}
	return b, nil
}

func (r *InMemoryBundleRepository) ListBySeller(_ context.Context, sellerID, cursor string, pageSize int32) (store.BundlePage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.byID))
	var total int64
	for id, b := range r.byID {
		if b.SellerID == sellerID {
			ids = append(ids, id)
			total++
		}
	}
	sort.Strings(ids)

	limit := pagination.Clamp(int(pageSize), store.DefaultPageSize, store.MaxPageSize)
	items := make([]store.Bundle, 0, limit)
	var next string
	for _, id := range ids {
		if cursor != "" && id <= cursor {
			continue
		}
		if len(items) == limit {
			next = items[len(items)-1].ID
			break
		}
		items = append(items, r.byID[id])
	}
	return store.BundlePage{Items: items, NextCursor: next, Total: total}, nil
}

var _ store.BundleRepository = (*InMemoryBundleRepository)(nil)
