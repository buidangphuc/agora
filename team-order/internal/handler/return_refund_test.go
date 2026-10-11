package handler_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/interceptor"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

type rmaEnv struct {
	h      *handler.OrderHandler
	orders *repository.InMemoryOrderRepository
	outbox *repository.InMemoryOutboxRepository
}

func newRMAEnv() rmaEnv {
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository()
	returns := repository.NewInMemoryReturnRepository(repository.WithReturnOutbox(events.BuildReturnRefundedOutboxRow),
		repository.WithInMemoryReturnOutbox(outbox, orders))
	svc := service.NewOrderService(orders, nil, returns, nil, nil, nil, nil)
	return rmaEnv{h: handler.NewOrderHandler(svc, nil, nil), orders: orders, outbox: outbox}
}

func (e rmaEnv) order(t *testing.T, paidOnline bool) repository.Order {
	t.Helper()
	ctx := context.Background()
	o, err := e.orders.CreateOrder(ctx, repository.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 500000})
	if err != nil {
		t.Fatal(err)
	}
	from := repository.OrderStatusPending
	if paidOnline {
		if _, err := e.orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, []repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
			t.Fatal(err)
		}
		from = repository.OrderStatusPaid
	}
	if o, err = e.orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped, []repository.OrderStatus{from}, "T"); err != nil {
		t.Fatal(err)
	}
	return o
}

func (e rmaEnv) createReturn(t *testing.T, orderID string, amount int64) string {
	t.Helper()
	resp, err := e.h.CreateReturnRequest(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateReturnRequestRequest{
		OrderId: orderID, Reason: "broken", RefundAmount: amount})
	if err != nil {
		t.Fatal(err)
	}
	return resp.GetReturnRequest().GetId()
}

func (e rmaEnv) setStatus(ctx context.Context, id string, st orderv1.ReturnStatus) error {
	_, err := e.h.UpdateReturnStatus(ctx, &orderv1.UpdateReturnStatusRequest{Id: id, Status: st})
	return err
}

func (e rmaEnv) refundRows() int {
	n := 0
	for _, r := range e.outbox.EnqueuedRows() {
		if r.EventType == events.ReturnRefundedEventType {
			n++
		}
	}
	return n
}

func wantCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	if status.Code(err) != code {
		t.Fatalf("want %v, got %v", code, err)
	}
}

func adminCtx() context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{Id: "ops", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER, Scopes: []string{"admin", "order.admin"}})
}

func TestReturnRefund_Handler(t *testing.T) {
	seller := incomingPrincipalCtx("seller_1", "seller")
	buyer := incomingPrincipalCtx("buyer_1", "buyer")

	t.Run("the seller refunds an approved return and one fact is written", func(t *testing.T) {
		e := newRMAEnv()
		o := e.order(t, true)
		id := e.createReturn(t, o.ID, 200000)
		wantCode(t, e.setStatus(seller, id, orderv1.ReturnStatus_RETURN_STATUS_REFUNDED), codes.FailedPrecondition) // PENDING -> REFUNDED
		if e.refundRows() != 0 {
			t.Fatal("refused transition wrote a fact")
		}
		if err := e.setStatus(seller, id, orderv1.ReturnStatus_RETURN_STATUS_APPROVED); err != nil {
			t.Fatal(err)
		}
		if err := e.setStatus(seller, id, orderv1.ReturnStatus_RETURN_STATUS_REFUNDED); err != nil {
			t.Fatal(err)
		}
		if e.refundRows() != 1 {
			t.Fatalf("want 1 ReturnRefunded row, got %d", e.refundRows())
		}
		wantCode(t, e.setStatus(seller, id, orderv1.ReturnStatus_RETURN_STATUS_REFUNDED), codes.FailedPrecondition)
		if e.refundRows() != 1 {
			t.Fatal("a repeated refund wrote another fact")
		}
	})

	t.Run("the buyer cannot refund their own return", func(t *testing.T) {
		e := newRMAEnv()
		o := e.order(t, true)
		id := e.createReturn(t, o.ID, 200000)
		if err := e.setStatus(seller, id, orderv1.ReturnStatus_RETURN_STATUS_APPROVED); err != nil {
			t.Fatal(err)
		}
		wantCode(t, e.setStatus(buyer, id, orderv1.ReturnStatus_RETURN_STATUS_REFUNDED), codes.PermissionDenied)
		got, err := e.h.GetReturnRequest(buyer, &orderv1.GetReturnRequestRequest{Id: id})
		if err != nil || got.GetReturnRequest().GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_APPROVED {
			t.Fatalf("status: %v %v", got, err)
		}
		if e.refundRows() != 0 {
			t.Fatal("buyer refund wrote a fact")
		}
	})

	t.Run("a COD order's approved return is refused with the spec message", func(t *testing.T) {
		e := newRMAEnv()
		o := e.order(t, false)
		id := e.createReturn(t, o.ID, 200000)
		if err := e.setStatus(seller, id, orderv1.ReturnStatus_RETURN_STATUS_APPROVED); err != nil {
			t.Fatalf("approve on COD: %v", err)
		}
		err := e.setStatus(seller, id, orderv1.ReturnStatus_RETURN_STATUS_REFUNDED)
		wantCode(t, err, codes.FailedPrecondition)
		if msg := status.Convert(err).Message(); msg != "order was not paid online; cash-on-delivery refunds are handled outside the system" {
			t.Fatalf("message %q", msg)
		}
		got, _ := e.h.GetReturnRequest(seller, &orderv1.GetReturnRequestRequest{Id: id})
		if got.GetReturnRequest().GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_APPROVED {
			t.Fatalf("status %v", got.GetReturnRequest().GetStatus())
		}
		if n := len(e.outbox.EnqueuedRows()); n != 0 {
			t.Fatalf("COD refund wrote %d rows", n)
		}
		if err := e.setStatus(seller, id, orderv1.ReturnStatus_RETURN_STATUS_REJECTED); err != nil {
			t.Fatalf("reject on COD: %v", err)
		}
	})

	t.Run("the return cap codes", func(t *testing.T) {
		e := newRMAEnv()
		o := e.order(t, true)
		e.createReturn(t, o.ID, 300000)
		_, err := e.h.CreateReturnRequest(buyer, &orderv1.CreateReturnRequestRequest{OrderId: o.ID, Reason: "r", RefundAmount: 300000})
		wantCode(t, err, codes.InvalidArgument)
		e.createReturn(t, o.ID, 200000)
		_, err = e.h.CreateReturnRequest(buyer, &orderv1.CreateReturnRequestRequest{OrderId: o.ID, Reason: "r"})
		wantCode(t, err, codes.FailedPrecondition)
		_, err = e.h.CreateReturnRequest(buyer, &orderv1.CreateReturnRequestRequest{OrderId: o.ID, Reason: "r", RefundAmount: 100000})
		wantCode(t, err, codes.InvalidArgument)
	})
}

