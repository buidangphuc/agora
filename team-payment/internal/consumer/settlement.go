// Package consumer holds team-payment's consumer of team-order's order.events. It credits
// the seller's wallet ledger for every OrderPaidEvent, and refunds the payment of an order
// cancelled from Paid on OrderCancelled, durably (at-least-once, bounded
// retry, dead-letter topic, commit only after apply or DLQ). The credit is idempotent on
// (ORDER_SETTLEMENT, payment id), so redelivery and replays are no-ops; there is no dedupe
// table. Transport (franz-go reader, DLQ producer) is injected so the logic is testable
// without a broker; the concrete one is in internal/bootstrap. Mirrors team-order's
// PaymentConsumer.Run (design D5).
package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

// Envelope types handled on order.events; every other type is acknowledged without effect.
const (
	OrderPaidEventType      = "platform.order.v1.OrderPaidEvent"
	OrderCancelledEventType = "platform.order.v1.OrderCancelled"
)

// Documented defaults (bootstrap reads the env, design D6).
const (
	DefaultTopic    = "order.events"
	DefaultGroup    = "team-payment.settlement"
	DefaultDLQTopic = "order.events.payment-settlement.dlq"
)

// ErrPermanent marks an error that can never succeed on retry; the record goes straight
// to the DLQ instead of being retried.
var ErrPermanent = errors.New("permanent consumer error")

// Applier applies order facts to the seller ledger. *service.PaymentService satisfies it.
type Applier interface {
	// CreditSettlement credits sellerID for orderID's payment (idempotent).
	CreditSettlement(ctx context.Context, orderID, sellerID string, eventTotal int64) error
	// RefundCancelledOrder refunds a cancelled paid order's payment in full (no-op when
	// already refunded).
	RefundCancelledOrder(ctx context.Context, orderID string) error
}

// SettlementConsumer applies order.events records to the seller ledger.
type SettlementConsumer struct {
	apply  Applier
	logger *slog.Logger
}

// NewSettlementConsumer builds the consumer over an Applier.
func NewSettlementConsumer(apply Applier, logger *slog.Logger) *SettlementConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &SettlementConsumer{apply: apply, logger: logger}
}

// HandleRaw decodes a Kafka record value (a marshalled EventEnvelope) and applies it.
func (c *SettlementConsumer) HandleRaw(ctx context.Context, value []byte) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return fmt.Errorf("%w: unmarshal envelope: %v", ErrPermanent, err)
	}
	return c.HandleEnvelope(ctx, &env)
}

// HandleEnvelope applies one envelope. Errors are ErrPermanent-wrapped for poison input
// or a fact that can never be applied; anything else is retryable (e.g. DB errors).
func (c *SettlementConsumer) HandleEnvelope(ctx context.Context, env *eventsv1.EventEnvelope) error {
	switch env.GetType() {
	case OrderPaidEventType, OrderCancelledEventType:
	default:
		return nil // e.g. OrderShipped: not ours
	}
	if env.GetEventId() == "" {
		return fmt.Errorf("%w: envelope missing event_id", ErrPermanent)
	}
	if env.GetType() == OrderCancelledEventType {
		return c.handleCancelled(ctx, env)
	}
	return c.handlePaid(ctx, env)
}

// handleCancelled refunds the payment of an order cancelled from Paid (design D12); a
// cancel from any other status (Pending) triggers nothing.
func (c *SettlementConsumer) handleCancelled(ctx context.Context, env *eventsv1.EventEnvelope) error {
	var cancelled orderv1.OrderCancelled
	if err := proto.Unmarshal(env.GetPayload(), &cancelled); err != nil {
		return fmt.Errorf("%w: unmarshal OrderCancelled: %v", ErrPermanent, err)
	}
	if cancelled.GetOrderId() == "" {
		return fmt.Errorf("%w: OrderCancelled missing order_id", ErrPermanent)
	}
	if cancelled.GetPreviousStatus() != orderv1.OrderStatus_ORDER_STATUS_PAID {
		return nil
	}
	if err := c.apply.RefundCancelledOrder(ctx, cancelled.GetOrderId()); err != nil {
		return classify(err, "refund cancelled order "+cancelled.GetOrderId())
	}
	return nil
}

