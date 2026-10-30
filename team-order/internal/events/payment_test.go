package events_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

type committer struct {
	promotionv1.FlashSaleServiceClient
	mu        sync.Mutex
	attempts  int
	failures  int
	failID    string
	failErr   error
	committed []string
	flash     []string
}

func (c *committer) CommitReservation(_ context.Context, in *promotionv1.CommitReservationRequest, _ ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attempts++
	if c.failures > 0 {
		c.failures--
		return nil, errors.New("promotion unavailable")
	}
	if c.failID != "" && in.GetReservationId() == c.failID {
		return nil, c.failErr
	}
	c.committed = append(c.committed, in.GetReservationId())
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

func (c *committer) GetActiveFlashSale(context.Context, *promotionv1.GetActiveFlashSaleRequest, ...grpc.CallOption) (*promotionv1.GetActiveFlashSaleResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flash = append(c.flash, "get")
	return &promotionv1.GetActiveFlashSaleResponse{}, nil
}

func (c *committer) ReserveFlashSaleStock(context.Context, *promotionv1.ReserveFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flash = append(c.flash, "reserve")
	return &promotionv1.ReserveFlashSaleStockResponse{Reserved: true}, nil
}

func (c *committer) CommitFlashSaleStock(context.Context, *promotionv1.CommitFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.CommitFlashSaleStockResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flash = append(c.flash, "commit")
	return &promotionv1.CommitFlashSaleStockResponse{Committed: true}, nil
}

func (c *committer) ReleaseFlashSaleStock(context.Context, *promotionv1.ReleaseFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flash = append(c.flash, "release")
	return &promotionv1.ReleaseFlashSaleStockResponse{Released: true}, nil
}

func (r *rig) paymentHandler(opts ...events.PaymentHandlerOption) *events.PaymentHandler {
	return events.NewPaymentHandler(r.orders, r.pool, quietLogger(), opts...)
}

// Scenario: A settled payment moves the order to PAID once
func TestPaymentRedeliveryAppliesOnce(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{})
	h := r.paymentHandler()
	rec := paymentRecord(t, "evt-1", paidSettlement(o.ID))

	if err := h.Handle(ctx, rec); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if err := h.Handle(ctx, rec); err != nil {
		t.Fatalf("a redelivery must be acknowledged, got %v", err)
	}

	paid := r.order(o.ID)
	if paid.Status != store.OrderStatusPaid || paid.PaidAt == nil {
		t.Fatalf("order = %+v, want PAID with paid_at", paid)
	}
	if n := r.outboxRowsOfType(o.ID, events.OrderPaidEventType); n != 1 {
		t.Fatalf("OrderPaidEvent outbox rows = %d, want exactly 1", n)
	}
	if n := r.processedRows(paymentConsumerName, "evt-1"); n != 1 {
		t.Fatalf("processed_events rows for %s = %d, want exactly 1", paymentConsumerName, n)
	}
}

// Scenario: A settled payment moves the order to PAID once
func TestPaymentConsumer_IdempotentDoubleConsume(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{})
	h := r.paymentHandler()
	rec := paymentRecord(t, "evt_1", paidSettlement(o.ID))

	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, rec); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
	first := r.order(o.ID)
	if first.Status != store.OrderStatusPaid {
		t.Fatalf("status = %v, want PAID", first.Status)
	}
	if n := r.outboxRows(o.ID); n != 1 {
		t.Fatalf("outbox rows = %d, want 1 (one PAID transition)", n)
	}
}

// Scenario: A settled payment moves the order to PAID once
func TestPaymentConsumer_AlreadyPaidOrder_NoDoubleTransition(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{})
	r.pay(o.ID)
	before := r.order(o.ID)
	h := r.paymentHandler()

	if err := h.Handle(ctx, paymentRecord(t, "evt_late", paidSettlement(o.ID))); err != nil {
		t.Fatalf("consume: %v", err)
	}
	after := r.order(o.ID)
	if !after.UpdatedAt.Equal(before.UpdatedAt) || after.PaidAt == nil || !after.PaidAt.Equal(*before.PaidAt) {
		t.Fatalf("an already-PAID order was changed: %+v -> %+v", before, after)
	}
	if n := r.outboxRows(o.ID); n != 1 {
		t.Fatalf("outbox rows = %d, want the single original PAID fact", n)
	}
}

// Scenario: A settled payment moves the order to PAID once
func TestPaymentConsumer_FailedPayment_NoTransition(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{})
	h := r.paymentHandler()

	rec := paymentRecord(t, "evt_fail", &paymentv1.PaymentSettled{OrderId: o.ID, Status: paymentv1.PaymentStatus_PAYMENT_STATUS_FAILED})
	if err := h.Handle(ctx, rec); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got := r.order(o.ID).Status; got != store.OrderStatusPending {
		t.Fatalf("status = %v, want PENDING for a FAILED payment", got)
	}
	if n := r.outboxRows(o.ID); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

