package events_test

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	orderv1 "github.com/buidangphuc/team-notification/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-notification/generated/platform/payment/v1"
	"github.com/buidangphuc/team-notification/internal/events"
)

const (
	orderShippedType  = "platform.order.v1.OrderShipped"
	orderPlacedType   = "platform.order.v1.OrderPlaced"
	orderPaidType     = "platform.order.v1.OrderPaidEvent"
	orderReminderType = "platform.order.v1.OrderPaymentReminder"
	orderCancelType   = "platform.order.v1.OrderCancelled"
	orderDoneType     = "platform.order.v1.OrderCompleted"
	returnReqType     = "platform.order.v1.ReturnRequested"
	returnRefType     = "platform.order.v1.ReturnRefunded"
)

var shipBy = timestamppb.New(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))

func shipped() *orderv1.OrderShipped {
	return &orderv1.OrderShipped{
		OrderId: "ord-1", BuyerId: "buyer-1", SellerId: "seller-1",
		Carrier: "GHN", TrackingCode: "GHN-VN-123", ShippedAt: timestamppb.Now(),
	}
}

func placed(method paymentv1.PaymentMethod) *orderv1.OrderPlaced {
	return &orderv1.OrderPlaced{
		OrderId: "O1", BuyerId: "B1", SellerId: "S1", PaymentMethod: method,
		TotalAmount: 250000, Currency: "VND", ShipBy: shipBy,
	}
}

func paid() *orderv1.OrderPaidEvent {
	return &orderv1.OrderPaidEvent{
		OrderId: "O1", BuyerId: "B1", SellerId: "S1", TotalAmount: 250000, Currency: "VND", ShipBy: shipBy,
	}
}

func reminder() *orderv1.OrderPaymentReminder {
	return &orderv1.OrderPaymentReminder{
		OrderId: "O1", BuyerId: "B1", TotalAmount: 250000, Currency: "VND",
		PaymentDueAt: timestamppb.New(time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)),
	}
}

func completed(id string, by orderv1.CompletedBy) *orderv1.OrderCompleted {
	return &orderv1.OrderCompleted{
		OrderId: id, BuyerId: "B1", SellerId: "S1", CompletedBy: by,
		ReturnWindowEndsAt: timestamppb.New(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)),
	}
}

func assertSellerNew(t *testing.T, n *notificationv1.Notification) {
	t.Helper()
	if n.GetTitle() != "Đơn hàng mới" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/seller/orders/O1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER {
		t.Errorf("type = %v", n.GetType())
	}
	if !strings.Contains(n.GetBody(), "O1") {
		t.Errorf("body %q should name the order id", n.GetBody())
	}
}

func TestOrderShipped_NotifiesBuyer(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "evt-s1", orderShippedType, shipped()))

	n := one(t, pool, "buyer-1")
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER {
		t.Errorf("type = %v", n.GetType())
	}
	if n.GetLinkUrl() != "/account/orders/ord-1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if n.GetTitle() != "Đơn hàng đã được giao cho GHN" || n.GetBody() != "Mã vận đơn: GHN-VN-123" {
		t.Errorf("title/body = %q / %q", n.GetTitle(), n.GetBody())
	}
	if got := totalNotifications(t, pool); got != 1 {
		t.Errorf("only the buyer is notified, total %d", got)
	}
}

func TestOrderShipped_DisabledPreferenceSkips(t *testing.T) {
	pool := newDB(t)
	disable(t, pool, "buyer-1", notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "evt-s-pref", orderShippedType, shipped()))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("disabled ORDER pref must skip, got %d", got)
	}
}

func TestOrder_DisabledPreferenceSkips(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		msg  proto.Message
		user string
	}{
		{"shipped to buyer", orderShippedType, shipped(), "buyer-1"},
		{"paid to seller", orderPaidType, paid(), "S1"},
		{"reminder to buyer", orderReminderType, reminder(), "B1"},
		{"completed to buyer", orderDoneType, completed("O1", orderv1.CompletedBy_COMPLETED_BY_SYSTEM), "B1"},
		{"return requested to seller", returnReqType, &orderv1.ReturnRequested{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1"}, "S1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := newDB(t)
			disable(t, pool, c.user, notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER)
			h := events.NewOrderHandler(pool, nil)

			mustDeliver(t, h, envelopeOf(t, "e1", c.typ, c.msg))

			if got := totalNotifications(t, pool); got != 0 {
				t.Fatalf("disabled ORDER type must skip, got %d notifications", got)
			}
		})
	}
}

