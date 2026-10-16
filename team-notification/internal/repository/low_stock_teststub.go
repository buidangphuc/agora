package repository

// TEST-STUB: compile-only placeholder so the consumer Postgres test builds before
// task 3 is implemented. The implementer deletes this file and implements
// PostgresLowStockStore in consumer_state.go.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresLowStockStore struct{ pool *pgxpool.Pool }

func NewPostgresLowStockStore(pool *pgxpool.Pool) *PostgresLowStockStore {
	return &PostgresLowStockStore{pool: pool}
}

var errLowStockStub = errors.New("PostgresLowStockStore not implemented")

func (*PostgresLowStockStore) Get(context.Context, string) (string, string, bool, bool, error) {
	return "", "", false, false, errLowStockStub
}
func (*PostgresLowStockStore) Upsert(context.Context, string, string, string, bool) error {
	return errLowStockStub
}
func (*PostgresLowStockStore) SetLow(context.Context, string, bool) error { return errLowStockStub }
func (*PostgresLowStockStore) Delete(context.Context, string) error       { return errLowStockStub }
