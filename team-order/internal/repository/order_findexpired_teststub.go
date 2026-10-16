package repository

// TEST-STUB (task 6): compile-only placeholder so the red tests build. The
// implementer deletes this file and writes the real FindExpired in order.go
// (and adds it to the OrderRepository interface).

import (
	"context"
	"time"
)

func (r *PostgresOrderRepository) FindExpired(_ context.Context, _ time.Time, _ int) ([]Order, error) {
	return nil, nil
}

func (r *InMemoryOrderRepository) FindExpired(_ context.Context, _ time.Time, _ int) ([]Order, error) {
	return nil, nil
}
