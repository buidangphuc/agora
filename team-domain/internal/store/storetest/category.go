package storetest

import (
	"context"
	"github.com/buidangphuc/team-domain/internal/store"
	"sort"
	"sync"
)

type InMemoryCategoryRepository struct {
	mu   sync.RWMutex
	byID map[string]store.Category
}

func NewInMemoryCategoryRepository(seed ...store.Category) *InMemoryCategoryRepository {
	if len(seed) == 0 {
		seed = []store.Category{
			{ID: "cat-electronics", Name: "Điện tử & Công nghệ", Slug: "dien-tu", DisplayOrder: 1, IconURL: "📱"},
			{ID: "cat-fashion", Name: "Thời trang & Phụ kiện", Slug: "thoi-trang", DisplayOrder: 2, IconURL: "👕"},
			{ID: "cat-home", Name: "Nhà cửa & Đời sống", Slug: "nha-cua", DisplayOrder: 3, IconURL: "🏠"},
		}
	}
	byID := make(map[string]store.Category, len(seed))
	for _, c := range seed {
		byID[c.ID] = c
	}
	return &InMemoryCategoryRepository{byID: byID}
}

func (r *InMemoryCategoryRepository) List(_ context.Context, parentID string) ([]store.Category, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []store.Category
	for _, c := range r.byID {
		if c.ParentID == parentID {
			items = append(items, c)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DisplayOrder == items[j].DisplayOrder {
			return items[i].Name < items[j].Name
		}
		return items[i].DisplayOrder < items[j].DisplayOrder
	})
	return items, nil
}

func (r *InMemoryCategoryRepository) Get(_ context.Context, id string) (store.Category, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byID[id]
	if !ok {
		return store.Category{}, store.ErrCategoryNotFound
	}
	return c, nil
}
