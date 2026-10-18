package consumer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	orderv1 "github.com/buidangphuc/team-notification/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-notification/generated/platform/payment/v1"
)

const (
	orderPlacedType   = "platform.order.v1.OrderPlaced"
	orderPaidType     = "platform.order.v1.OrderPaidEvent"
	orderReminderType = "platform.order.v1.OrderPaymentReminder"
	orderCancelType   = "platform.order.v1.OrderCancelled"
	orderDoneType     = "platform.order.v1.OrderCompleted"
	returnReqType     = "platform.order.v1.ReturnRequested"
	returnRefType     = "platform.order.v1.ReturnRefunded"
)

func owEnv(t *testing.T, eventID, typ string, m proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return envelope(t, eventID, typ, payload)
}

func owHandle(t *testing.T, c *OrderConsumer, raw []byte) {
	t.Helper()
	if err := c.HandleRaw(context.Background(), raw); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
}

func owFor(f *fakeNotif, user string) []*notificationv1.Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*notificationv1.Notification
	for _, n := range f.created {
		if n.GetUserId() == user {
			out = append(out, n)
		}
	}
	return out
}

// owOne asserts the user has exactly one notification and returns it.
func owOne(t *testing.T, f *fakeNotif, user string) *notificationv1.Notification {
	t.Helper()
	got := owFor(f, user)
	if len(got) != 1 {
		t.Fatalf("user %q: want exactly 1 notification, got %d (total %d)", user, len(got), f.count())
	}
	return got[0]
}

func owSetup() (*OrderConsumer, *fakeNotif) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	return NewOrderConsumer(notif, prefs, nil), notif
}

func owPlaced(method paymentv1.PaymentMethod) *orderv1.OrderPlaced {
	return &orderv1.OrderPlaced{
		OrderId: "O1", BuyerId: "B1", SellerId: "S1", PaymentMethod: method,
		TotalAmount: 250000, Currency: "VND",
		ShipBy: timestamppb.New(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)),
	}
}

func owPaid() *orderv1.OrderPaidEvent {
	return &orderv1.OrderPaidEvent{
		OrderId: "O1", BuyerId: "B1", SellerId: "S1", TotalAmount: 250000, Currency: "VND",
		ShipBy: timestamppb.New(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)),
	}
}

func owAssertSellerNew(t *testing.T, n *notificationv1.Notification) {
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

// Scenario: Order work: A COD order notifies its seller at placement
func TestOrderPlacedCODNotifiesSeller(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderPlacedType, owPlaced(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)))
	n := owOne(t, notif, "S1")
	owAssertSellerNew(t, n)
	if !strings.Contains(n.GetBody(), "250000") {
		t.Errorf("body %q should contain the amount 250000", n.GetBody())
	}
}

// Scenario: Order work: An online order notifies its seller at payment
func TestOrderPaidNotifiesSeller(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderPaidType, owPaid()))
	owAssertSellerNew(t, owOne(t, notif, "S1"))
}

// Scenario: Order work: Another seller receives no order notification
func TestOrderPlacedOtherSellerNotNotified(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderPlacedType, owPlaced(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)))
	owOne(t, notif, "S1")
	if len(owFor(notif, "S2")) != 0 || len(owFor(notif, "B1")) != 0 {
		t.Fatalf("S2 and the buyer must have none, total %d", notif.count())
	}
}

// Scenario: Order work: An online order is not notified at placement
func TestOrderPlacedOnlineCreatesNothing(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderPlacedType, owPlaced(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)))
	if notif.count() != 0 {
		t.Fatalf("want 0 notifications, got %d", notif.count())
	}
}

// Scenario: Order work: A redelivered order event does not notify twice
func TestOrderPaidRedeliveryNotifiesOnce(t *testing.T) {
	c, notif := owSetup()
	raw := owEnv(t, "e-dup", orderPaidType, owPaid())
	owHandle(t, c, raw)
	owHandle(t, c, raw)
	owOne(t, notif, "S1")
}

// Scenario: Order work: A paid event without seller_id is skipped
func TestOrderPaidWithoutSellerSkipped(t *testing.T) {
	c, notif := owSetup()
	e := owPaid()
	e.SellerId = ""
	owHandle(t, c, owEnv(t, "e1", orderPaidType, e))
	if notif.count() != 0 {
		t.Fatalf("want 0 notifications, got %d", notif.count())
	}
}

