package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

type returnEnv struct {
	svc    *service.OrderService
	orders *repository.InMemoryOrderRepository
	outbox *repository.InMemoryOutboxRepository
}

func newReturnEnv() returnEnv {
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository()
	returns := repository.NewInMemoryReturnRepository(repository.WithReturnOutbox(events.BuildReturnRefundedOutboxRow),
		repository.WithInMemoryReturnOutbox(outbox, orders))
	return returnEnv{svc: service.NewOrderService(orders, nil, returns, nil, nil, nil, nil), orders: orders, outbox: outbox}
}

// paidOrder is an order paid online (Pending -> Paid records paid_at) and shipped.
func (e returnEnv) paidOrder(t *testing.T, total int64) repository.Order {
	t.Helper()
	ctx := context.Background()
	o, err := e.orders.CreateOrder(ctx, repository.Order{BuyerID: "buyer", SellerID: "seller", TotalAmount: total})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, []repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	if o, err = e.orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped, []repository.OrderStatus{repository.OrderStatusPaid}, "T"); err != nil {
		t.Fatal(err)
	}
	return o
}

// codOrder is a cash-on-delivery order handed over from Pending: no paid_at.
func (e returnEnv) codOrder(t *testing.T, total int64) repository.Order {
	t.Helper()
	ctx := context.Background()
	o, err := e.orders.CreateOrder(ctx, repository.Order{BuyerID: "buyer", SellerID: "seller", TotalAmount: total})
	if err != nil {
		t.Fatal(err)
	}
	if o, err = e.orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped, []repository.OrderStatus{repository.OrderStatusPending}, "T"); err != nil {
		t.Fatal(err)
	}
	if o.PaidAt != nil {
		t.Fatal("a COD hand-over must not record paid_at")
	}
	return o
}

func (e returnEnv) refundRows(returnID string) int {
	n := 0
	for _, r := range e.outbox.EnqueuedRows() {
		if r.EventID == events.ReturnRefundedEventID(returnID) {
			n++
		}
	}
	return n
}

func TestCreateReturnRequest_Cap(t *testing.T) {
	ctx := context.Background()
	e := newReturnEnv()
	o := e.paidOrder(t, 500000)

	if _, err := e.svc.CreateReturnRequest(ctx, "buyer", o.ID, "r", 300000); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateReturnRequest(ctx, "buyer", o.ID, "r", 300000); !errors.Is(err, service.ErrInvalidRefundAmount) {
		t.Fatalf("over the remainder: want ErrInvalidRefundAmount, got %v", err)
	}
	rest, err := e.svc.CreateReturnRequest(ctx, "buyer", o.ID, "r", 0)
	if err != nil || rest.RefundAmount != 200000 {
		t.Fatalf("no amount defaults to the remainder: %+v %v", rest, err)
	}
	if _, err := e.svc.CreateReturnRequest(ctx, "buyer", o.ID, "r", -5); !errors.Is(err, service.ErrNothingToReturn) {
		t.Fatalf("remainder 0 without amount: want ErrNothingToReturn, got %v", err)
	}
	if _, err := e.svc.CreateReturnRequest(ctx, "buyer", o.ID, "r", 1); !errors.Is(err, service.ErrInvalidRefundAmount) {
		t.Fatalf("remainder 0 with amount: want ErrInvalidRefundAmount, got %v", err)
	}
	// A rejected return frees its amount.
	if _, err := e.svc.UpdateReturnStatus(ctx, rest.ID, repository.ReturnStatusRejected); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateReturnRequest(ctx, "buyer", o.ID, "r", 200000); err != nil {
		t.Fatalf("freed amount: %v", err)
	}
}

func TestUpdateReturnStatus_RefundWritesOneFact(t *testing.T) {
	ctx := context.Background()
	e := newReturnEnv()
	o := e.paidOrder(t, 500000)
	r, err := e.svc.CreateReturnRequest(ctx, "buyer", o.ID, "r", 200000)
	if err != nil {
		t.Fatal(err)
	}
	// PENDING -> REFUNDED is refused and writes nothing.
	if _, err := e.svc.UpdateReturnStatus(ctx, r.ID, repository.ReturnStatusRefunded); !errors.Is(err, service.ErrInvalidReturnStatus) {
		t.Fatalf("want ErrInvalidReturnStatus, got %v", err)
	}
	if got, _ := e.svc.GetReturnRequest(ctx, r.ID); got.Status != repository.ReturnStatusPending {
		t.Fatalf("status %v", got.Status)
	}
	if _, err := e.svc.UpdateReturnStatus(ctx, r.ID, repository.ReturnStatusApproved); err != nil {
		t.Fatal(err)
	}
	if n := e.refundRows(r.ID); n != 0 {
		t.Fatalf("approve wrote %d rows", n)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, refused := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.UpdateReturnStatus(ctx, r.ID, repository.ReturnStatusRefunded)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else if errors.Is(err, service.ErrInvalidReturnStatus) {
				refused++
			} else {
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins != 1 || refused != 7 {
		t.Fatalf("wins=%d refused=%d", wins, refused)
	}
	if n := e.refundRows(r.ID); n != 1 {
		t.Fatalf("want 1 ReturnRefunded row, got %d", n)
	}
}

// A return on an order never paid online cannot be refunded in the system: the
// call is ErrNotPaidOnline, the return stays APPROVED and nothing is written,
// while approve and reject still work.
func TestUpdateReturnStatus_CODRefundRefused(t *testing.T) {
	ctx := context.Background()
	e := newReturnEnv()
	o := e.codOrder(t, 500000)
	r, err := e.svc.CreateReturnRequest(ctx, "buyer", o.ID, "r", 200000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateReturnStatus(ctx, r.ID, repository.ReturnStatusApproved); err != nil {
		t.Fatalf("approve on COD: %v", err)
	}
	_, err = e.svc.UpdateReturnStatus(ctx, r.ID, repository.ReturnStatusRefunded)
	if !errors.Is(err, service.ErrNotPaidOnline) {
		t.Fatalf("want ErrNotPaidOnline, got %v", err)
	}
	if err.Error() != "order was not paid online; cash-on-delivery refunds are handled outside the system" {
		t.Fatalf("message %q", err.Error())
	}
	if got, _ := e.svc.GetReturnRequest(ctx, r.ID); got.Status != repository.ReturnStatusApproved {
		t.Fatalf("status must stay APPROVED: %v", got.Status)
	}
	if n := len(e.outbox.EnqueuedRows()); n != 0 {
		t.Fatalf("COD refund wrote %d outbox rows", n)
	}
	if _, err := e.svc.UpdateReturnStatus(ctx, r.ID, repository.ReturnStatusRejected); err != nil {
		t.Fatalf("reject on COD: %v", err)
	}
}
