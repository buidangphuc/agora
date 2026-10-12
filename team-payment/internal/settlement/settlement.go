// Package settlement holds team-payment's consumer of team-order's order.events. It
// credits the seller's wallet ledger for every OrderPaidEvent, refunds the remainder of
// the payment of an order cancelled from Paid on OrderCancelled, and refunds an RMA
// return on ReturnRefunded (payment-refund-model D7). Delivery is at-least-once through
// the SDK consume loop (bounded retry, dead-letter topic, commit only after handling or
// a successful DLQ produce). Every effect is idempotent in the ledger (the unique
// (type, reference_id) index and the refund keys), so redelivery and replays are no-ops
// and there is no dedupe table: all writes happen in the one database transaction.
package settlement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/store"
)

// Envelope types handled on order.events; every other type is acknowledged without effect.
const (
	OrderPaidEventType      = "platform.order.v1.OrderPaidEvent"
	OrderCancelledEventType = "platform.order.v1.OrderCancelled"
	ReturnRefundedEventType = "platform.order.v1.ReturnRefunded"
)

// Documented defaults (the config tags in internal/config carry the same values).
const (
	DefaultTopic    = "order.events"
	DefaultGroup    = "team-payment.settlement"
	DefaultDLQTopic = "order.events.payment-settlement.dlq"
)

// Applier applies order facts to the seller ledger. *payment.Service satisfies it.
type Applier interface {
	// CreditSettlement credits sellerID for orderID's payment (idempotent).
	CreditSettlement(ctx context.Context, orderID, sellerID string, eventTotal int64) error
	// RefundCancelledOrder refunds what remains of a cancelled paid order's payment (no-op
	// when already refunded or redelivered).
	RefundCancelledOrder(ctx context.Context, orderID string) error
	// RefundReturn refunds an RMA return of orderID by amount, clamped to the payment's
	// remainder, keyed by the return (no-op when redelivered).
	RefundReturn(ctx context.Context, orderID, returnID string, amount int64) error
}

// Handler applies order.events records to the seller ledger.
type Handler struct {
	apply  Applier
	logger *slog.Logger
}

// New builds the handler over an Applier.
func New(apply Applier, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{apply: apply, logger: logger}
}

// Handle is the consume.Handler: it decodes the record value (a marshalled
// EventEnvelope) and applies it.
func (c *Handler) Handle(ctx context.Context, r consume.Record) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(r.Value, &env); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}
	return c.HandleEnvelope(ctx, &env)
}

// HandleEnvelope applies one envelope. Errors are consume.Permanent-wrapped for poison input
// or a fact that can never be applied; anything else is retryable (e.g. DB errors).
func (c *Handler) HandleEnvelope(ctx context.Context, env *eventsv1.EventEnvelope) error {
	switch env.GetType() {
	case OrderPaidEventType, OrderCancelledEventType, ReturnRefundedEventType:
	default:
		return nil // e.g. OrderShipped: not ours
	}
	if env.GetEventId() == "" {
		return consume.Permanent(errors.New("envelope missing event_id"))
	}
	switch env.GetType() {
	case OrderCancelledEventType:
		return c.handleCancelled(ctx, env)
	case ReturnRefundedEventType:
		return c.handleReturnRefunded(ctx, env)
	}
	return c.handlePaid(ctx, env)
}

// handleReturnRefunded refunds an RMA return (design D6/D7). A malformed fact, or one
// whose order has no settled payment, can never apply and is parked; an applied 0 (the
// payment had nothing left) is not an error.
func (c *Handler) handleReturnRefunded(ctx context.Context, env *eventsv1.EventEnvelope) error {
	var rr orderv1.ReturnRefunded
	if err := proto.Unmarshal(env.GetPayload(), &rr); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal ReturnRefunded: %w", err))
	}
	switch {
	case rr.GetReturnId() == "":
		return consume.Permanent(errors.New("ReturnRefunded missing return_id"))
	case rr.GetOrderId() == "":
		return consume.Permanent(errors.New("ReturnRefunded missing order_id"))
	case rr.GetRefundAmount() <= 0:
		return consume.Permanent(fmt.Errorf("ReturnRefunded %s has non-positive refund_amount %d", rr.GetReturnId(), rr.GetRefundAmount()))
	}
	if err := c.apply.RefundReturn(ctx, rr.GetOrderId(), rr.GetReturnId(), rr.GetRefundAmount()); err != nil {
		return classify(err, "refund return "+rr.GetReturnId()+" of order "+rr.GetOrderId())
	}
	return nil
}

// handleCancelled refunds the payment of an order cancelled from Paid (design D12); a
// cancel from any other status (Pending) triggers nothing.
func (c *Handler) handleCancelled(ctx context.Context, env *eventsv1.EventEnvelope) error {
	var cancelled orderv1.OrderCancelled
	if err := proto.Unmarshal(env.GetPayload(), &cancelled); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal OrderCancelled: %w", err))
	}
	if cancelled.GetOrderId() == "" {
		return consume.Permanent(errors.New("OrderCancelled missing order_id"))
	}
	if cancelled.GetPreviousStatus() != orderv1.OrderStatus_ORDER_STATUS_PAID {
		return nil
	}
	if err := c.apply.RefundCancelledOrder(ctx, cancelled.GetOrderId()); err != nil {
		return classify(err, "refund cancelled order "+cancelled.GetOrderId())
	}
	return nil
}

func (c *Handler) handlePaid(ctx context.Context, env *eventsv1.EventEnvelope) error {
	var paid orderv1.OrderPaidEvent
	if err := proto.Unmarshal(env.GetPayload(), &paid); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal OrderPaidEvent: %w", err))
	}
	if paid.GetOrderId() == "" {
		return consume.Permanent(errors.New("OrderPaidEvent missing order_id"))
	}
	seller, err := sellerOf(&paid)
	if err != nil {
		return consume.Permanent(fmt.Errorf("order %q: %w", paid.GetOrderId(), err))
	}
	if err := c.apply.CreditSettlement(ctx, paid.GetOrderId(), seller, paid.GetTotalAmount()); err != nil {
		return classify(err, "credit settlement for order "+paid.GetOrderId())
	}
	return nil
}

// sellerOf is the order's single seller: every line item's seller_id, non-empty and equal.
func sellerOf(paid *orderv1.OrderPaidEvent) (string, error) {
	var seller string
	for _, it := range paid.GetItems() {
		switch s := it.GetSellerId(); {
		case s == "":
			return "", errors.New("line item without seller_id")
		case seller == "":
			seller = s
		case s != seller:
			return "", errors.New("line items of more than one seller")
		}
	}
	if seller == "" {
		return "", errors.New("no line items")
	}
	return seller, nil
}

// classify marks errors that can never succeed as permanent.
func classify(err error, what string) error {
	if errors.Is(err, payment.ErrNotSettled) || errors.Is(err, store.ErrInvalidAmount) {
		return consume.Permanent(fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}
