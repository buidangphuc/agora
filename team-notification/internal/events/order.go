package events

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	orderv1 "github.com/buidangphuc/team-notification/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-notification/generated/platform/payment/v1"
)

const (
	orderShippedType  = "platform.order.v1.OrderShipped"
	evOrderPlaced     = "platform.order.v1.OrderPlaced"
	evOrderPaid       = "platform.order.v1.OrderPaidEvent"
	evOrderReminder   = "platform.order.v1.OrderPaymentReminder"
	evOrderCancelled  = "platform.order.v1.OrderCancelled"
	evOrderCompleted  = "platform.order.v1.OrderCompleted"
	evReturnRequested = "platform.order.v1.ReturnRequested"
	evReturnRefunded  = "platform.order.v1.ReturnRefunded"
)

func NewOrderHandler(pool TxBeginner, logger *slog.Logger) consume.Handler {
	logger = orDefault(logger)
	return func(ctx context.Context, r consume.Record) error {
		env, err := decode(r.Value)
		if err != nil {
			return err
		}
		var msg proto.Message
		switch env.GetType() {
		case orderShippedType:
			msg = &orderv1.OrderShipped{}
		case evOrderPlaced:
			msg = &orderv1.OrderPlaced{}
		case evOrderPaid:
			msg = &orderv1.OrderPaidEvent{}
		case evOrderReminder:
			msg = &orderv1.OrderPaymentReminder{}
		case evOrderCancelled:
			msg = &orderv1.OrderCancelled{}
		case evOrderCompleted:
			msg = &orderv1.OrderCompleted{}
		case evReturnRequested:
			msg = &orderv1.ReturnRequested{}
		case evReturnRefunded:
			msg = &orderv1.ReturnRefunded{}
		default:
			return nil
		}
		return apply(ctx, pool, orderConsumerName, env.GetEventId(), func(tx pgx.Tx) error {
			if err := unmarshal(env.GetPayload(), msg); err != nil {
				return err
			}
			n, err := buildOrderNotification(ctx, logger, msg)
			if err != nil || n == nil {
				return err
			}
			return notifyRespectingPrefs(ctx, tx, logger, n)
		})
	}
}

func orderNotification(user, title, body, link string) *notificationv1.Notification {
	return &notificationv1.Notification{
		UserId:  user,
		Title:   title,
		Body:    body,
		Type:    notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER,
		LinkUrl: link,
	}
}

// build maps a decoded order event to its notification; nil means nobody to notify.
func buildOrderNotification(ctx context.Context, logger *slog.Logger, msg proto.Message) (*notificationv1.Notification, error) {
	skip := func(what string) (*notificationv1.Notification, error) {
		logger.WarnContext(ctx, "order event missing recipient; skipping", "event", what)
		return nil, nil
	}
	switch e := msg.(type) {
	case *orderv1.OrderShipped:
		if e.GetOrderId() == "" || e.GetBuyerId() == "" {
			return nil, consume.Permanent(errMissing("OrderShipped order_id or buyer_id"))
		}
		carrier := strings.TrimSpace(e.GetCarrier())
		if carrier == "" {
			carrier = "đơn vị vận chuyển"
		}
		return orderNotification(e.GetBuyerId(), "Đơn hàng đã được giao cho "+carrier,
			"Mã vận đơn: "+e.GetTrackingCode(), "/account/orders/"+e.GetOrderId()), nil
	case *orderv1.OrderPlaced:
		if e.GetPaymentMethod() != paymentv1.PaymentMethod_PAYMENT_METHOD_COD {
			return nil, nil
		}
		if e.GetSellerId() == "" {
			return skip("OrderPlaced")
		}
		return sellerNewOrder(e.GetSellerId(), e.GetOrderId(), e.GetTotalAmount(), e.GetCurrency()), nil
	case *orderv1.OrderPaidEvent:
		if e.GetSellerId() == "" {
			return skip("OrderPaidEvent")
		}
		return sellerNewOrder(e.GetSellerId(), e.GetOrderId(), e.GetTotalAmount(), e.GetCurrency()), nil
	case *orderv1.OrderPaymentReminder:
		if e.GetBuyerId() == "" {
			return skip("OrderPaymentReminder")
		}
		return orderNotification(e.GetBuyerId(), "Đơn hàng sắp hết hạn thanh toán",
			fmt.Sprintf("Đơn %s (%d %s) cần thanh toán trước %s", e.GetOrderId(), e.GetTotalAmount(), e.GetCurrency(),
				e.GetPaymentDueAt().AsTime().Format("15:04 02/01/2006")),
			"/checkout/pay/"+e.GetOrderId()), nil
	case *orderv1.OrderCancelled:
		if e.GetReasonCode() != orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED {
			return nil, nil
		}
		if e.GetBuyerId() == "" {
			return skip("OrderCancelled")
		}
		return orderNotification(e.GetBuyerId(), "Đơn hàng đã bị hủy do quá hạn thanh toán",
			"Đơn "+e.GetOrderId()+" đã bị hủy vì không được thanh toán đúng hạn", "/account/orders/"+e.GetOrderId()), nil
	case *orderv1.OrderCompleted:
		if e.GetBuyerId() == "" {
			return skip("OrderCompleted")
		}
		return orderNotification(e.GetBuyerId(), "Đơn hàng đã hoàn tất",
			fmt.Sprintf("Đơn %s đã hoàn tất. Hãy đánh giá sản phẩm; có thể trả hàng đến hết %s", e.GetOrderId(),
				e.GetReturnWindowEndsAt().AsTime().Format("02/01/2006")),
			"/account/orders/"+e.GetOrderId()), nil
	case *orderv1.ReturnRequested:
		if e.GetSellerId() == "" {
			return skip("ReturnRequested")
		}
		return orderNotification(e.GetSellerId(), "Yêu cầu trả hàng mới",
			"Đơn "+e.GetOrderId()+" có yêu cầu trả hàng mới", "/seller/orders/"+e.GetOrderId()+"?tab=returns"), nil
	case *orderv1.ReturnRefunded:
		if e.GetBuyerId() == "" {
			return skip("ReturnRefunded")
		}
		return orderNotification(e.GetBuyerId(), "Yêu cầu trả hàng đã được hoàn tiền",
			fmt.Sprintf("Đơn %s đã được hoàn %d", e.GetOrderId(), e.GetRefundAmount()),
			"/account/orders/"+e.GetOrderId()), nil
	}
	return nil, nil
}

func sellerNewOrder(seller, orderID string, amount int64, currency string) *notificationv1.Notification {
	return orderNotification(seller, "Đơn hàng mới",
		fmt.Sprintf("Đơn %s trị giá %d %s", orderID, amount, currency), "/seller/orders/"+orderID)
}

// Run consumes order.events until ctx is cancelled.