// Scenario: A settled payment commits the order voucher once
func TestPaymentConsumer_CommitsVoucherOnSettle(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{VoucherCode: "SAVE30", DiscountAmount: 30000})
	promo := &committer{}
	h := r.paymentHandler(events.WithVoucherCommitter(promo))
	rec := paymentRecord(t, "evt_v", paidSettlement(o.ID))

	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, rec); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
	if len(promo.committed) != 1 || promo.committed[0] != o.ID {
		t.Fatalf("voucher commits = %v, want exactly one keyed by the order id", promo.committed)
	}
	if got := r.order(o.ID).Status; got != store.OrderStatusPaid {
		t.Fatalf("status = %v, want PAID", got)
	}
}

// Scenario: A settled payment commits the order voucher once
func TestPaymentConsumer_NoVoucher_NoCommit(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{})
	promo := &committer{}
	h := r.paymentHandler(events.WithVoucherCommitter(promo))

	if err := h.Handle(ctx, paymentRecord(t, "evt_p", paidSettlement(o.ID))); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if promo.attempts != 0 {
		t.Fatalf("voucher commit attempts = %d for a voucher-less order, want 0", promo.attempts)
	}
}

// Scenario: Multishop checkout: First paid order commits the platform voucher
func TestSettleCommitsPlatformVoucherOnFirstPaid(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	a := r.createOrder(store.Order{CheckoutID: "chk_1", PlatformVoucherCode: "SAVE10", PlatformDiscountAmount: 5000})
	r.createOrder(store.Order{CheckoutID: "chk_1", PlatformVoucherCode: "SAVE10", PlatformDiscountAmount: 5000})
	promo := &committer{}
	h := r.paymentHandler(events.WithVoucherCommitter(promo))

	if err := h.Handle(ctx, paymentRecord(t, "evt_pa", paidSettlement(a.ID))); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if len(promo.committed) != 1 || promo.committed[0] != "platform:chk_1" {
		t.Fatalf("commits = %v, want exactly platform:chk_1", promo.committed)
	}
	if got := r.order(a.ID).Status; got != store.OrderStatusPaid {
		t.Fatalf("status = %v, want PAID", got)
	}
}

// Scenario: Multishop checkout: An order without a platform voucher makes no platform commit
func TestSettleNoPlatformCommitWithoutPlatformVoucher(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{CheckoutID: "chk_2", VoucherCode: "SHOP5", DiscountAmount: 5000})
	promo := &committer{}
	h := r.paymentHandler(events.WithVoucherCommitter(promo))

	if err := h.Handle(ctx, paymentRecord(t, "evt_s", paidSettlement(o.ID))); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if len(promo.committed) != 1 || promo.committed[0] != o.ID {
		t.Fatalf("commits = %v, want only the shop commit under the order id", promo.committed)
	}
}

// Scenario: Multishop checkout: A platform commit failure is retried
func TestSettlePlatformCommitErrorRetried(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{CheckoutID: "chk_3", PlatformVoucherCode: "SAVE10", PlatformDiscountAmount: 5000})
	promo := &committer{failID: "platform:chk_3", failErr: errors.New("transport down")}
	h := r.paymentHandler(events.WithVoucherCommitter(promo))

	if err := h.Handle(ctx, paymentRecord(t, "evt_f", paidSettlement(o.ID))); err == nil {
		t.Fatal("the platform commit failure must be returned so the event is retried")
	}
	if n := r.processedRows(paymentConsumerName, "evt_f"); n != 0 {
		t.Fatalf("processed_events rows = %d, want 0 until the commit succeeds", n)
	}
}

// Scenario: Order expiry: A payment settling after the cancel changes nothing
func TestPaymentConsumer_LateSuccessAfterCancelIsIgnored(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{VoucherCode: "ONE"})
	if _, err := r.orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCancelled, cancelFrom, ""); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	promo := &committer{}
	h := r.paymentHandler(events.WithVoucherCommitter(promo))

	if err := h.Handle(ctx, paymentRecord(t, "evt_late", paidSettlement(o.ID))); err != nil {
		t.Fatalf("a late payment must be acknowledged, got %v", err)
	}
	got := r.order(o.ID)
	if got.Status != store.OrderStatusCancelled || got.PaidAt != nil {
		t.Fatalf("order must stay Cancelled and unpaid: %+v", got)
	}
	if promo.attempts != 0 {
		t.Fatalf("a cancelled order's voucher must never be committed (%d attempts)", promo.attempts)
	}
}

