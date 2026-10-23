package storetest

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-engagement/internal/store"
)

type FakeQARepository struct {
	mu        sync.RWMutex
	questions []store.ProductQuestion
	answers   []store.ProductAnswer
}

func NewFakeQARepository() *FakeQARepository {
	return &FakeQARepository{
		questions: make([]store.ProductQuestion, 0),
		answers:   make([]store.ProductAnswer, 0),
	}
}

func (r *FakeQARepository) CreateQuestion(_ context.Context, q store.ProductQuestion) (store.ProductQuestion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if q.ID == "" {
		q.ID = uuid.NewString()
	}
	if q.CreatedAt.IsZero() {
		q.CreatedAt = time.Now().UTC()
	}
	if q.Answers == nil {
		q.Answers = make([]store.ProductAnswer, 0)
	}
	r.questions = append(r.questions, q)
	return q, nil
}

func (r *FakeQARepository) GetQuestionByID(_ context.Context, id string) (store.ProductQuestion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, q := range r.questions {
		if q.ID == id {
			res := q
			res.Answers = make([]store.ProductAnswer, 0)
			for _, a := range r.answers {
				if a.QuestionID == id {
					res.Answers = append(res.Answers, a)
				}
			}
			return res, nil
		}
	}
	return store.ProductQuestion{}, store.ErrQuestionNotFound
}

func (r *FakeQARepository) CreateAnswer(_ context.Context, a store.ProductAnswer) (store.ProductAnswer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if question exists
	found := false
	for _, q := range r.questions {
		if q.ID == a.QuestionID {
			found = true
			if a.ListingID == "" {
				a.ListingID = q.ListingID
			}
			break
		}
	}
	if !found {
		return store.ProductAnswer{}, store.ErrQuestionNotFound
	}

	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	r.answers = append(r.answers, a)
	return a, nil
}

func (r *FakeQARepository) ListQuestionsByListing(_ context.Context, listingID string, offset, limit int) ([]store.ProductQuestion, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []store.ProductQuestion
	for i := len(r.questions) - 1; i >= 0; i-- {
		q := r.questions[i]
		if q.ListingID == listingID {
			qCopy := q
			qCopy.Answers = make([]store.ProductAnswer, 0)
			for _, a := range r.answers {
				if a.QuestionID == q.ID {
					qCopy.Answers = append(qCopy.Answers, a)
				}
			}
			filtered = append(filtered, qCopy)
		}
	}

	total := int64(len(filtered))
	if offset >= len(filtered) {
		return make([]store.ProductQuestion, 0), total, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

var _ store.QARepository = (*FakeQARepository)(nil)
