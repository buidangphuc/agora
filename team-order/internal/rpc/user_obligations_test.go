package rpc_test

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/rpc"
)

type obligationsRig struct {
	t *testing.T
	p *pgOutbox
	h *rpc.OrderHandler
}

func newObligationsRig(t *testing.T) *obligationsRig {
	t.Helper()
	ctx := context.Background()
	p := newPGOutbox(ctx, t)
	svc := newSvc(t, p.orders(), nil, p.returns(), nil, nil, nil, nil)
	return &obligationsRig{t: t, p: p, h: newHandler(t, svc, nil, nil)}
}

func (r *obligationsRig) order(id, buyer, seller string, status int) {
	r.t.Helper()
	addr, _ := json.Marshal(map[string]any{"recipient_name": "Nguyễn Văn A"})
	if _, err := r.p.pool.Exec(context.Background(),
		`INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, shipping_address) VALUES ($1,$2,$3,$4,1000,$5::jsonb)`,
		id, buyer, seller, status, string(addr)); err != nil {
		r.t.Fatalf("seed order: %v", err)
	}
}

func (r *obligationsRig) ret(id, orderID, buyer, seller string, status int) {
	r.t.Helper()
	if _, err := r.p.pool.Exec(context.Background(),
		`INSERT INTO order_returns (id, order_id, buyer_id, seller_id, reason, refund_amount, status) VALUES ($1,$2,$3,$4,'x',1,$5)`,
		id, orderID, buyer, seller, status); err != nil {
		r.t.Fatalf("seed return: %v", err)
	}
}

func userCtx(id string) context.Context {
	return withPrincipal(context.Background(), &commonv1.Principal{
		Id: id, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER, Scopes: []string{"order.internal.read"},
	})
}

func TestGetUserObligationsCounts(t *testing.T) {
	// OBL-1
	r := newObligationsRig(t)
	r.order("o-1", "u-1", "s-1", 1)
	r.order("o-2", "u-1", "s-1", 1)
	r.order("o-3", "u-1", "s-1", 3)
	r.order("o-4", "b-1", "u-1", 2)
	r.ret("ret-1", "o-1", "u-1", "s-1", 1)
	res, err := r.h.GetUserObligations(serviceCtx("order.internal.read"), &orderv1.GetUserObligationsRequest{UserId: "u-1"})
	if err != nil {
		t.Fatalf("OBL-1 unexpected error: %v", err)
	}
	if res.GetOpenBuyerOrders() != 3 || res.GetOpenSellerOrders() != 1 || res.GetOpenReturns() != 1 {
		t.Fatalf("OBL-1 got %v, want buyer 3 seller 1 returns 1", res)
	}
}

func TestGetUserObligationsClosedAreNotCounted(t *testing.T) {
	// OBL-2
	r := newObligationsRig(t)
	r.order("o-1", "u-1", "s-1", 4)
	r.order("o-2", "u-1", "s-1", 5)
	r.ret("ret-1", "o-1", "u-1", "s-1", 3)
	r.ret("ret-2", "o-2", "u-1", "s-1", 4)
	res, err := r.h.GetUserObligations(serviceCtx("order.internal.read"), &orderv1.GetUserObligationsRequest{UserId: "u-1"})
	if err != nil {
		t.Fatalf("OBL-2 unexpected error: %v", err)
	}
	if res.GetOpenBuyerOrders() != 0 || res.GetOpenSellerOrders() != 0 || res.GetOpenReturns() != 0 {
		t.Fatalf("OBL-2 got %v, want all zero", res)
	}
}

func TestGetUserObligationsUnknownUserReadsZeros(t *testing.T) {
	// OBL-3
	r := newObligationsRig(t)
	res, err := r.h.GetUserObligations(serviceCtx("order.internal.read"), &orderv1.GetUserObligationsRequest{UserId: "nobody"})
	if err != nil {
		t.Fatalf("OBL-3 unexpected error: %v", err)
	}
	if res.GetOpenBuyerOrders() != 0 || res.GetOpenSellerOrders() != 0 || res.GetOpenReturns() != 0 {
		t.Fatalf("OBL-3 got %v, want all zero", res)
	}
}

