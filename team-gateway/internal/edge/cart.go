package edge

import (
	"context"

	"connectrpc.com/connect"

	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
)

type CartForwarder struct {
	orderv1connect.UnimplementedCartServiceHandler
	client orderv1.CartServiceClient
	edge   *Edge
}

func NewCartForwarder(client orderv1.CartServiceClient, edge *Edge) *CartForwarder {
	return &CartForwarder{client: client, edge: edge}
}

func (f *CartForwarder) GetCart(
	ctx context.Context,
	req *connect.Request[orderv1.GetCartRequest],
) (*connect.Response[orderv1.GetCartResponse], error) {
	var out *orderv1.GetCartResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetCart(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *CartForwarder) AddToCart(
	ctx context.Context,
	req *connect.Request[orderv1.AddToCartRequest],
) (*connect.Response[orderv1.AddToCartResponse], error) {
	var out *orderv1.AddToCartResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.AddToCart(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *CartForwarder) UpdateCartItem(
	ctx context.Context,
	req *connect.Request[orderv1.UpdateCartItemRequest],
) (*connect.Response[orderv1.UpdateCartItemResponse], error) {
	var out *orderv1.UpdateCartItemResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UpdateCartItem(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *CartForwarder) RemoveFromCart(
	ctx context.Context,
	req *connect.Request[orderv1.RemoveFromCartRequest],
) (*connect.Response[orderv1.RemoveFromCartResponse], error) {
	var out *orderv1.RemoveFromCartResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RemoveFromCart(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *CartForwarder) ClearCart(
	ctx context.Context,
	req *connect.Request[orderv1.ClearCartRequest],
) (*connect.Response[orderv1.ClearCartResponse], error) {
	var out *orderv1.ClearCartResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ClearCart(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// Reorder re-adds a past order's items to the cart — a write.
func (f *CartForwarder) Reorder(
	ctx context.Context,
	req *connect.Request[orderv1.ReorderRequest],
) (*connect.Response[orderv1.ReorderResponse], error) {
	var out *orderv1.ReorderResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.Reorder(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
