package storetest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-engagement/internal/store"
)

type FakeCollectionRepository struct {
	mu    sync.Mutex
	cols  map[string]store.Collection    // collectionID -> collection
	items map[string]map[string]struct{} // collectionID -> set(listingID)
}

func NewFakeCollectionRepository() *FakeCollectionRepository {
	return &FakeCollectionRepository{
		cols:  map[string]store.Collection{},
		items: map[string]map[string]struct{}{},
	}
}

func (r *FakeCollectionRepository) CreateCollection(_ context.Context, userID, name string) (store.Collection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := store.Collection{ID: uuid.NewString(), UserID: userID, Name: name, CreatedAt: time.Now()}
	r.cols[c.ID] = c
	r.items[c.ID] = map[string]struct{}{}
	return c, nil
}

func (r *FakeCollectionRepository) ListCollections(_ context.Context, userID string) ([]store.Collection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []store.Collection
	for id, c := range r.cols {
		if c.UserID != userID {
			continue
		}
		c.ItemCount = int64(len(r.items[id]))
		out = append(out, c)
	}
	// newest first
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *FakeCollectionRepository) owns(userID, collectionID string) error {
	c, ok := r.cols[collectionID]
	if !ok || c.UserID != userID {
		return store.ErrCollectionNotFound
	}
	return nil
}

func (r *FakeCollectionRepository) AddToCollection(_ context.Context, userID, collectionID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.owns(userID, collectionID); err != nil {
		return false, err
	}
	set := r.items[collectionID]
	if _, ok := set[listingID]; ok {
		return false, nil
	}
	set[listingID] = struct{}{}
	return true, nil
}

func (r *FakeCollectionRepository) RemoveFromCollection(_ context.Context, userID, collectionID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.owns(userID, collectionID); err != nil {
		return false, err
	}
	set := r.items[collectionID]
	if _, ok := set[listingID]; !ok {
		return false, nil
	}
	delete(set, listingID)
	return true, nil
}

func (r *FakeCollectionRepository) ListCollectionItems(_ context.Context, userID, collectionID, cursor string, pageSize int32) ([]string, string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.owns(userID, collectionID); err != nil {
		return nil, "", 0, err
	}
	all := make([]string, 0, len(r.items[collectionID]))
	for id := range r.items[collectionID] {
		all = append(all, id)
	}
	sort.Strings(all)

	limit := clampPageSize(pageSize)
	ids := make([]string, 0, limit)
	var next string
	for _, id := range all {
		if cursor != "" && id <= cursor {
			continue
		}
		if len(ids) == limit {
			next = ids[len(ids)-1]
			break
		}
		ids = append(ids, id)
	}
	return ids, next, int64(len(all)), nil
}

var _ store.CollectionRepository = (*FakeCollectionRepository)(nil)
