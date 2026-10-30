package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

// dbErrRepo fails every read with a driver-looking error.
type dbErrRepo struct{ mockOrderServiceRepo }

const driverText = "pq: connection to 10.0.0.9 refused"

func (*dbErrRepo) GetOrder(context.Context, string) (store.Order, error) {
	return store.Order{}, errors.New(driverText)
}

func (*dbErrRepo) ListBuyerOrders(context.Context, string, int32) ([]store.Order, error) {
	return nil, errors.New(driverText)
}

// Scenario: An unexpected error does not leak its cause
func TestHandlerInternalErrorsAreGenericAndDoNotLeakCause(t *testing.T) {
	svc := newSvc(t, &dbErrRepo{}, nil, nil, nil, nil, nil, nil)
	h := newHandler(t, svc, nil, nil)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")

	_, err := h.GetOrder(ctx, &orderv1.GetOrderRequest{Id: "o1"})
	assertGenericInternal(t, err, "internal error")

	_, err = h.ListBuyerOrders(ctx, &orderv1.ListBuyerOrdersRequest{})
	assertGenericInternal(t, err, "internal error")

	_, err = h.CancelOrder(ctx, &orderv1.CancelOrderRequest{Id: "o1"})
	assertGenericInternal(t, err, "internal error")
}

func assertGenericInternal(t *testing.T, err error, want string) {
	t.Helper()
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal (err=%v)", status.Code(err), err)
	}
	msg := status.Convert(err).Message()
	if msg != want {
		t.Fatalf("message = %q, want %q", msg, want)
	}
	if strings.Contains(msg, "pq:") || strings.Contains(msg, "10.0.0.9") {
		t.Fatalf("driver text leaked to client: %q", msg)
	}
}