// Scenario: Order work: A malformed order payload is a permanent error
func TestOrderPlacedMalformedIsPermanent(t *testing.T) {
	c, notif := owSetup()
	raw := envelope(t, "e1", orderPlacedType, []byte{0xff, 0xff, 0xff})
	err := c.HandleRaw(context.Background(), raw)
	if !errors.Is(err, ErrPermanent) {
		t.Fatalf("want ErrPermanent, got %v", err)
	}
	if notif.count() != 0 {
		t.Fatalf("want 0 notifications, got %d", notif.count())
	}
}

// Scenario: Order work: A seller disabling ORDER notifications receives none
func TestOrderPaidRespectsTypePreference(t *testing.T) {
	notif := &fakeNotif{}
	prefs, repo := newPrefs()
	disable(t, repo, "S1", notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER)
	c := NewOrderConsumer(notif, prefs, nil)
	owHandle(t, c, owEnv(t, "e1", orderPaidType, owPaid()))
	if len(owFor(notif, "S1")) != 0 {
		t.Fatalf("disabled seller must receive none")
	}
}

func owReminder() *orderv1.OrderPaymentReminder {
	return &orderv1.OrderPaymentReminder{
		OrderId: "O1", BuyerId: "B1", TotalAmount: 250000, Currency: "VND",
		PaymentDueAt: timestamppb.New(time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)),
	}
}

// Scenario: Order work: A payment reminder notifies the buyer with a pay link
func TestPaymentReminderNotifiesBuyer(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderReminderType, owReminder()))
	n := owOne(t, notif, "B1")
	if n.GetTitle() != "Đơn hàng sắp hết hạn thanh toán" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/checkout/pay/O1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER {
		t.Errorf("type = %v", n.GetType())
	}
	if !strings.Contains(n.GetBody(), "O1") || !strings.Contains(n.GetBody(), "250000") {
		t.Errorf("body %q should name order id and amount", n.GetBody())
	}
}

// Scenario: Order work: An expiry cancel notifies the buyer
func TestCancelExpiredNotifiesBuyer(t *testing.T) {
	c, notif := owSetup()
	e := &orderv1.OrderCancelled{OrderId: "O1", BuyerId: "B1", SellerId: "S1", ReasonCode: orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED}
	owHandle(t, c, owEnv(t, "e1", orderCancelType, e))
	n := owOne(t, notif, "B1")
	if n.GetTitle() != "Đơn hàng đã bị hủy do quá hạn thanh toán" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/account/orders/O1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
}

// Scenario: Order work: A buyer's own cancel creates no notification
func TestCancelOtherReasonCreatesNothing(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderCancelType, &orderv1.OrderCancelled{OrderId: "O1", BuyerId: "B1", SellerId: "S1", ReasonCode: orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND}))
	owHandle(t, c, owEnv(t, "e2", orderCancelType, &orderv1.OrderCancelled{OrderId: "O2", BuyerId: "B1", SellerId: "S1"}))
	if notif.count() != 0 {
		t.Fatalf("want 0 notifications, got %d", notif.count())
	}
}

// Scenario: Order work: A reminder without buyer_id is skipped
func TestPaymentReminderMissingBuyerSkipped(t *testing.T) {
	c, notif := owSetup()
	e := owReminder()
	e.BuyerId = ""
	owHandle(t, c, owEnv(t, "e1", orderReminderType, e))
	if notif.count() != 0 {
		t.Fatalf("want 0 notifications, got %d", notif.count())
	}
}

// Scenario: Order work: A reminder is created for the buyer only
func TestPaymentReminderOnlyRecipientIsBuyer(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderReminderType, owReminder()))
	owOne(t, notif, "B1")
	if notif.count() != 1 || len(owFor(notif, "S1")) != 0 {
		t.Fatalf("only the buyer may be notified, total %d", notif.count())
	}
}

