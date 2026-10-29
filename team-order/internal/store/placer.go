package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrReservationLost is returned by PlaceOrders when a reservation it must bind is
// no longer RESERVED (a sweep released it meanwhile, or it was never persisted), or
// the checkout attempt is no longer PENDING: the orders must not be placed, their
// stock may already be back on sale.
var ErrReservationLost = errors.New("item is no longer reserved")

// PlacedOrder is one order of a checkout with the reservations holding its stock.
type PlacedOrder struct {
	Order          Order
	ReservationIDs []string
	// PaymentDeadline, when > 0, sets the order's payment_due_at to its created_at
	// plus this duration; zero (COD) leaves it unset.
	PaymentDeadline time.Duration
}

// OrderPlacer places every order of one checkout attempt as ONE atomic step: it
// inserts every order and its items, binds each reservation to its order
// (RESERVED -> COMMITTED, order_id set) and marks the saga COMPLETED. Either all of
// it happened or none of it did; any error leaves no order behind (design D6).
type OrderPlacer interface {
	PlaceOrders(ctx context.Context, sagaID string, placed []PlacedOrder) ([]Order, error)
}

const bindReservationsSQL = `UPDATE order_reservations
	SET status = $3, order_id = $2, updated_at = now()
	WHERE id = ANY($1) AND status = $4`

const completeSagaSQL = `UPDATE order_sagas SET status = $2, updated_at = now() WHERE id = $1 AND status = $3`

func PrepareOrder(o *Order, now time.Time, deadline, shipBy time.Duration) {
	if o.ID == "" {
		o.ID = uuid.NewString()
	}
	if o.Currency == "" {
		o.Currency = "VND"
	}
	if o.Status == 0 {
		o.Status = OrderStatusPending
	}
	o.CreatedAt = now
	o.UpdatedAt = now
	o.PaymentDueAt = nil
	if deadline > 0 {
		due := now.Add(deadline)
		o.PaymentDueAt = &due
	}
	o.ShipBy = nil
	if o.PaymentMethod == CodPaymentMethod {
		o.ShipBy = stampAfter(now, shipBy)
	}
	items := make([]OrderItem, len(o.Items))
	copy(items, o.Items)
	for i := range items {
		if items[i].ID == "" {
			items[i].ID = uuid.NewString()
		}
		items[i].OrderID = o.ID
	}
	o.Items = items
}

// PlaceOrders implements OrderPlacer in one pgx transaction. Each order's binding
// must affect exactly len(ReservationIDs) rows and the saga must still be PENDING,
// otherwise everything rolls back with ErrReservationLost.
func (r *PostgresOrderRepository) PlaceOrders(ctx context.Context, sagaID string, placed []PlacedOrder) ([]Order, error) {
	if len(placed) == 0 {
		return nil, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now()
	out := make([]Order, 0, len(placed))
	for _, p := range placed {
		o := p.Order
		PrepareOrder(&o, now, p.PaymentDeadline, r.shipByAfter)
		if err := insertOrderTx(ctx, tx, &o); err != nil {
			return nil, err
		}
		if r.placedOutbox != nil {
			row, err := r.placedOutbox(o)
			if err != nil {
				return nil, fmt.Errorf("build order placed outbox row: %w", err)
			}
			if err := r.outbox.EnqueueTx(ctx, tx, row); err != nil {
				return nil, err
			}
		}
		if err := bindReservationsTx(ctx, tx, o.ID, p.ReservationIDs); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	ct, err := tx.Exec(ctx, completeSagaSQL, sagaID, int32(SagaStatusCompleted), int32(SagaStatusPending))
	if err != nil {
		return nil, fmt.Errorf("complete saga %s: %w", sagaID, err)
	}
	if ct.RowsAffected() != 1 {
		return nil, fmt.Errorf("%w: checkout attempt %s is no longer pending", ErrReservationLost, sagaID)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return out, nil
}

func bindReservationsTx(ctx context.Context, tx pgx.Tx, orderID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	ct, err := tx.Exec(ctx, bindReservationsSQL, ids, orderID, int32(ReservationStatusCommitted), int32(ReservationStatusReserved))
	if err != nil {
		return fmt.Errorf("bind reservations to order %s: %w", orderID, err)
	}
	if ct.RowsAffected() != int64(len(ids)) {
		return fmt.Errorf("%w: order %s bound %d of %d reservations", ErrReservationLost, orderID, ct.RowsAffected(), len(ids))
	}
	return nil
}
