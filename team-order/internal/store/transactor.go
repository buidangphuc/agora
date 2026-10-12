package store

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
)

// Unit is what one delivery of a consumed event may do inside a single
// transaction: read and move orders, and record the event in the dedupe ledger.
// Everything it did commits or rolls back together.
type Unit interface {
	GetOrder(ctx context.Context, id string) (Order, error)
	// UpdateOrderStatusFrom is the compare-and-set of OrderRepository.
	UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string) (Order, error)
	// Seen records (consumer, eventID) in the ledger and reports whether it was
	// already there, in which case the event must be skipped.
	Seen(ctx context.Context, consumer, eventID string) (bool, error)
}

// Transactor runs fn as one transaction: commit when fn returns nil, roll back
// otherwise, so a failed delivery leaves neither a transition nor a dedupe row.
type Transactor interface {
	InTx(ctx context.Context, fn func(Unit) error) error
}

// PostgresTransactor is the Transactor over the order tables and the
// processed_events ledger.
type PostgresTransactor struct {
	pool   *pgxpool.Pool
	orders *PostgresOrderRepository
	dedupe consume.Dedupe
}

// NewPostgresTransactor binds the transactor to pool. orders supplies the status
// compare-and-set (and the outbox rows it writes).
func NewPostgresTransactor(pool *pgxpool.Pool, orders *PostgresOrderRepository) *PostgresTransactor {
	return &PostgresTransactor{pool: pool, orders: orders, dedupe: consume.NewDedupe("")}
}

// InTx implements Transactor.
func (t *PostgresTransactor) InTx(ctx context.Context, fn func(Unit) error) error {
	return sdkpgx.WithTx(ctx, t.pool, func(tx pgx.Tx) error {
		return fn(pgUnit{tx: tx, orders: t.orders, dedupe: t.dedupe})
	})
}

type pgUnit struct {
	tx     pgx.Tx
	orders *PostgresOrderRepository
	dedupe consume.Dedupe
}

func (u pgUnit) GetOrder(ctx context.Context, id string) (Order, error) {
	return getOrderTx(ctx, u.tx, id)
}

func (u pgUnit) UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string) (Order, error) {
	return u.orders.UpdateOrderStatusFromTx(ctx, u.tx, id, to, allowedFrom, trackingNumber)
}

func (u pgUnit) Seen(ctx context.Context, consumer, eventID string) (bool, error) {
	return u.dedupe.Seen(ctx, u.tx, consumer, eventID)
}

// OrderStatusStore is the slice of an order repository a Unit needs. The
// in-memory repository and test doubles satisfy it.
type OrderStatusStore interface {
	GetOrder(ctx context.Context, id string) (Order, error)
	UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string) (Order, error)
}

// InMemoryTransactor is the Transactor over in-memory repositories, for unit
// tests. It models the ledger's transaction only: the (consumer, event) marks
// made by a failed fn are dropped, as a rollback would, while writes to the
// order store stay, because the store has no transaction to roll back.
type InMemoryTransactor struct {
	orders OrderStatusStore
	mu     sync.Mutex
	seen   map[[2]string]struct{}
}

// NewInMemoryTransactor wraps orders and an empty ledger.
func NewInMemoryTransactor(orders OrderStatusStore) *InMemoryTransactor {
	return &InMemoryTransactor{orders: orders, seen: map[[2]string]struct{}{}}
}

// InTx implements Transactor.
func (t *InMemoryTransactor) InTx(_ context.Context, fn func(Unit) error) error {
	u := &memUnit{t: t}
	if err := fn(u); err != nil {
		t.mu.Lock()
		for _, k := range u.marked {
			delete(t.seen, k)
		}
		t.mu.Unlock()
		return err
	}
	return nil
}

type memUnit struct {
	t      *InMemoryTransactor
	marked [][2]string
}

func (u *memUnit) GetOrder(ctx context.Context, id string) (Order, error) {
	return u.t.orders.GetOrder(ctx, id)
}

func (u *memUnit) UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string) (Order, error) {
	return u.t.orders.UpdateOrderStatusFrom(ctx, id, to, allowedFrom, trackingNumber)
}

func (u *memUnit) Seen(_ context.Context, consumer, eventID string) (bool, error) {
	u.t.mu.Lock()
	defer u.t.mu.Unlock()
	k := [2]string{consumer, eventID}
	if _, ok := u.t.seen[k]; ok {
		return true, nil
	}
	u.t.seen[k] = struct{}{}
	u.marked = append(u.marked, k)
	return false, nil
}
