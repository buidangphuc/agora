// Package consumer holds team-order's Kafka event consumers. The PaymentSettled
// consumer transitions an order to PAID by consuming the event-carried
// platform.payment.v1.PaymentSettled (AD4) instead of a synchronous RPC, and does
// so idempotently (AD4/H3) so redelivery never double-applies.
//
// The transport (a real Kafka reader + DLQ producer) is injected as interfaces so
// the consumption logic is unit-testable without a broker. Wiring the concrete
// franz-go reader lives in cmd/server + bootstrap (a Wave-3 wiring item); it must
// not change any constructor here.
package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

// paymentSettledType is the fully-qualified proto name the EventEnvelope carries
// for a settled payment.
const paymentSettledType = "platform.payment.v1.PaymentSettled"

// consumerName namespaces this consumer's rows in the dedupe ledger.
const consumerName = "team-order.payment"

// OrderStore is the slice of the order repository the consumer needs.
type OrderStore interface {
	GetOrder(ctx context.Context, id string) (repository.Order, error)
	UpdateOrderStatusFrom(ctx context.Context, id string, to repository.OrderStatus, allowedFrom []repository.OrderStatus, trackingNumber string) (repository.Order, error)
}

// VoucherCommitter commits a voucher reservation once its order is settled. The
// order id is the reservation_id (see service redemption), so the consumer needs
// only the order to close the hold. CommitReservation is idempotent on that id.
// The generated promotionv1.VoucherServiceClient satisfies this.
type VoucherCommitter interface {
	CommitReservation(ctx context.Context, in *promotionv1.CommitReservationRequest, opts ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error)
}

// PaymentConsumer applies PaymentSettled events to orders idempotently.
type PaymentConsumer struct {
	orders OrderStore
	dedupe repository.ProcessedEventRepository
	promo  VoucherCommitter
	logger *slog.Logger
}

// PaymentConsumerOption customizes a PaymentConsumer without breaking the
// positional constructor (existing call sites keep compiling).
type PaymentConsumerOption func(*PaymentConsumer)

// WithVoucherCommitter wires the promotion client used to commit a voucher hold
// when its order settles. When unset, orders without a voucher are unaffected and
// orders with one keep their (still-reserved) hold — the commit is simply skipped.
func WithVoucherCommitter(vc VoucherCommitter) PaymentConsumerOption {
	return func(c *PaymentConsumer) {
		if vc != nil {
			c.promo = vc
		}
	}
}

// NewPaymentConsumer builds the consumer over an order store and a dedupe ledger.
func NewPaymentConsumer(orders OrderStore, dedupe repository.ProcessedEventRepository, logger *slog.Logger, opts ...PaymentConsumerOption) *PaymentConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	if dedupe == nil {
		dedupe = repository.NewInMemoryProcessedEventRepository()
	}
	c := &PaymentConsumer{orders: orders, dedupe: dedupe, logger: logger}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HandleRaw decodes a Kafka record value (a marshalled EventEnvelope) and applies
// it. It is the entry point the transport loop calls per record.
func (c *PaymentConsumer) HandleRaw(ctx context.Context, value []byte) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		// A malformed envelope is a poison record: it can never succeed, so report
		// a permanent error the loop routes to the DLQ rather than retrying forever.
		return fmt.Errorf("%w: unmarshal envelope: %v", ErrPermanent, err)
	}
	return c.HandleEnvelope(ctx, &env)
}

