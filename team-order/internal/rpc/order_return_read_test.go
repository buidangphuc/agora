package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
)

func newReturnReadEnv(t *testing.T) (rmaEnv, string) {
	t.Helper()
	e := newRMAEnv(t.Context(), t)
	o := e.order(t.Context(), t, true)
	return e, e.createReturn(t.Context(), t, o.ID, 200000)
}

func (e rmaEnv) getReturn(ctx context.Context, id string) (*orderv1.GetReturnRequestResponse, error) {
	return e.h.GetReturnRequest(ctx, &orderv1.GetReturnRequestRequest{Id: id})
}

func TestGetReturnRequestServicePrincipalOrderRead(t *testing.T) {
	e, id := newReturnReadEnv(t)
	resp, err := e.getReturn(serviceCtx("order.read"), id)
	if err != nil {
		t.Fatalf("service with order.read must read a return: %v", err)
	}
	if resp.GetReturnRequest().GetId() != id {
		t.Fatalf("want return %s, got %s", id, resp.GetReturnRequest().GetId())
	}
}

func TestGetReturnRequestServiceWithoutScopeDenied(t *testing.T) {
	e, id := newReturnReadEnv(t)
	_, err := e.getReturn(serviceCtx("payment.read"), id)
	wantCode(t, err, codes.PermissionDenied)
}

func TestGetReturnRequestUserWithOrderReadScopeDenied(t *testing.T) {
	e, id := newReturnReadEnv(t)
	_, err := e.getReturn(scopedUser("stranger_1", "order.read"), id)
	wantCode(t, err, codes.PermissionDenied)
}

func TestGetReturnRequestStrangerDenied(t *testing.T) {
	e, id := newReturnReadEnv(t)
	_, err := e.getReturn(incomingPrincipalCtx("stranger_1", "buyer"), id)
	wantCode(t, err, codes.PermissionDenied)
}

func TestGetReturnRequestPartiesAndAdminUnchanged(t *testing.T) {
	e, id := newReturnReadEnv(t)
	for name, ctx := range map[string]context.Context{
		"buyer":  incomingPrincipalCtx("buyer_1", "buyer"),
		"seller": incomingPrincipalCtx("seller_1", "seller"),
		"admin":  adminCtx(),
	} {
		if _, err := e.getReturn(ctx, id); err != nil {
			t.Fatalf("%s must keep access: %v", name, err)
		}
	}
}

func TestGetReturnRequestAnonymousDenied(t *testing.T) {
	e, id := newReturnReadEnv(t)
	_, err := e.getReturn(context.Background(), id)
	wantCode(t, err, codes.Unauthenticated)
}

func TestGetReturnRequestServiceUnknownNotFound(t *testing.T) {
	e, _ := newReturnReadEnv(t)
	_, err := e.getReturn(serviceCtx("order.read"), "ret_missing")
	wantCode(t, err, codes.NotFound)
}

func TestUpdateReturnStatusServicePrincipalDenied(t *testing.T) {
	e, id := newReturnReadEnv(t)
	err := e.setStatus(serviceCtx("order.read"), id, orderv1.ReturnStatus_RETURN_STATUS_APPROVED)
	wantCode(t, err, codes.PermissionDenied)
	resp, err := e.getReturn(adminCtx(), id)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetReturnRequest().GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_PENDING {
		t.Fatalf("return status must be unchanged, got %v", resp.GetReturnRequest().GetStatus())
	}
}
