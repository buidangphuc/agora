package settlement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
)

const (
	OrderPaidEventType      = "platform.order.v1.OrderPaidEvent"
	OrderCancelledEventType = "platform.order.v1.OrderCancelled"
	ReturnRefundedEventType = "platform.order.v1.ReturnRefunded"
)

var skippedNoBreakdown, _ = otel.Meter("team-payment/settlement").Int64Counter("payment_commission_skipped")

const (
	DefaultTopic    = "order.events"
	DefaultGroup    = "team-payment.settlement"
	DefaultDLQTopic = "order.events.payment-settlement.dlq"
)

type Applier interface {
	CreditSettlement(ctx context.Context, orderID, sellerID string, eventTotal int64) error
	RefundCancelledOrder(ctx context.Context, orderID string) error
	RefundReturn(ctx context.Context, orderID, returnID string, amount int64) error
}

type BreakdownApplier interface {
	CreditSettlementWithBreakdown(ctx context.Context, orderID, sellerID string, eventTotal int64, b payment.Breakdown) error
}

type Voider interface {
	VoidCancelledOrder(ctx context.Context, orderID string) error
}

type Handler struct {
	apply  Applier
	logger *slog.Logger
}

func NewHandler(apply Applier, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{apply: apply, logger: logger}
}

func (h *Handler) Handle(ctx context.Context, r consume.Record) error {
	err := h.handle(ctx, r.Value)
	if errors.Is(err, errPermanent) {
		h.logger.WarnContext(ctx, "settlement handler: permanent failure", "err", err)
	}
	return err
}

var errPermanent = errors.New("permanent")

func permanent(format string, args ...any) error {
	return consume.Permanent(fmt.Errorf("%w: %s", errPermanent, fmt.Sprintf(format, args...)))
}

func (h *Handler) handle(ctx context.Context, value []byte) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return permanent("unmarshal envelope: %v", err)
	}
	switch env.GetType() {
	case OrderPaidEventType, OrderCancelledEventType, ReturnRefundedEventType:
	default:
		return nil
	}
	if env.GetEventId() == "" {
		return permanent("envelope missing event_id")
	}
	switch env.GetType() {
	case OrderCancelledEventType:
		return h.handleCancelled(ctx, &env)
	case ReturnRefundedEventType:
		return h.handleReturnRefunded(ctx, &env)
	}
	return h.handlePaid(ctx, &env)
}

func (h *Handler) handleReturnRefunded(ctx context.Context, env *eventsv1.EventEnvelope) error {
	var rr orderv1.ReturnRefunded
	if err := proto.Unmarshal(env.GetPayload(), &rr); err != nil {
		return permanent("unmarshal ReturnRefunded: %v", err)
	}
	switch {
	case rr.GetReturnId() == "":
		return permanent("ReturnRefunded missing return_id")
	case rr.GetOrderId() == "":
		return permanent("ReturnRefunded missing order_id")
	case rr.GetRefundAmount() <= 0:
		return permanent("ReturnRefunded %s has non-positive refund_amount %d", rr.GetReturnId(), rr.GetRefundAmount())
	}
	if err := h.apply.RefundReturn(ctx, rr.GetOrderId(), rr.GetReturnId(), rr.GetRefundAmount()); err != nil {
		return classify(err, "refund return "+rr.GetReturnId()+" of order "+rr.GetOrderId())
	}
	return nil
}

func (h *Handler) handleCancelled(ctx context.Context, env *eventsv1.EventEnvelope) error {
	var cancelled orderv1.OrderCancelled
	if err := proto.Unmarshal(env.GetPayload(), &cancelled); err != nil {
		return permanent("unmarshal OrderCancelled: %v", err)
	}
	if cancelled.GetOrderId() == "" {
		return permanent("OrderCancelled missing order_id")
	}
	switch cancelled.GetPreviousStatus() {
	case orderv1.OrderStatus_ORDER_STATUS_PAID:
		if err := h.apply.RefundCancelledOrder(ctx, cancelled.GetOrderId()); err != nil {
			return classify(err, "refund cancelled order "+cancelled.GetOrderId())
		}
	case orderv1.OrderStatus_ORDER_STATUS_PENDING:
		v, ok := h.apply.(Voider)
		if !ok {
			return nil
		}
		if err := v.VoidCancelledOrder(ctx, cancelled.GetOrderId()); err != nil {
			return classify(err, "void cancelled order "+cancelled.GetOrderId())
		}
	}
	return nil
}

func (h *Handler) handlePaid(ctx context.Context, env *eventsv1.EventEnvelope) error {
	var paid orderv1.OrderPaidEvent
	if err := proto.Unmarshal(env.GetPayload(), &paid); err != nil {
		return permanent("unmarshal OrderPaidEvent: %v", err)
	}
	if paid.GetOrderId() == "" {
		return permanent("OrderPaidEvent missing order_id")
	}
	seller, err := sellerOf(&paid)
	if err != nil {
		return permanent("order %q: %v", paid.GetOrderId(), err)
	}
	what := "credit settlement for order " + paid.GetOrderId()
	ba, ok := h.apply.(BreakdownApplier)
	if !ok {
		return classify(h.apply.CreditSettlement(ctx, paid.GetOrderId(), seller, paid.GetTotalAmount()), what)
	}
	if paid.ItemsSubtotal == nil || paid.ShippingFee == nil || paid.ShopDiscountAmount == nil || paid.PlatformDiscountAmount == nil {
		skippedNoBreakdown.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "no_breakdown")))
		return classify(h.apply.CreditSettlement(ctx, paid.GetOrderId(), seller, paid.GetTotalAmount()), what)
	}
	b := payment.Breakdown{
		ItemsSubtotal: paid.GetItemsSubtotal(), ShippingFee: paid.GetShippingFee(),
		ShopDiscount: paid.GetShopDiscountAmount(), PlatformDiscount: paid.GetPlatformDiscountAmount(),
	}
	if b.ItemsSubtotal < 0 || b.ShippingFee < 0 || b.ShopDiscount < 0 || b.PlatformDiscount < 0 || b.ShopDiscount > b.ItemsSubtotal {
		return permanent("order %q: inconsistent price breakdown", paid.GetOrderId())
	}
	return classify(ba.CreditSettlementWithBreakdown(ctx, paid.GetOrderId(), seller, paid.GetTotalAmount(), b), what)
}

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

func classify(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, payment.ErrNotSettled) || errors.Is(err, payment.ErrInvalidAmount) {
		return permanent("%s: %v", what, err)
	}
	return fmt.Errorf("%s: %w", what, err)
}