func TestOrderShipped_MissingBuyerIsPermanent(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	e := shipped()
	e.BuyerId = ""
	requirePermanent(t, deliver(h, envelopeOf(t, "evt-s-bad", orderShippedType, e)))
	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestOrderShipped_OnlyBuyerOfTheShippedOrderIsNotified(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "evt-s-a", orderShippedType, shipped()))
	other := shipped()
	other.OrderId, other.BuyerId = "ord-2", "buyer-2"
	mustDeliver(t, h, envelopeOf(t, "evt-s-b", orderShippedType, other))

	if n := one(t, pool, "buyer-1"); n.GetLinkUrl() != "/account/orders/ord-1" {
		t.Errorf("buyer-1 link = %q", n.GetLinkUrl())
	}
	if n := one(t, pool, "buyer-2"); n.GetLinkUrl() != "/account/orders/ord-2" {
		t.Errorf("buyer-2 link = %q", n.GetLinkUrl())
	}
	if got := len(notificationsOf(t, pool, "seller-1")); got != 0 {
		t.Errorf("seller notified of shipment: %d", got)
	}
}

func TestOrderPaidEvent_IsIgnoredWithoutSeller(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	e := paid()
	e.SellerId = ""
	mustDeliver(t, h, envelopeOf(t, "e1", orderPaidType, e))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestOrderPlacedCODNotifiesSeller(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderPlacedType, placed(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)))

	n := one(t, pool, "S1")
	assertSellerNew(t, n)
	if !strings.Contains(n.GetBody(), "250000") {
		t.Errorf("body %q should contain the amount 250000", n.GetBody())
	}
}

func TestOrderPaidNotifiesSeller(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderPaidType, paid()))

	assertSellerNew(t, one(t, pool, "S1"))
}

func TestOrderPlacedOtherSellerNotNotified(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderPlacedType, placed(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)))

	one(t, pool, "S1")
	if len(notificationsOf(t, pool, "S2")) != 0 || len(notificationsOf(t, pool, "B1")) != 0 {
		t.Fatalf("S2 and the buyer must have none, total %d", totalNotifications(t, pool))
	}
}

func TestOrderPlacedOnlineCreatesNothing(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderPlacedType, placed(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestOrderPlacedMalformedIsPermanent(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	requirePermanent(t, deliver(h, envelope(t, "e1", orderPlacedType, []byte{0xff, 0xff, 0xff})))
	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestOrderPaidRespectsTypePreference(t *testing.T) {
	pool := newDB(t)
	disable(t, pool, "S1", notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderPaidType, paid()))

	if got := len(notificationsOf(t, pool, "S1")); got != 0 {
		t.Fatalf("disabled seller must receive none, got %d", got)
	}
}

func TestPaymentReminderNotifiesBuyer(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderReminderType, reminder()))

	n := one(t, pool, "B1")
	if n.GetTitle() != "Đơn hàng sắp hết hạn thanh toán" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/checkout/pay/O1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if !strings.Contains(n.GetBody(), "O1") || !strings.Contains(n.GetBody(), "250000") {
		t.Errorf("body %q should name order id and amount", n.GetBody())
	}
	if got := totalNotifications(t, pool); got != 1 {
		t.Errorf("only the buyer may be notified, total %d", got)
	}
}