func owCompleted(id string, by orderv1.CompletedBy) *orderv1.OrderCompleted {
	return &orderv1.OrderCompleted{
		OrderId: id, BuyerId: "B1", SellerId: "S1", CompletedBy: by,
		ReturnWindowEndsAt: timestamppb.New(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)),
	}
}

// Scenario: Order work: A completed order prompts the buyer to review
func TestOrderCompletedNotifiesBuyer(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderDoneType, owCompleted("O1", orderv1.CompletedBy_COMPLETED_BY_SYSTEM)))
	n := owOne(t, notif, "B1")
	if n.GetTitle() != "Đơn hàng đã hoàn tất" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/account/orders/O1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	b := n.GetBody()
	if !strings.Contains(b, "2026-11-05") && !strings.Contains(b, "05/11/2026") {
		t.Errorf("body %q should contain the return window end date (2026-11-05 or 05/11/2026)", b)
	}
}

// Scenario: Order work: Completion by the buyer or the seller notifies the buyer too
func TestOrderCompletedAnyActorNotifiesBuyerOnly(t *testing.T) {
	c, notif := owSetup()
	owHandle(t, c, owEnv(t, "e1", orderDoneType, owCompleted("O1", orderv1.CompletedBy_COMPLETED_BY_BUYER)))
	owHandle(t, c, owEnv(t, "e2", orderDoneType, owCompleted("O2", orderv1.CompletedBy_COMPLETED_BY_SELLER)))
	got := owFor(notif, "B1")
	if len(got) != 2 {
		t.Fatalf("buyer want 2, got %d", len(got))
	}
	if got[0].GetLinkUrl() == got[1].GetLinkUrl() {
		t.Errorf("one notification per order expected, links %q %q", got[0].GetLinkUrl(), got[1].GetLinkUrl())
	}
	if len(owFor(notif, "S1")) != 0 {
		t.Fatalf("seller must have none")
	}
}

// Scenario: Order work: A return request notifies the seller with the returns link
func TestReturnRequestedNotifiesSeller(t *testing.T) {
	c, notif := owSetup()
	e := &orderv1.ReturnRequested{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1", RefundAmount: 120000}
	owHandle(t, c, owEnv(t, "e1", returnReqType, e))
	n := owOne(t, notif, "S1")
	if n.GetTitle() != "Yêu cầu trả hàng mới" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetLinkUrl() != "/seller/orders/O1?tab=returns" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
}

// Scenario: Order work: A refunded return notifies the buyer
func TestReturnRefundedNotifiesBuyer(t *testing.T) {
	c, notif := owSetup()
	e := &orderv1.ReturnRefunded{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1", RefundAmount: 120000}
	owHandle(t, c, owEnv(t, "e1", returnRefType, e))
	n := owOne(t, notif, "B1")
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

// Scenario: Order work: A redelivered return request does not notify twice
func TestReturnRequestedRedeliveryNotifiesOnce(t *testing.T) {
	c, notif := owSetup()
	raw := owEnv(t, "e-dup", returnReqType, &orderv1.ReturnRequested{ReturnId: "R1", OrderId: "O1", BuyerId: "B1", SellerId: "S1"})
	owHandle(t, c, raw)
	owHandle(t, c, raw)
	owOne(t, notif, "S1")
}

// Scenario: Order work: A notification failure leaves the event unrecorded
func TestOrderCompletedTransientFailureRetries(t *testing.T) {
	notif := &fakeNotif{err: errors.New("db down")}
	prefs, _ := newPrefs()
	ledger := NewInMemoryDeduper()
	c := NewOrderConsumer(notif, prefs, nil, WithUserEventDeduper(ledger))

	raw := owEnv(t, "e-retry", orderDoneType, owCompleted("O1", orderv1.CompletedBy_COMPLETED_BY_SYSTEM))
	err := c.HandleRaw(context.Background(), raw)
	if err == nil || errors.Is(err, ErrPermanent) {
		t.Fatalf("want a non-permanent error, got %v", err)
	}
	if done, _ := ledger.IsProcessed(context.Background(), orderConsumerName, "e-retry"); done {
		t.Fatalf("a failed event must not be recorded in the ledger")
	}

	notif.mu.Lock()
	notif.err = nil
	notif.mu.Unlock()
	owHandle(t, c, raw)
	owOne(t, notif, "B1")
}