func TestListOrderReturns_Handler(t *testing.T) {
	e := newRMAEnv()
	o := e.order(t, true)
	first := e.createReturn(t, o.ID, 200000)
	time.Sleep(2 * time.Millisecond)
	second := e.createReturn(t, o.ID, 100000)

	for name, ctx := range map[string]context.Context{
		"buyer":  incomingPrincipalCtx("buyer_1", "buyer"),
		"seller": incomingPrincipalCtx("seller_1", "seller"),
		"admin":  adminCtx(),
	} {
		resp, err := e.h.ListOrderReturns(ctx, &orderv1.ListOrderReturnsRequest{OrderId: o.ID})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := resp.GetReturns()
		if len(got) != 2 || got[0].GetId() != second || got[1].GetId() != first {
			t.Fatalf("%s: newest first: %v", name, got)
		}
		if got[1].GetReason() != "broken" || got[1].GetRefundAmount() != 200000 || got[1].GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_PENDING {
			t.Fatalf("%s: fields %v", name, got[1])
		}
	}

	_, err := e.h.ListOrderReturns(incomingPrincipalCtx("other_buyer", "buyer"), &orderv1.ListOrderReturnsRequest{OrderId: o.ID})
	wantCode(t, err, codes.PermissionDenied)
	_, err = e.h.ListOrderReturns(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.ListOrderReturnsRequest{OrderId: "missing"})
	wantCode(t, err, codes.NotFound)
	_, err = e.h.ListOrderReturns(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.ListOrderReturnsRequest{})
	wantCode(t, err, codes.InvalidArgument)
	_, err = e.h.ListOrderReturns(context.Background(), &orderv1.ListOrderReturnsRequest{OrderId: o.ID})
	if err == nil {
		t.Fatal("no principal must be refused")
	}
}

func TestGetOrder_PaidAtOnTheWire(t *testing.T) {
	e := newRMAEnv()
	buyer := incomingPrincipalCtx("buyer_1", "buyer")
	paid := e.order(t, true)
	resp, err := e.h.GetOrder(buyer, &orderv1.GetOrderRequest{Id: paid.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetOrder().GetPaidAt() == nil || !resp.GetOrder().GetPaidAt().AsTime().Equal(*paid.PaidAt) {
		t.Fatalf("paid_at %v, want %v", resp.GetOrder().GetPaidAt(), paid.PaidAt)
	}
	cod := e.order(t, false)
	resp, err = e.h.GetOrder(buyer, &orderv1.GetOrderRequest{Id: cod.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetOrder().GetPaidAt() != nil {
		t.Fatalf("a COD order must leave paid_at unset: %v", resp.GetOrder().GetPaidAt())
	}
}