// Scenario: A settled payment moves the order to PAID once
func TestPaymentConsumer_VoucherCommitErrorRedeliversThenCommitsOnce(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{VoucherCode: "SAVE"})
	promo := &committer{failures: 1}
	h := r.paymentHandler(events.WithVoucherCommitter(promo))
	rec := paymentRecord(t, "evt_v", paidSettlement(o.ID))

	if err := h.Handle(ctx, rec); err == nil {
		t.Fatal("a voucher commit error must be returned so the event redelivers")
	}
	paid := r.order(o.ID)
	if paid.Status != store.OrderStatusPaid {
		t.Fatalf("the order is PAID after the first delivery, got %v", paid.Status)
	}
	if n := r.processedRows(paymentConsumerName, "evt_v"); n != 0 {
		t.Fatalf("processed_events rows = %d after the failed commit, want 0", n)
	}
	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, rec); err != nil {
			t.Fatalf("redelivery %d: %v", i+1, err)
		}
	}
	if len(promo.committed) != 1 || promo.committed[0] != o.ID {
		t.Fatalf("voucher must be committed exactly once: %v", promo.committed)
	}
	if n := r.outboxRowsOfType(o.ID, events.OrderPaidEventType); n != 1 {
		t.Fatalf("OrderPaidEvent rows = %d, want exactly one PAID transition", n)
	}
	again := r.order(o.ID)
	if !again.UpdatedAt.Equal(paid.UpdatedAt) {
		t.Fatalf("a redelivery moved the order again: %v -> %v", paid.UpdatedAt, again.UpdatedAt)
	}
}

// Scenario: Order expiry: A payment racing a cancel ends cancelled
func TestPaymentConsumer_PaymentRacingCancelEndsCancelled(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	h := r.paymentHandler()
	for i := 0; i < 10; i++ {
		o := r.createOrder(store.Order{})
		rec := paymentRecord(t, fmt.Sprintf("evt-race-%d", i), paidSettlement(o.ID))

		var wg sync.WaitGroup
		start := make(chan struct{})
		var cancelErr, payErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, cancelErr = r.orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCancelled, cancelFrom, "")
		}()
		go func() {
			defer wg.Done()
			<-start
			payErr = h.Handle(ctx, rec)
		}()
		close(start)
		wg.Wait()

		if cancelErr != nil || payErr != nil {
			t.Fatalf("round %d: cancel=%v pay=%v", i, cancelErr, payErr)
		}
		if got := r.order(o.ID).Status; got != store.OrderStatusCancelled {
			t.Fatalf("round %d: order must end Cancelled, is %v", i, got)
		}
	}
}

// Scenario: Order expiry: A payment settling after the expiry changes nothing in team-order
func TestLateSettleAfterExpiryLeavesOrderCancelled(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	due := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	o := r.createOrder(store.Order{
		VoucherCode: "SAVE", TotalAmount: 3000,
		PaymentMethod: int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO), PaymentDueAt: &due,
	})
	if _, err := r.orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCancelled, cancelFrom, "",
		store.WithCancelReason(int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED))); err != nil {
		t.Fatalf("setup expiry: %v", err)
	}
	promo := &committer{}
	h := r.paymentHandler(events.WithVoucherCommitter(promo))
	rec := paymentRecord(t, "evt_late_expiry", paidSettlement(o.ID))

	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, rec); err != nil {
			t.Fatalf("delivery %d: a late payment must be acknowledged, got %v", i+1, err)
		}
	}
	got := r.order(o.ID)
	if got.Status != store.OrderStatusCancelled || got.PaidAt != nil ||
		got.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) {
		t.Fatalf("order must stay Cancelled/unpaid with its expiry reason: %+v", got)
	}
	if promo.attempts != 0 {
		t.Fatalf("an expired order's voucher must not be committed (%d attempts)", promo.attempts)
	}
	if n := r.outboxRowsOfType(o.ID, events.OrderPaidEventType); n != 0 {
		t.Fatalf("a late settle must not emit OrderPaid, found %d rows", n)
	}
}

// Scenario: Settling payment makes no flash call
func TestPaymentConsumerMakesNoFlashCall(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(store.Order{
		VoucherCode: "SAVE", DiscountAmount: 1000,
		Items: []store.OrderItem{{ListingID: "lst_1", Quantity: 2, UnitPrice: 350000}},
	})
	spy := &committer{}
	h := r.paymentHandler(events.WithVoucherCommitter(spy))

	if err := h.Handle(ctx, paymentRecord(t, "evt_f", paidSettlement(o.ID))); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got := r.order(o.ID).Status; got != store.OrderStatusPaid {
		t.Fatalf("status = %v, want PAID", got)
	}
	if len(spy.flash) != 0 {
		t.Errorf("consumer made flash calls: %v", spy.flash)
	}
}
