package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
)

type CartHandler struct {
	orderv1.UnimplementedCartServiceServer

	svc    *order.CartService
	logger *slog.Logger
}

func NewCartHandler(svc *order.CartService, logger *slog.Logger) *CartHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CartHandler{svc: svc, logger: logger}
}

func (h *CartHandler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, grpcx.ErrorTable{})
}

func (h *CartHandler) GetCart(ctx context.Context, _ *orderv1.GetCartRequest) (*orderv1.GetCartResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	items, subtotal, err := h.svc.GetCart(ctx, principal.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &orderv1.GetCartResponse{Cart: toWireCart(principal.ID, items, subtotal)}, nil
}

func (h *CartHandler) AddToCart(ctx context.Context, req *orderv1.AddToCartRequest) (*orderv1.AddToCartResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	items, subtotal, err := h.svc.AddToCart(ctx, principal.ID, req.GetListingId(), req.GetVariantId(), req.GetQuantity())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &orderv1.AddToCartResponse{Cart: toWireCart(principal.ID, items, subtotal)}, nil
}

func (h *CartHandler) UpdateCartItem(ctx context.Context, req *orderv1.UpdateCartItemRequest) (*orderv1.UpdateCartItemResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "item_id is required")
	}
	items, subtotal, err := h.svc.UpdateCartItem(ctx, principal.ID, req.GetItemId(), req.GetQuantity())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &orderv1.UpdateCartItemResponse{Cart: toWireCart(principal.ID, items, subtotal)}, nil
}

func (h *CartHandler) RemoveFromCart(ctx context.Context, req *orderv1.RemoveFromCartRequest) (*orderv1.RemoveFromCartResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "item_id is required")
	}
	items, subtotal, err := h.svc.RemoveFromCart(ctx, principal.ID, req.GetItemId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &orderv1.RemoveFromCartResponse{Cart: toWireCart(principal.ID, items, subtotal)}, nil
}

func (h *CartHandler) ClearCart(ctx context.Context, _ *orderv1.ClearCartRequest) (*orderv1.ClearCartResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.ClearCart(ctx, principal.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &orderv1.ClearCartResponse{}, nil
}

func (h *CartHandler) Reorder(ctx context.Context, req *orderv1.ReorderRequest) (*orderv1.ReorderResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	items, subtotal, err := h.svc.Reorder(ctx, principal.ID, req.GetOrderId())
	if err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		if errors.Is(err, order.ErrReorderNotOwner) {
			return nil, status.Error(codes.PermissionDenied, "cannot reorder another user's order")
		}
		return nil, h.fail(ctx, err)
	}
	return &orderv1.ReorderResponse{Cart: toWireCart(principal.ID, items, subtotal)}, nil
}

func toWireCart(userID string, items []store.CartItem, subtotal int64) *orderv1.Cart {
	wireItems := make([]*orderv1.CartItem, 0, len(items))
	for _, it := range items {
		wireItems = append(wireItems, &orderv1.CartItem{
			Id:          it.ID,
			ListingId:   it.ListingID,
			VariantId:   it.VariantID,
			Quantity:    it.Quantity,
			UnitPrice:   it.UnitPrice,
			Title:       it.Title,
			VariantName: it.VariantName,
			ImageUrl:    it.ImageURL,
			SellerId:    it.SellerID,
		})
	}
	return &orderv1.Cart{
		UserId:   userID,
		Items:    wireItems,
		Subtotal: subtotal,
	}
}