func (c *SettlementConsumer) handlePaid(ctx context.Context, env *eventsv1.EventEnvelope) error {
	var paid orderv1.OrderPaidEvent
	if err := proto.Unmarshal(env.GetPayload(), &paid); err != nil {
		return fmt.Errorf("%w: unmarshal OrderPaidEvent: %v", ErrPermanent, err)
	}
	if paid.GetOrderId() == "" {
		return fmt.Errorf("%w: OrderPaidEvent missing order_id", ErrPermanent)
	}
	seller, err := sellerOf(&paid)
	if err != nil {
		return fmt.Errorf("%w: order %q: %v", ErrPermanent, paid.GetOrderId(), err)
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
	if errors.Is(err, service.ErrNotSettled) || errors.Is(err, repository.ErrInvalidAmount) {
		return fmt.Errorf("%w: %s: %v", ErrPermanent, what, err)
	}
	return fmt.Errorf("%s: %w", what, err)
}

// Record is one Kafka record handed over by the transport.
type Record struct {
	Key   string
	Value []byte
}

// RecordReader is the transport the loop reads from. Fetch blocks until the next record
// (or ctx is done); Commit advances the committed offset past rec.
type RecordReader interface {
	Fetch(ctx context.Context) (Record, error)
	Commit(ctx context.Context, rec Record) error
}

// DeadLetterProducer parks poison / max-retried records on a DLQ topic.
type DeadLetterProducer interface {
	Produce(ctx context.Context, topic string, rec Record) error
}

// RunConfig tunes the consume loop.
type RunConfig struct {
	DLQTopic    string        // default DefaultDLQTopic
	MaxAttempts int           // attempts before DLQ (default 5)
	BaseBackoff time.Duration // retry delay = BaseBackoff * attempt (default 200ms)
	FetchPause  time.Duration // pause after a failed Fetch or DLQ produce (default 1s)
}

func (c RunConfig) withDefaults() RunConfig {
	if c.DLQTopic == "" {
		c.DLQTopic = DefaultDLQTopic
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 200 * time.Millisecond
	}
	if c.FetchPause <= 0 {
		c.FetchPause = time.Second
	}
	return c
}

// Run consumes until ctx is cancelled. Per record: bounded retries, then the DLQ. A
// record's offset is committed only after it was applied or DLQ'd. If the DLQ produce
// fails, the SAME record is retried (after FetchPause) and nothing later on the partition
// is processed or committed until it succeeds: committing a later record would move the
// partition offset past it and lose it. A broker outage stalls the consumer rather than
// dropping records.
func (c *SettlementConsumer) Run(ctx context.Context, reader RecordReader, dlq DeadLetterProducer, cfg RunConfig) error {
	cfg = cfg.withDefaults()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rec, err := reader.Fetch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			c.logger.WarnContext(ctx, "settlement consumer: fetch failed", slog.Any("err", err))
			if !sleep(ctx, cfg.FetchPause) {
				return ctx.Err()
			}
			continue
		}
		for {
			perr := c.processWithRetry(ctx, rec, dlq, cfg)
			if perr == nil {
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logger.ErrorContext(ctx, "settlement consumer: record neither applied nor dead-lettered; retrying it, committing nothing",
				slog.String("key", rec.Key), slog.Any("err", perr))
			if !sleep(ctx, cfg.FetchPause) {
				return ctx.Err()
			}
		}
		if err := reader.Commit(ctx, rec); err != nil {
			c.logger.WarnContext(ctx, "settlement consumer: commit failed; record may redeliver", slog.Any("err", err))
		}
	}
}

func (c *SettlementConsumer) processWithRetry(ctx context.Context, rec Record, dlq DeadLetterProducer, cfg RunConfig) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := c.HandleRaw(ctx, rec.Value)
		if err == nil {
			return nil
		}
		lastErr = err
		if errors.Is(err, ErrPermanent) {
			break
		}
		if attempt < cfg.MaxAttempts && !sleep(ctx, cfg.BaseBackoff*time.Duration(attempt)) {
			return ctx.Err()
		}
	}
	c.logger.WarnContext(ctx, "settlement consumer: routing record to DLQ",
		slog.String("dlq_topic", cfg.DLQTopic), slog.String("key", rec.Key), slog.Any("err", lastErr))
	if err := dlq.Produce(ctx, cfg.DLQTopic, rec); err != nil {
		return fmt.Errorf("produce to DLQ: %w (original: %v)", err, lastErr)
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
