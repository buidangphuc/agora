package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
)

var ErrPermanent = errors.New("permanent consumer error")

func permanent(format string, args ...any) error {
	return consume.Permanent(fmt.Errorf("%w: "+format, append([]any{ErrPermanent}, args...)...))
}

const (
	paymentSettledType  = "platform.payment.v1.PaymentSettled"
	paymentConsumerName = "team-order.payment"
)

var dedupe consume.Dedupe

type DB interface {
	consume.Querier
	consume.Executor
}

type OrderStore interface {
	GetOrder(ctx context.Context, id string) (store.Order, error)
	UpdateOrderStatusFrom(ctx context.Context, id string, to store.OrderStatus, allowedFrom []store.OrderStatus, trackingNumber string, opts ...store.StatusOption) (store.Order, error)
}

type VoucherCommitter interface {
	CommitReservation(ctx context.Context, in *promotionv1.CommitReservationRequest, opts ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error)
}

type PaymentHandler struct {
	orders OrderStore
	db     DB
	promo  VoucherCommitter
	logger *slog.Logger
}

type PaymentHandlerOption func(*PaymentHandler)

func WithVoucherCommitter(vc VoucherCommitter) PaymentHandlerOption {
	return func(h *PaymentHandler) {
		if vc != nil {
			h.promo = vc
		}
	}
}

func NewPaymentHandler(orders OrderStore, db DB, logger *slog.Logger, opts ...PaymentHandlerOption) *PaymentHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &PaymentHandler{orders: orders, db: db, logger: logger}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *PaymentHandler) Handle(ctx context.Context, rec consume.Record) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		return permanent("unmarshal envelope: %v", err)
	}
	if env.GetType() != paymentSettledType {
		return nil
	}
	if env.GetEventId() == "" {
		return permanent("envelope missing event_id")
	}
	already, err := dedupe.Processed(ctx, h.db, paymentConsumerName, env.GetEventId())
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	var settled paymentv1.PaymentSettled
	if err := proto.Unmarshal(env.GetPayload(), &settled); err != nil {
		return permanent("unmarshal PaymentSettled: %v", err)
	}
	if settled.GetOrderId() == "" {
		return permanent("PaymentSettled missing order_id")
	}
	if err := h.apply(ctx, &settled); err != nil {
		return err
	}
	if _, err := dedupe.Seen(ctx, h.db, paymentConsumerName, env.GetEventId()); err != nil {
		return err
	}
	return nil
}

func (h *PaymentHandler) apply(ctx context.Context, settled *paymentv1.PaymentSettled) error {
	if settled.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID {
		h.logger.InfoContext(ctx, "payment not settled; no PAID transition",
			"order_id", settled.GetOrderId(), "status", int(settled.GetStatus()))
		return nil
	}

	if _, err := h.orders.GetOrder(ctx, settled.GetOrderId()); err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return permanent("order %q not found for settled payment", settled.GetOrderId())
		}
		return fmt.Errorf("get order %q: %w", settled.GetOrderId(), err)
	}

	paid, err := h.orders.UpdateOrderStatusFrom(ctx, settled.GetOrderId(), store.OrderStatusPaid,
		order.AllowedFrom(store.OrderStatusPaid, order.ActorSystem), "")
	switch {
	case err == nil:
		h.logger.InfoContext(ctx, "order transitioned to PAID from PaymentSettled",
			"order_id", paid.ID, "payment_id", settled.GetPaymentId())
	case errors.Is(err, store.ErrOrderNotFound):
		return permanent("order %q not found for settled payment", settled.GetOrderId())
	case errors.Is(err, store.ErrStatusConflict):
		current, gerr := h.orders.GetOrder(ctx, settled.GetOrderId())
		if gerr != nil {
			return fmt.Errorf("get order %q: %w", settled.GetOrderId(), gerr)
		}
		paid = current
		h.logger.WarnContext(ctx, "settled payment for an order that is no longer pending; acknowledged without change",
			"order_id", paid.ID, "status", int(paid.Status), "payment_id", settled.GetPaymentId())
	default:
		return fmt.Errorf("transition order %q to PAID: %w", settled.GetOrderId(), err)
	}

	if h.promo != nil && paid.VoucherCode != "" && paid.PaidAt != nil {
		if _, err := h.promo.CommitReservation(ctx, &promotionv1.CommitReservationRequest{ReservationId: paid.ID}); err != nil {
			return fmt.Errorf("commit voucher for order %q: %w", paid.ID, err)
		}
		h.logger.InfoContext(ctx, "committed voucher reservation on settle",
			"order_id", paid.ID, "voucher_code", paid.VoucherCode)
	}
	if h.promo != nil && paid.PlatformVoucherCode != "" && paid.CheckoutID != "" && paid.PaidAt != nil {
		if _, err := h.promo.CommitReservation(ctx, &promotionv1.CommitReservationRequest{ReservationId: "platform:" + paid.CheckoutID}); err != nil {
			return fmt.Errorf("commit platform voucher for checkout %q: %w", paid.CheckoutID, err)
		}
		h.logger.InfoContext(ctx, "committed platform voucher reservation on settle",
			"order_id", paid.ID, "checkout_id", paid.CheckoutID)
	}
	return nil
}
