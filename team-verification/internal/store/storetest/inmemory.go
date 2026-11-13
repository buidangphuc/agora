package storetest

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/team-verification/internal/store"
)

type InMemoryKycRepo struct {
	mu    sync.Mutex
	byID  map[string]*store.Submission
	order []string // insertion order, for deterministic "latest by user"

	grants map[string]*store.Grant
	docs   map[string][]store.Document
}

func NewInMemoryKycRepo() *InMemoryKycRepo {
	return &InMemoryKycRepo{
		byID:   make(map[string]*store.Submission),
		grants: make(map[string]*store.Grant),
		docs:   make(map[string][]store.Document),
	}
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

func (r *InMemoryKycRepo) CreateGrant(_ context.Context, g *store.Grant, cap int, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	open := 0
	for _, x := range r.grants {
		if x.UserID == g.UserID && x.UsedAt == nil && x.ExpiresAt.After(now) {
			open++
		}
	}
	if open >= cap {
		return store.ErrGrantCapReached
	}
	c := *g
	r.grants[g.ObjectKey] = &c
	return nil
}

func (r *InMemoryKycRepo) GetGrant(_ context.Context, key string) (*store.Grant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.grants[key]
	if !ok {
		return nil, store.ErrNotFound
	}
	c := *g
	return &c, nil
}

func (r *InMemoryKycRepo) SubmitWithDocuments(_ context.Context, s *store.Submission, keys []string, now time.Time, verify func([]store.Grant) error) (*store.Submission, error) {
	if len(keys) == 0 {
		return nil, store.ErrGrantInvalid
	}
	sorted := slices.Clone(keys)
	slices.Sort(sorted)
	if len(slices.Compact(slices.Clone(sorted))) != len(sorted) {
		return nil, store.ErrGrantInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	grants := make([]store.Grant, 0, len(keys))
	expired := false
	for _, k := range keys {
		g, ok := r.grants[k]
		if !ok || g.UserID != s.UserID || g.UsedAt != nil {
			return nil, store.ErrGrantInvalid
		}
		if !g.ExpiresAt.After(now) {
			expired = true
		}
		grants = append(grants, *g)
	}
	if expired {
		return nil, store.ErrGrantExpired
	}
	if verify != nil {
		if err := verify(grants); err != nil {
			return nil, err
		}
	}
	sub := clone(s)
	sub.DocRef = keys[0]
	sub.CreatedAt = now
	for _, id := range r.order {
		if p := r.byID[id]; p.UserID == sub.UserID && p.SupersededAt == nil {
			t := now
			p.SupersededAt = &t
		}
	}
	r.byID[sub.ID] = sub
	r.order = append(r.order, sub.ID)
	for i, g := range grants {
		r.docs[sub.ID] = append(r.docs[sub.ID], store.Document{
			SubmissionID: sub.ID, Index: i, ObjectKey: g.ObjectKey, ContentType: g.ContentType, SizeBytes: g.SizeBytes,
		})
		t := now
		r.grants[g.ObjectKey].UsedAt = &t
	}
	return clone(sub), nil
}

func (r *InMemoryKycRepo) ListDocuments(_ context.Context, submissionID string) ([]store.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.docs[submissionID]), nil
}

func (r *InMemoryKycRepo) ReviewWithReason(_ context.Context, id string, status store.Status, reasonCode string, reviewedAt time.Time) (*store.Submission, error) {
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
	s.ReasonCode = reasonCode
	rt := reviewedAt
	s.ReviewedAt = &rt
	return clone(s), nil
}

func (r *InMemoryKycRepo) ListSubmissions(_ context.Context, f store.ListFilter) ([]store.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []*store.Submission
	for _, s := range r.byID {
		if f.Status != "" && s.Status != f.Status {
			continue
		}
		if f.HasCursor && !(s.CreatedAt.After(f.AfterAt) || (s.CreatedAt.Equal(f.AfterAt) && s.ID > f.AfterID)) {
			continue
		}
		all = append(all, s)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.Before(all[j].CreatedAt)
		}
		return all[i].ID < all[j].ID
	})
	if f.Limit > 0 && len(all) > f.Limit {
		all = all[:f.Limit]
	}
	out := make([]store.Submission, len(all))
	for i, s := range all {
		c := clone(s)
		c.DocumentCount = len(r.docs[s.ID])
		out[i] = *c
	}
	return out, nil
}

func (r *InMemoryKycRepo) LatestStatuses(_ context.Context, userIDs []string) (map[string]store.Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]store.Status{}
	for _, id := range r.order {
		if s := r.byID[id]; slices.Contains(userIDs, s.UserID) {
			out[s.UserID] = s.Status
		}
	}
	return out, nil
}
