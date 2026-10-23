package storetest

import (
	"context"
	"sync"
	"time"

	"github.com/buidangphuc/team-verification/internal/store"
)

type InMemoryKycRepo struct {
	mu    sync.Mutex
	byID  map[string]*store.Submission
	order []string // insertion order, for deterministic "latest by user"
}

func NewInMemoryKycRepo() *InMemoryKycRepo {
	return &InMemoryKycRepo{byID: make(map[string]*store.Submission)}
}

func (r *InMemoryKycRepo) Create(_ context.Context, s *store.Submission) (*store.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := clone(s)
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = time.Now()
	}
	r.byID[stored.ID] = stored
	r.order = append(r.order, stored.ID)
	return clone(stored), nil
}

func (r *InMemoryKycRepo) GetByID(_ context.Context, id string) (*store.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return clone(s), nil
}

func (r *InMemoryKycRepo) GetLatestByUser(_ context.Context, userID string) (*store.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *store.Submission
	// Walk insertion order; the last matching entry is the most recent.
	for _, id := range r.order {
		if s := r.byID[id]; s.UserID == userID {
			latest = s
		}
	}
	if latest == nil {
		return nil, store.ErrNotFound
	}
	return clone(latest), nil
}

func (r *InMemoryKycRepo) UpdateStatus(_ context.Context, id string, status store.Status, reviewedAt time.Time) (*store.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	if s.Status != store.StatusPending {
		return nil, store.ErrAlreadyReviewed
	}
	s.Status = status
	rt := reviewedAt
	s.ReviewedAt = &rt
	return clone(s), nil
}

func clone(s *store.Submission) *store.Submission {
	out := *s
	if s.ReviewedAt != nil {
		rt := *s.ReviewedAt
		out.ReviewedAt = &rt
	}
	return &out
}
