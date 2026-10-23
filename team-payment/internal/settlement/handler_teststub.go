package settlement

import (
	"context"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
)

const (
	OrderPaidEventType      = "platform.order.v1.OrderPaidEvent"
	OrderCancelledEventType = "platform.order.v1.OrderCancelled"
	ReturnRefundedEventType = "platform.order.v1.ReturnRefunded"
)

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

type Handler struct{}

func NewHandler(Applier, *slog.Logger) *Handler { return &Handler{} }

func (*Handler) Handle(context.Context, consume.Record) error { return nil }
