package store

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
)

// Unit is what the transactional part of one delivery of a consumed event may
// do inside a single transaction: read and move orders. Everything it did commits
// or rolls back together.
type Unit interface {
	GetOrder(ctx context.Context, id string) (Order, error)
	// UpdateOrderStatusFrom is the compare-and-set of OrderRepository.
	UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string) (Order, error)
}

// Transactor runs the steps of one delivery. The dedupe ledger is deliberately
// outside InTx: the event is marked processed only after the work that follows
// the transaction (the voucher commit) has succeeded.
type Transactor interface {
	// InTx runs fn as one transaction: commit when fn returns nil, roll back
	// otherwise.
	InTx(ctx context.Context, fn func(Unit) error) error
	// Processed reports, by a plain read, whether (consumer, eventID) is already
	// in the ledger. It never records anything.
	Processed(ctx context.Context, consumer, eventID string) (bool, error)
	// MarkProcessed records (consumer, eventID) in its own small transaction. A
	// pair that is already recorded is not an error.
	MarkProcessed(ctx context.Context, consumer, eventID string) error
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
	return &PostgresTransactor{pool: pool, orders: orders, dedupe: consume.Dedupe{}}
}

// InTx implements Transactor.
func (t *PostgresTransactor) InTx(ctx context.Context, fn func(Unit) error) error {
	return postgres.WithTx(ctx, t.pool, func(tx pgx.Tx) error {
		return fn(pgUnit{tx: tx, orders: t.orders})
	})
}

// Processed implements Transactor through consume.Dedupe.Processed, a read of
// the ledger that records nothing.
func (t *PostgresTransactor) Processed(ctx context.Context, consumer, eventID string) (bool, error) {
	return t.dedupe.Processed(ctx, t.pool, consumer, eventID)
}

// MarkProcessed implements Transactor through consume.Dedupe.Seen in its own
// transaction.
func (t *PostgresTransactor) MarkProcessed(ctx context.Context, consumer, eventID string) error {
	return postgres.WithTx(ctx, t.pool, func(tx pgx.Tx) error {
		_, err := t.dedupe.Seen(ctx, tx, consumer, eventID)
		return err
	})
}

type pgUnit struct {
	tx     pgx.Tx
	orders *PostgresOrderRepository
}

func (u pgUnit) GetOrder(ctx context.Context, id string) (Order, error) {
	return getOrderTx(ctx, u.tx, id)
}

func (u pgUnit) UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string) (Order, error) {
	return u.orders.UpdateOrderStatusFromTx(ctx, u.tx, id, to, allowedFrom, trackingNumber)
}

// OrderStatusStore is the slice of an order repository a Unit needs. The
// in-memory repository and test doubles satisfy it.
type OrderStatusStore interface {
	GetOrder(ctx context.Context, id string) (Order, error)
	UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string) (Order, error)
}

// InMemoryTransactor is the Transactor over in-memory repositories, for unit
// tests. InTx has no rollback: writes to the order store stay even when fn fails,
// because the store has no transaction to undo.
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
	return fn(&memUnit{t: t})
}

// Processed implements Transactor.
func (t *InMemoryTransactor) Processed(_ context.Context, consumer, eventID string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.seen[[2]string{consumer, eventID}]
	return ok, nil
}

// MarkProcessed implements Transactor.
func (t *InMemoryTransactor) MarkProcessed(_ context.Context, consumer, eventID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seen[[2]string{consumer, eventID}] = struct{}{}
	return nil
}

type memUnit struct{ t *InMemoryTransactor }

func (u *memUnit) GetOrder(ctx context.Context, id string) (Order, error) {
	return u.t.orders.GetOrder(ctx, id)
}

func (u *memUnit) UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string) (Order, error) {
	return u.t.orders.UpdateOrderStatusFrom(ctx, id, to, allowedFrom, trackingNumber)
}