func TestPaymentReminderMissingBuyerSkipped(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	e := reminder()
	e.BuyerId = ""
	mustDeliver(t, h, envelopeOf(t, "e1", orderReminderType, e))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestCancelExpiredNotifiesBuyer(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	e := &orderv1.OrderCancelled{OrderId: "O1", BuyerId: "B1", SellerId: "S1", ReasonCode: orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED}
	mustDeliver(t, h, envelopeOf(t, "e1", orderCancelType, e))

	n := one(t, pool, "B1")
	if n.GetTitle() != "Đơn hàng đã bị hủy do quá hạn thanh toán" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/account/orders/O1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
}

func TestCancelOtherReasonCreatesNothing(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderCancelType, &orderv1.OrderCancelled{OrderId: "O1", BuyerId: "B1", SellerId: "S1", ReasonCode: orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND}))
	mustDeliver(t, h, envelopeOf(t, "e2", orderCancelType, &orderv1.OrderCancelled{OrderId: "O2", BuyerId: "B1", SellerId: "S1"}))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestOrderCompletedNotifiesBuyer(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderDoneType, completed("O1", orderv1.CompletedBy_COMPLETED_BY_SYSTEM)))

	n := one(t, pool, "B1")
	if n.GetTitle() != "Đơn hàng đã hoàn tất" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/account/orders/O1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if b := n.GetBody(); !strings.Contains(b, "2026-11-05") && !strings.Contains(b, "05/11/2026") {
		t.Errorf("body %q should contain the return window end date", b)
	}
}

func TestOrderCompletedAnyActorNotifiesBuyerOnly(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelopeOf(t, "e1", orderDoneType, completed("O1", orderv1.CompletedBy_COMPLETED_BY_BUYER)))
	mustDeliver(t, h, envelopeOf(t, "e2", orderDoneType, completed("O2", orderv1.CompletedBy_COMPLETED_BY_SELLER)))

	if got := len(notificationsOf(t, pool, "B1")); got != 2 {
		t.Fatalf("buyer want 2, got %d", got)
	}
	if got := len(notificationsOf(t, pool, "S1")); got != 0 {
		t.Fatalf("seller must have none, got %d", got)
	}
}

func TestReturnRequestedNotifiesSeller(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	e := &orderv1.ReturnRequested{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1", RefundAmount: 120000}
	mustDeliver(t, h, envelopeOf(t, "e1", returnReqType, e))

	n := one(t, pool, "S1")
	if n.GetTitle() != "Yêu cầu trả hàng mới" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/seller/orders/O1?tab=returns" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if got := totalNotifications(t, pool); got != 1 {
		t.Errorf("total = %d, want 1", got)
	}
}

func TestReturnRefundedNotifiesBuyer(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	e := &orderv1.ReturnRefunded{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1", RefundAmount: 120000}
	mustDeliver(t, h, envelopeOf(t, "e1", returnRefType, e))

	n := one(t, pool, "B1")
	if n.GetTitle() != "Yêu cầu trả hàng đã được hoàn tiền" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/account/orders/O1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if !strings.Contains(n.GetBody(), "120000") {
		t.Errorf("body %q should contain 120000", n.GetBody())
	}
}

func TestOrderEventsIgnoreOtherTypesAndPoison(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	mustDeliver(t, h, envelope(t, "e", "platform.payment.v1.PaymentSettled", []byte("x")))
	requirePermanent(t, deliver(h, []byte("not a proto \xff\xff")))
	requirePermanent(t, deliver(h, envelope(t, "", orderPaidType, nil)))
	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestOrderPaidRedeliveryNotifiesOnce(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	raw := envelopeOf(t, "e-dup", orderPaidType, paid())
	mustDeliver(t, h, raw)
	mustDeliver(t, h, raw)

	one(t, pool, "S1")
}

func TestReturnRequestedRedeliveryNotifiesOnce(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)

	raw := envelopeOf(t, "e-dup", returnReqType, &orderv1.ReturnRequested{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1"})
	mustDeliver(t, h, raw)
	mustDeliver(t, h, raw)

	one(t, pool, "S1")
}

func TestOrderCompletedTransientFailureRetries(t *testing.T) {
	pool := newDB(t)
	h := events.NewOrderHandler(pool, nil)
	raw := envelopeOf(t, "e-retry", orderDoneType, completed("O1", orderv1.CompletedBy_COMPLETED_BY_SYSTEM))

	failInserts(t, pool)
	requireTransient(t, deliver(h, raw))
	if got := ledgerRows(t, pool, "e-retry"); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}

	healInserts(t, pool)
	mustDeliver(t, h, raw)
	one(t, pool, "B1")
}
