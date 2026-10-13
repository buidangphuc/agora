package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
)

// paymentSettledType is the fully-qualified proto name the EventEnvelope carries
// for a settled payment.
const paymentSettledType = "platform.payment.v1.PaymentSettled"

// PaymentConsumerName namespaces this consumer's rows in the dedupe ledger.
const PaymentConsumerName = "team-order.payment"

// VoucherCommitter commits a voucher reservation once its order is settled. The
// order id is the reservation_id (see order redemption), so the consumer needs
// only the order to close the hold. CommitReservation is idempotent on that id.
// The generated promotionv1.VoucherServiceClient satisfies this.
type VoucherCommitter interface {
	CommitReservation(ctx context.Context, in *promotionv1.CommitReservationRequest, opts ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error)
}

// PaymentConsumer applies PaymentSettled events to orders idempotently. It is
// the record handler of the go-sdk consumer (Handle): the consumer owns polling,
// retries, the dead-letter topic and offset commits.
type PaymentConsumer struct {
	tx     store.Transactor
	promo  VoucherCommitter
	logger *slog.Logger
}

// PaymentConsumerOption customizes a PaymentConsumer without breaking the
// positional constructor.
type PaymentConsumerOption func(*PaymentConsumer)

// WithVoucherCommitter wires the promotion client used to commit a voucher hold
// when its order settles. When unset, orders without a voucher are unaffected and
// orders with one keep their (still-reserved) hold: the commit is simply skipped.
func WithVoucherCommitter(vc VoucherCommitter) PaymentConsumerOption {
	return func(c *PaymentConsumer) {
		if vc != nil {
			c.promo = vc
		}
	}
}

// NewPaymentConsumer builds the consumer over a transactor that runs a delivery
// (PAID transition and outbox row in one transaction, then the dedupe ledger).
func NewPaymentConsumer(tx store.Transactor, logger *slog.Logger, opts ...PaymentConsumerOption) *PaymentConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	c := &PaymentConsumer{tx: tx, logger: logger}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Handle is the consume.Handler: it applies one payment.events record.
func (c *PaymentConsumer) Handle(ctx context.Context, rec consume.Record) error {
	return c.HandleRaw(ctx, rec.Value)
}

// HandleRaw decodes a Kafka record value (a marshalled EventEnvelope) and applies
// it.
func (c *PaymentConsumer) HandleRaw(ctx context.Context, value []byte) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		// A malformed envelope is a poison record: it can never succeed, so it is
		// reported as permanent and goes to the DLQ rather than being retried.
		return consume.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}
	return c.HandleEnvelope(ctx, &env)
}

// HandleEnvelope applies one decoded envelope. Non-PaymentSettled envelopes are
// ignored. Delivery is at-least-once, so the effect is guarded two ways: a read of
// the dedupe ledger skips an event already processed, and the order transition is
// a compare-and-set from PENDING only, so consuming the same settlement twice
// transitions the order exactly once (H3).
//
// A delivery runs in this order:
//  1. read the ledger; a processed event is acknowledged at once;
//  2. one transaction: the PAID compare-and-set and its outbox row, committed;
//  3. the voucher commit to team-promotion;
//  4. only then the event is marked processed, in its own small transaction.
//
// A failed voucher commit therefore leaves the order PAID (the buyer paid) with
// its outbox row, and the event unprocessed: the redelivery finds the order
// already PAID (the transition is a no-op) and retries the voucher commit, which
// is idempotent on the order id. A failed transition rolls back and marks nothing.
func (c *PaymentConsumer) HandleEnvelope(ctx context.Context, env *eventsv1.EventEnvelope) error {
	if env.GetType() != paymentSettledType {
		return nil // not ours; ignore
	}
	if env.GetEventId() == "" {
		return consume.Permanent(errors.New("envelope missing event_id"))
	}
	var settled paymentv1.PaymentSettled
	if err := proto.Unmarshal(env.GetPayload(), &settled); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal PaymentSettled: %w", err))
	}
	if settled.GetOrderId() == "" {
		return consume.Permanent(errors.New("PaymentSettled missing order_id"))
	}

	done, err := c.tx.Processed(ctx, PaymentConsumerName, env.GetEventId())
	if err != nil {
		return err // transient: retry, do not commit the offset
	}
	if done {
		return nil // fully processed earlier: idempotent no-op
	}

	var settledOrder *store.Order
	if err := c.tx.InTx(ctx, func(u store.Unit) error {
		o, err := c.transition(ctx, u, &settled)
		settledOrder = o
		return err
	}); err != nil {
		return err
	}

	if settledOrder != nil {
		if err := c.commitVoucher(ctx, *settledOrder); err != nil {
			return err
		}
	}
	return c.tx.MarkProcessed(ctx, PaymentConsumerName, env.GetEventId())
}