// HandleEnvelope applies one decoded envelope. Non-PaymentSettled envelopes are
// ignored. Delivery is at-least-once, so the effect is guarded two ways: the
// dedupe ledger short-circuits a redelivered event_id, and the order transition
// itself is applied only when the order is still PENDING — together they make
// consuming the same settlement twice transition the order exactly once (H3).
func (c *PaymentConsumer) HandleEnvelope(ctx context.Context, env *eventsv1.EventEnvelope) error {
	if env.GetType() != paymentSettledType {
		return nil // not ours; ignore
	}
	if env.GetEventId() == "" {
		return fmt.Errorf("%w: envelope missing event_id", ErrPermanent)
	}

	already, err := c.dedupe.IsProcessed(ctx, consumerName, env.GetEventId())
	if err != nil {
		return fmt.Errorf("dedupe lookup: %w", err) // transient → retry, don't commit
	}
	if already {
		return nil // duplicate delivery — idempotent no-op
	}

	var settled paymentv1.PaymentSettled
	if err := proto.Unmarshal(env.GetPayload(), &settled); err != nil {
		return fmt.Errorf("%w: unmarshal PaymentSettled: %v", ErrPermanent, err)
	}
	if settled.GetOrderId() == "" {
		return fmt.Errorf("%w: PaymentSettled missing order_id", ErrPermanent)
	}

	if err := c.apply(ctx, &settled); err != nil {
		return err // transient (e.g. DB blip) → retry, don't commit the offset
	}

	// Record the event only AFTER the effect is applied. A crash between apply and
	// mark simply redelivers; the transition is a no-op the second time.
	if _, err := c.dedupe.MarkProcessed(ctx, consumerName, env.GetEventId()); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}

// apply moves the order to PAID on a settled payment (design D11). The move is a
// compare-and-set from Pending only, which also records paid_at: a settlement for
// an order in any other status (a late payment after a cancel, a redelivery) is
// acknowledged without changing the order and logged with its current status.
// The voucher hold is committed only for an order that reached Paid (paid_at set,
// by this write or an earlier delivery), so a cancelled order's voucher is never
// redeemed by a late payment. A commit error is returned so the event redelivers;
// the redelivery's CAS conflicts but paid_at is set, so the commit is retried.
func (c *PaymentConsumer) apply(ctx context.Context, settled *paymentv1.PaymentSettled) error {
	if settled.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID {
		// FAILED (or unspecified): do not drive the order to PAID. Leave the order in
		// its current state for the buyer to retry; the event is still deduped.
		c.logger.InfoContext(ctx, "payment not settled; no PAID transition",
			slog.String("order_id", settled.GetOrderId()),
			slog.Int("status", int(settled.GetStatus())),
		)
		return nil
	}

	// the transition table is the single source of truth for which statuses
	// settlement may move to PAID (today only PENDING)
	order, err := c.orders.UpdateOrderStatusFrom(ctx, settled.GetOrderId(), repository.OrderStatusPaid,
		service.AllowedFrom(repository.OrderStatusPaid, service.ActorSystem), "")
	switch {
	case err == nil:
		c.logger.InfoContext(ctx, "order transitioned to PAID from PaymentSettled",
			slog.String("order_id", order.ID),
			slog.String("payment_id", settled.GetPaymentId()),
		)
	case errors.Is(err, repository.ErrOrderNotFound):
		// The order should exist; treat absence as permanent so it is DLQ'd for
		// inspection rather than retried forever.
		return fmt.Errorf("%w: order %q not found for settled payment", ErrPermanent, settled.GetOrderId())
	case errors.Is(err, repository.ErrStatusConflict):
		current, gerr := c.orders.GetOrder(ctx, settled.GetOrderId())
		if gerr != nil {
			return fmt.Errorf("get order %q: %w", settled.GetOrderId(), gerr)
		}
		order = current
		c.logger.WarnContext(ctx, "settled payment for an order that is no longer pending; acknowledged without change",
			slog.String("order_id", order.ID),
			slog.Int("status", int(order.Status)),
			slog.String("payment_id", settled.GetPaymentId()),
		)
	default:
		return fmt.Errorf("transition order %q to PAID: %w", settled.GetOrderId(), err)
	}

	if c.promo != nil && order.VoucherCode != "" && order.PaidAt != nil {
		if _, err := c.promo.CommitReservation(ctx, &promotionv1.CommitReservationRequest{ReservationId: order.ID}); err != nil {
			return fmt.Errorf("commit voucher for order %q: %w", order.ID, err)
		}
		c.logger.InfoContext(ctx, "committed voucher reservation on settle",
			slog.String("order_id", order.ID), slog.String("voucher_code", order.VoucherCode))
	}
	return nil
}