func TestHandlerMappedErrorsUseStableMessages(t *testing.T) {
	repo := &mockOrderServiceRepo{orders: map[string]store.Order{
		"o1": {ID: "o1", BuyerID: "buyer_1", SellerID: "seller_1", Status: store.OrderStatusCancelled},
	}}
	svc := newSvc(t, repo, nil, nil, nil, nil, nil, nil)
	h := newHandler(t, svc, nil, nil)

	// Cancelling an already-cancelled order is a client error, not INTERNAL, and
	// the message does not echo internal status names.
	_, err := h.CancelOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CancelOrderRequest{Id: "o1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("cancel code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	if got := status.Convert(err).Message(); got != "order cannot be cancelled in its current status" {
		t.Fatalf("cancel message = %q", got)
	}

	// Seller status update with a disallowed transition: stable message.
	_, err = h.UpdateOrderStatus(incomingPrincipalCtx("seller_1", "seller"), &orderv1.UpdateOrderStatusRequest{
		Id: "o1", Status: orderv1.OrderStatus_ORDER_STATUS_SHIPPED,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("update code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	if got := status.Convert(err).Message(); got != "invalid order status transition" {
		t.Fatalf("update message = %q", got)
	}
}

type failingGetRepo struct {
	mockOrderServiceRepo
	err error
}

func (r *failingGetRepo) GetOrder(context.Context, string) (store.Order, error) {
	return store.Order{}, r.err
}

func getOrderErr(t *testing.T, cause error) error {
	t.Helper()
	h := newHandler(t, newSvc(t, &failingGetRepo{err: cause}, nil, nil, nil, nil, nil, nil), nil, nil)
	_, err := h.GetOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.GetOrderRequest{Id: "o1"})
	return err
}

// Scenario: A unique violation maps to AlreadyExists
func TestUniqueViolationMapsToAlreadyExists(t *testing.T) {
	err := getOrderErr(t, fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"}))
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists (err=%v)", status.Code(err), err)
	}
}

// Scenario: Context errors keep their gRPC codes
func TestContextErrorsKeepGRPCCodes(t *testing.T) {
	for _, tc := range []struct {
		cause error
		want  codes.Code
	}{
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{fmt.Errorf("get order: %w", context.Canceled), codes.Canceled},
		{fmt.Errorf("get order: %w", context.DeadlineExceeded), codes.DeadlineExceeded},
	} {
		if got := status.Code(getOrderErr(t, tc.cause)); got != tc.want {
			t.Fatalf("%v: code = %v, want %v", tc.cause, got, tc.want)
		}
	}
}

// Scenario: An anonymous principal is rejected
func TestAnonymousPrincipalUnauthenticated(t *testing.T) {
	carts := fake.NewInMemoryCartRepository()
	orders := fake.NewInMemoryOrderRepository()
	cart := rpc.NewCartHandler(newCart(t, carts, orders, upstreamtest.NewDomain(map[string]int32{}), nil), nil)
	h := newHandler(t, newSvc(t, orders, carts, nil, nil, nil, nil, nil), nil, nil)
	anon := withPrincipal(context.Background(), &commonv1.Principal{Id: "anonymous", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS})

	_, err := cart.GetCart(anon, &orderv1.GetCartRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetCart anonymous: want Unauthenticated, got %v", err)
	}
	_, err = h.ListBuyerOrders(context.Background(), &orderv1.ListBuyerOrdersRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListBuyerOrders without principal: want Unauthenticated, got %v", err)
	}
}

// Scenario: A buyer cannot read another buyer's order
func TestGetOrderStrangerPermissionDenied(t *testing.T) {
	repo := &mockOrderServiceRepo{orders: map[string]store.Order{
		"ord_1": {ID: "ord_1", BuyerID: "buyer_A", SellerID: "seller_S"},
	}}
	h := newHandler(t, newSvc(t, repo, nil, nil, nil, nil, nil, nil), nil, nil)
	_, err := h.GetOrder(incomingPrincipalCtx("buyer_B", "buyer"), &orderv1.GetOrderRequest{Id: "ord_1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
}

// Scenario: A nil flag evaluator is refused at construction
// Scenario: A nil audit gate is refused at construction
func TestNewOrderHandlerRefusesNilDependencies(t *testing.T) {
	logger := discardLogger()
	svc := newSvc(t, fake.NewInMemoryOrderRepository(), fake.NewInMemoryCartRepository(), nil, nil, nil, nil, nil)
	gate := auditx.NewGate(&fakeAudit{}, logger)
	for _, tc := range []struct {
		name string
		call func() (*rpc.OrderHandler, error)
		want string
	}{
		{"flag evaluator", func() (*rpc.OrderHandler, error) {
			return rpc.NewOrderHandler(svc, idleAddress{}, nil, gate, logger)
		}, "evaluator"},
		{"audit gate", func() (*rpc.OrderHandler, error) {
			return rpc.NewOrderHandler(svc, idleAddress{}, fakeFlags{enabled: true}, nil, logger)
		}, "gate"},
		{"address client", func() (*rpc.OrderHandler, error) {
			return rpc.NewOrderHandler(svc, nil, fakeFlags{enabled: true}, gate, logger)
		}, "address"},
		{"service", func() (*rpc.OrderHandler, error) {
			return rpc.NewOrderHandler(nil, idleAddress{}, fakeFlags{enabled: true}, gate, logger)
		}, "service"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := tc.call()
			if err == nil || h != nil {
				t.Fatalf("want a nil handler and an error naming the %s, got %v / %v", tc.name, h, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name %q", err, tc.want)
			}
		})
	}
}

// Scenario: An oversized key is rejected
func TestCreateOrderOversizedIdempotencyKeyInvalidArgument(t *testing.T) {
	h, domain, orders := checkoutHandler(t)
	_, err := h.CreateOrder(withKey(incomingPrincipalCtx("buyer_1", "buyer"), strings.Repeat("k", 256)), &orderv1.CreateOrderRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if domain.Calls.Reserve != 0 || ordersOf(t, orders, "buyer_1") != 0 {
		t.Fatalf("no reservation or order may exist: reserve calls %d", domain.Calls.Reserve)
	}
}

// Scenario: A padded key is trimmed and replays the first checkout
func TestCreateOrderPaddedIdempotencyKeyReplays(t *testing.T) {
	h, domain, orders := checkoutHandler(t)
	buyer := incomingPrincipalCtx("buyer_1", "buyer")
	first, err := h.CreateOrder(withKey(buyer, "k-1"), &orderv1.CreateOrderRequest{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.CreateOrder(withKey(buyer, "  k-1  "), &orderv1.CreateOrderRequest{})
	if err != nil {
		t.Fatalf("padded key must be trimmed and replay: %v", err)
	}
	if len(first.GetOrders()) != 1 || len(second.GetOrders()) != 1 || first.GetOrders()[0].GetId() != second.GetOrders()[0].GetId() {
		t.Fatalf("want the same order ids, got %v / %v", first.GetOrders(), second.GetOrders())
	}
	if n := ordersOf(t, orders, "buyer_1"); n != 1 {
		t.Fatalf("orders = %d, want 1", n)
	}
	if domain.Stock("lst_1") != 8 {
		t.Fatalf("stock %d, want 8 (one reservation)", domain.Stock("lst_1"))
	}
}
