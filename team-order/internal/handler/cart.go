package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/interceptor"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

type CartHandler struct {
	orderv1.UnimplementedCartServiceServer

	svc    *service.CartService
	logger *slog.Logger
}

func NewCartHandler(svc *service.CartService, logger *slog.Logger) *CartHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CartHandler{svc: svc, logger: logger}
}

func (h *CartHandler) GetCart(ctx context.Context, _ *orderv1.GetCartRequest) (*orderv1.GetCartResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	items, subtotal, err := h.svc.GetCart(ctx, principal.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cart: %v", err)
	}
	return &orderv1.GetCartResponse{Cart: toWireCart(principal.GetId(), items, subtotal)}, nil
}

func (h *CartHandler) AddToCart(ctx context.Context, req *orderv1.AddToCartRequest) (*orderv1.AddToCartResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	items, subtotal, err := h.svc.AddToCart(ctx, principal.GetId(), req.GetListingId(), req.GetVariantId(), req.GetQuantity())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "add to cart: %v", err)
	}
	return &orderv1.AddToCartResponse{Cart: toWireCart(principal.GetId(), items, subtotal)}, nil
}

func (h *CartHandler) UpdateCartItem(ctx context.Context, req *orderv1.UpdateCartItemRequest) (*orderv1.UpdateCartItemResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "item_id is required")
	}
	items, subtotal, err := h.svc.UpdateCartItem(ctx, principal.GetId(), req.GetItemId(), req.GetQuantity())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update cart item: %v", err)
	}
	return &orderv1.UpdateCartItemResponse{Cart: toWireCart(principal.GetId(), items, subtotal)}, nil
}

func (h *CartHandler) RemoveFromCart(ctx context.Context, req *orderv1.RemoveFromCartRequest) (*orderv1.RemoveFromCartResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "item_id is required")
	}
	items, subtotal, err := h.svc.RemoveFromCart(ctx, principal.GetId(), req.GetItemId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "remove from cart: %v", err)
	}
	return &orderv1.RemoveFromCartResponse{Cart: toWireCart(principal.GetId(), items, subtotal)}, nil
}

func (h *CartHandler) ClearCart(ctx context.Context, _ *orderv1.ClearCartRequest) (*orderv1.ClearCartResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.ClearCart(ctx, principal.GetId()); err != nil {
		return nil, status.Errorf(codes.Internal, "clear cart: %v", err)
	}
	return &orderv1.ClearCartResponse{}, nil
}

func (h *CartHandler) Reorder(ctx context.Context, req *orderv1.ReorderRequest) (*orderv1.ReorderResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	items, subtotal, err := h.svc.Reorder(ctx, principal.GetId(), req.GetOrderId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		if errors.Is(err, service.ErrReorderNotOwner) {
			return nil, status.Error(codes.PermissionDenied, "cannot reorder another user's order")
		}
		return nil, status.Errorf(codes.Internal, "reorder: %v", err)
	}
	return &orderv1.ReorderResponse{Cart: toWireCart(principal.GetId(), items, subtotal)}, nil
}

func toWireCart(userID string, items []repository.CartItem, subtotal int64) *orderv1.Cart {
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