// ── Transport loop (AD1 offset discipline) ──

// ErrPermanent marks an error that will never succeed on retry (a poison record).
// The loop routes such records to the DLQ instead of retrying indefinitely.
var ErrPermanent = errors.New("permanent consumer error")

// Record is one Kafka record the transport hands to the loop.
type Record struct {
	Key   string
	Value []byte
}

// RecordReader is the transport the loop reads from. A real franz-go reader
// satisfies it; tests use a fake. Fetch blocks until the next record (or ctx is
// done); Commit advances the committed offset past rec.
type RecordReader interface {
	Fetch(ctx context.Context) (Record, error)
	Commit(ctx context.Context, rec Record) error
}

// DeadLetterProducer publishes poison / max-retried records to a DLQ topic (AD1).
type DeadLetterProducer interface {
	Produce(ctx context.Context, topic string, rec Record) error
}

// RunConfig tunes the consume loop.
type RunConfig struct {
	DLQTopic    string        // e.g. "payment.events.dlq"
	MaxAttempts int           // in-process attempts before DLQ (default 5)
	BaseBackoff time.Duration // first retry delay; doubles per attempt (default 200ms)
	// FetchRetryBackoff spaces out retries after a failed fetch (default 1s).
	FetchRetryBackoff time.Duration
}

func (c RunConfig) withDefaults() RunConfig {
	if c.DLQTopic == "" {
		c.DLQTopic = "payment.events.dlq"
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 200 * time.Millisecond
	}
	if c.FetchRetryBackoff <= 0 {
		c.FetchRetryBackoff = time.Second
	}
	return c
}

// Run consumes until ctx is cancelled. Per record it applies the handler with
// bounded in-process retries; on a permanent error or once retries are exhausted
// it produces the record to the DLQ. Crucially it commits the offset ONLY after
// the record is either applied or DLQ'd — it never advances past an unprocessed
// record (AD1).
func (c *PaymentConsumer) Run(ctx context.Context, reader RecordReader, dlq DeadLetterProducer, cfg RunConfig) error {
	cfg = cfg.withDefaults()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rec, err := reader.Fetch(ctx)
		if err != nil {
			// Stop only when our own context is done. A fetch error that merely
			// wraps a deadline (a Kafka request timing out under load) is transient;
			// returning on it silently stopped the consumer and left orders PENDING.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logger.WarnContext(ctx, "fetch record failed; retrying", slog.Any("err", err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cfg.FetchRetryBackoff):
			}
			continue
		}

		if perr := c.processWithRetry(ctx, rec, dlq, cfg); perr != nil {
			// Could not process AND could not DLQ: do NOT commit — leave the offset so
			// the record is redelivered (never lose it).
			c.logger.ErrorContext(ctx, "record neither applied nor DLQ'd; not committing",
				slog.Any("err", perr))
			continue
		}
		if err := reader.Commit(ctx, rec); err != nil {
			c.logger.WarnContext(ctx, "commit offset failed; record may redeliver", slog.Any("err", err))
		}
	}
}

func (c *PaymentConsumer) processWithRetry(ctx context.Context, rec Record, dlq DeadLetterProducer, cfg RunConfig) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := c.HandleRaw(ctx, rec.Value)
		if err == nil {
			return nil
		}
		lastErr = err
		if errors.Is(err, ErrPermanent) {
			break // no point retrying a poison record
		}
		if attempt < cfg.MaxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cfg.BaseBackoff * time.Duration(attempt)):
			}
		}
	}
	c.logger.WarnContext(ctx, "routing record to DLQ",
		slog.String("dlq_topic", cfg.DLQTopic), slog.Any("err", lastErr))
	if err := dlq.Produce(ctx, cfg.DLQTopic, rec); err != nil {
		return fmt.Errorf("produce to DLQ: %w (original: %v)", err, lastErr)
	}
	return nil
}