// transition moves the order to PAID on a settled payment (design D11). The move
// is a compare-and-set from Pending only, which also records paid_at: a
// settlement for an order in any other status (a late payment after a cancel, a
// redelivery) is acknowledged without changing the order and logged with its
// current status. It returns the order as it stands, or nil when the payment is
// not a settled one and no order is involved.
func (c *PaymentConsumer) transition(ctx context.Context, u store.Unit, settled *paymentv1.PaymentSettled) (*store.Order, error) {
	if settled.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID {
		// FAILED (or unspecified): do not drive the order to PAID. Leave the order in
		// its current state for the buyer to retry; the event is still deduped.
		c.logger.InfoContext(ctx, "payment not settled; no PAID transition",
			"order_id", settled.GetOrderId(), "status", int(settled.GetStatus()))
		return nil, nil
	}

	// the transition table is the single source of truth for which statuses
	// settlement may move to PAID (today only PENDING)
	o, err := u.UpdateOrderStatusFrom(ctx, settled.GetOrderId(), store.OrderStatusPaid,
		order.AllowedFrom(store.OrderStatusPaid, order.ActorSystem), "")
	switch {
	case err == nil:
		c.logger.InfoContext(ctx, "order transitioned to PAID from PaymentSettled",
			"order_id", o.ID, "payment_id", settled.GetPaymentId())
	case errors.Is(err, store.ErrOrderNotFound):
		// The order should exist; treat absence as permanent so it is DLQ'd for
		// inspection rather than retried forever.
		return nil, consume.Permanent(fmt.Errorf("order %q not found for settled payment", settled.GetOrderId()))
	case errors.Is(err, store.ErrStatusConflict):
		current, gerr := u.GetOrder(ctx, settled.GetOrderId())
		if gerr != nil {
			return nil, fmt.Errorf("get order %q: %w", settled.GetOrderId(), gerr)
		}
		o = current
		c.logger.WarnContext(ctx, "settled payment for an order that is no longer pending; acknowledged without change",
			"order_id", o.ID, "status", int(o.Status), "payment_id", settled.GetPaymentId())
	default:
		return nil, fmt.Errorf("transition order %q to PAID: %w", settled.GetOrderId(), err)
	}
	return &o, nil
}

// commitVoucher commits the order's voucher hold, only for an order that reached
// Paid (paid_at set, by this write or an earlier delivery), so a cancelled
// order's voucher is never redeemed by a late payment. An error is returned so
// the event stays unprocessed and redelivers; the commit is idempotent.
func (c *PaymentConsumer) commitVoucher(ctx context.Context, o store.Order) error {
	if c.promo == nil || o.VoucherCode == "" || o.PaidAt == nil {
		return nil
	}
	if _, err := c.promo.CommitReservation(ctx, &promotionv1.CommitReservationRequest{ReservationId: o.ID}); err != nil {
		return fmt.Errorf("commit voucher for order %q: %w", o.ID, err)
	}
	c.logger.InfoContext(ctx, "committed voucher reservation on settle",
		"order_id", o.ID, "voucher_code", o.VoucherCode)
	return nil
}
