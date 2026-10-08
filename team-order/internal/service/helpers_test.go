package service_test

import (
	"context"
	"sync"

	"github.com/buidangphuc/team-order/internal/repository"
)

// recordingSagaRepo records the ids of the sagas it creates so a test can inspect
// one checkout attempt's reservations.
type recordingSagaRepo struct {
	repository.SagaRepository
	mu  sync.Mutex
	ids []string
}

func (r *recordingSagaRepo) CreateSaga(ctx context.Context, s repository.Saga) (repository.Saga, bool, error) {
	got, created, err := r.SagaRepository.CreateSaga(ctx, s)
	if err == nil && created {
		r.mu.Lock()
		r.ids = append(r.ids, got.ID)
		r.mu.Unlock()
	}
	return got, created, err
}

func (r *recordingSagaRepo) sagaIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

func (r *recordingSagaRepo) lastSagaID() string {
	ids := r.sagaIDs()
	if len(ids) == 0 {
		return ""
	}
	return ids[len(ids)-1]
}
