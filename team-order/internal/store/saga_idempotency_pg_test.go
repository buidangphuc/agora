package store_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-order/internal/store"
)

func runSagaKeyContract(t *testing.T, repo store.SagaRepository) {
	ctx := context.Background()
	buyer, other, key := uid("buyer"), uid("buyer"), uid("key")

	first, created, err := repo.CreateSaga(ctx, store.Saga{BuyerID: buyer, IdempotencyKey: key})
	if err != nil || !created {
		t.Fatalf("first insert: created=%v err=%v", created, err)
	}
	again, created, err := repo.CreateSaga(ctx, store.Saga{BuyerID: buyer, IdempotencyKey: key})
	if err != nil || created || again.ID != first.ID || again.IdempotencyKey != key {
		t.Fatalf("same buyer+key must return the holder: created=%v err=%v %+v", created, err, again)
	}
	if _, created, err := repo.CreateSaga(ctx, store.Saga{BuyerID: other, IdempotencyKey: key}); err != nil || !created {
		t.Fatalf("keys are per buyer: created=%v err=%v", created, err)
	}
	// COMPLETED keeps the key.
	if err := repo.UpdateSagaStatus(ctx, first.ID, store.SagaStatusCompleted); err != nil {
		t.Fatal(err)
	}
	if got, created, _ := repo.CreateSaga(ctx, store.Saga{BuyerID: buyer, IdempotencyKey: key}); created || got.Status != store.SagaStatusCompleted {
		t.Fatalf("a completed saga keeps its key: created=%v %+v", created, got)
	}
	// COMPENSATED frees it.
	second, _, _ := repo.CreateSaga(ctx, store.Saga{BuyerID: other, IdempotencyKey: uid("k2")})
	if err := repo.UpdateSagaStatus(ctx, second.ID, store.SagaStatusCompensated); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetSaga(ctx, second.ID); got.IdempotencyKey != "" {
		t.Fatalf("compensation must clear the key, got %q", got.IdempotencyKey)
	}
	// Two unkeyed sagas never collide.
	for i := 0; i < 2; i++ {
		if _, created, err := repo.CreateSaga(ctx, store.Saga{BuyerID: buyer}); err != nil || !created {
			t.Fatalf("unkeyed insert %d: %v", i, err)
		}
	}
}

func TestSagaIdempotencyKey_InMemory(t *testing.T) {
	runSagaKeyContract(t, store.NewInMemorySagaRepository())
}

func TestSagaIdempotencyKey_Postgres(t *testing.T) {
	runSagaKeyContract(t, store.NewPostgresSagaRepository(pgPool(t)))
}
