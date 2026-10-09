package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/interceptor"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

// authz-residuals-2: admin order operations need the order.admin scope.

func scopedUser(id string, scopes ...string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: id, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER, Scopes: scopes,
	})
}

func newAdminScopeHandler() (*handler.OrderHandler, *mockOrderServiceRepo) {
	repo := &mockOrderServiceRepo{orders: map[string]repository.Order{
		"ord_1": {ID: "ord_1", BuyerID: "buyer_1", SellerID: "seller_1", Status: repository.OrderStatusPending},
	}}
	return handler.NewOrderHandler(service.NewOrderService(repo, nil, nil, nil, nil, nil, nil), nil, nil), repo
}

func TestForceFailSaga_NeedsAdminAndOrderAdmin(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"admin and order.admin", scopedUser("admin_1", "admin", "order.admin"), codes.OK},
		{"admin without order.admin", scopedUser("admin_1", "admin"), codes.PermissionDenied},
		{"order.admin without admin", scopedUser("ops_1", "order.admin"), codes.PermissionDenied},
		{"owner", scopedUser("buyer_1", "order.read", "order.write"), codes.PermissionDenied},
		{"other buyer", scopedUser("buyer_2"), codes.PermissionDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, repo := newAdminScopeHandler()
			_, err := h.ForceFailSaga(tc.ctx, &orderv1.ForceFailSagaRequest{OrderId: "ord_1"})
			if status.Code(err) != tc.want {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if tc.want != codes.OK && repo.orders["ord_1"].Status != repository.OrderStatusPending {
				t.Fatalf("order must be unchanged, got %v", repo.orders["ord_1"].Status)
			}
		})
	}
}

func TestBareAdminDoesNotOpenOrders(t *testing.T) {
	bare := scopedUser("admin_1", "admin")
	full := scopedUser("admin_1", "admin", "order.admin")

	t.Run("GetSagaState bare admin denied", func(t *testing.T) {
		h, _ := newAdminScopeHandler()
		if _, err := h.GetSagaState(bare, &orderv1.GetSagaStateRequest{OrderId: "ord_1"}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %v", err)
		}
	})
	t.Run("GetSagaState order.admin allowed", func(t *testing.T) {
		h, _ := newAdminScopeHandler()
		if _, err := h.GetSagaState(full, &orderv1.GetSagaStateRequest{OrderId: "ord_1"}); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
	t.Run("GetOrder bare admin denied", func(t *testing.T) {
		h, _ := newAdminScopeHandler()
		if _, err := h.GetOrder(bare, &orderv1.GetOrderRequest{Id: "ord_1"}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %v", err)
		}
	})
	t.Run("UpdateOrderStatus bare admin denied", func(t *testing.T) {
		h, _ := newAdminScopeHandler()
		_, err := h.UpdateOrderStatus(bare, &orderv1.UpdateOrderStatusRequest{Id: "ord_1", Status: orderv1.OrderStatus_ORDER_STATUS_SHIPPED})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %v", err)
		}
	})
}