func TestGetUserObligationsRejectsEmptyUserID(t *testing.T) {
	// OBL-6: the pool is closed, so a query would surface as a store error rather than INVALID_ARGUMENT
	r := newObligationsRig(t)
	r.p.pool.Close()
	res, err := r.h.GetUserObligations(serviceCtx("order.internal.read"), &orderv1.GetUserObligationsRequest{UserId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("OBL-6 code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}
	if res != nil && (res.GetOpenBuyerOrders() != 0 || res.GetOpenSellerOrders() != 0 || res.GetOpenReturns() != 0) {
		t.Fatalf("OBL-6 response carries counts: %v", res)
	}
}

func TestGetUserObligationsRefusesUserPrincipal(t *testing.T) {
	// OBL-7
	r := newObligationsRig(t)
	r.order("o-1", "u-1", "s-1", 1)
	res, err := r.h.GetUserObligations(userCtx("u-1"), &orderv1.GetUserObligationsRequest{UserId: "u-1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("OBL-7 code = %v, want PermissionDenied (err=%v)", status.Code(err), err)
	}
	if res != nil && res.GetOpenBuyerOrders() != 0 {
		t.Fatalf("OBL-7 response leaked counts: %v", res)
	}
}

func TestGetUserObligationsRefusesServiceWithoutScope(t *testing.T) {
	// OBL-8
	r := newObligationsRig(t)
	r.order("o-1", "u-1", "s-1", 1)
	res, err := r.h.GetUserObligations(serviceCtx("order.read"), &orderv1.GetUserObligationsRequest{UserId: "u-1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("OBL-8 code = %v, want PermissionDenied (err=%v)", status.Code(err), err)
	}
	if res != nil && res.GetOpenBuyerOrders() != 0 {
		t.Fatalf("OBL-8 response leaked counts: %v", res)
	}
}

func TestGetUserObligationsRefusesAnonymous(t *testing.T) {
	// OBL-9
	r := newObligationsRig(t)
	_, err := r.h.GetUserObligations(context.Background(), &orderv1.GetUserObligationsRequest{UserId: "u-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("OBL-9 code = %v, want Unauthenticated (err=%v)", status.Code(err), err)
	}
}

func TestGetUserObligationsStoreFailure(t *testing.T) {
	// OBL-11
	r := newObligationsRig(t)
	r.order("o-1", "u-1", "s-1", 1)
	r.p.pool.Close()
	res, err := r.h.GetUserObligations(serviceCtx("order.internal.read"), &orderv1.GetUserObligationsRequest{UserId: "u-1"})
	if err == nil || status.Code(err) == codes.OK {
		t.Fatalf("OBL-11 want a non-OK status, got err=%v", err)
	}
	if res != nil {
		t.Fatalf("OBL-11 response must carry no counts, got %v", res)
	}
}

func TestGetOrderAfterBuyerErasure(t *testing.T) {
	// ORD-9
	ctx := context.Background()
	r := newObligationsRig(t)
	r.order("o-1", "u-1", "s-1", 2)
	if err := r.p.orders().EraseUser(ctx, "u-1"); err != nil {
		t.Fatalf("EraseUser: %v", err)
	}
	seller := withPrincipal(ctx, &commonv1.Principal{Id: "s-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER})
	res, err := r.h.GetOrder(seller, &orderv1.GetOrderRequest{Id: "o-1"})
	if err != nil {
		t.Fatalf("ORD-9 seller GetOrder failed: %v", err)
	}
	if got := res.GetOrder().GetShippingAddress().GetRecipientName(); got != "Đã xóa" {
		t.Fatalf("ORD-9 recipient name = %q, want %q", got, "Đã xóa")
	}
	if got := res.GetOrder().GetTotalAmount(); got != 1000 {
		t.Fatalf("ORD-9 total = %d, want 1000", got)
	}
}
