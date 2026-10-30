package rpc_test

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/buidangphuc/team-search/internal/store"
)

type fakeSaved struct {
	mu   sync.Mutex
	rows []store.SavedSearch
	n    int
}

func newFakeSaved() *fakeSaved { return &fakeSaved{} }

func (f *fakeSaved) Create(_ context.Context, s store.SavedSearch) (store.SavedSearch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	if s.ID == "" {
		s.ID = "ss-" + strconv.Itoa(f.n)
	}
	if s.FiltersJSON == "" {
		s.FiltersJSON = "{}"
	}
	s.CreatedAt = time.Unix(1_700_000_000, 0).Add(time.Duration(f.n) * time.Second)
	f.rows = append(f.rows, s)
	return s, nil
}

func (f *fakeSaved) List(_ context.Context, userID string, limit, offset int) ([]store.SavedSearch, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var own []store.SavedSearch
	for i := len(f.rows) - 1; i >= 0; i-- {
		if f.rows[i].UserID == userID {
			own = append(own, f.rows[i])
		}
	}
	total := int64(len(own))
	if offset >= len(own) {
		return []store.SavedSearch{}, total, nil
	}
	end := min(offset+limit, len(own))
	return own[offset:end], total, nil
}

func (f *fakeSaved) Get(_ context.Context, id, userID string) (store.SavedSearch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID == id && r.UserID == userID {
			return r, nil
		}
	}
	return store.SavedSearch{}, store.ErrSavedSearchNotFound
}

func (f *fakeSaved) Delete(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.rows {
		if r.ID == id && r.UserID == userID {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return store.ErrSavedSearchNotFound
}
