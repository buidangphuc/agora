package edge

import (
	"context"

	"connectrpc.com/connect"

	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
)

type OrderForwarder struct {
	orderv1connect.UnimplementedOrderServiceHandler
	client orderv1.OrderServiceClient
	edge   *Edge
}

func NewOrderForwarder(client orderv1.OrderServiceClient, edge *Edge) *OrderForwarder {
	return &OrderForwarder{client: client, edge: edge}
}

func (f *OrderForwarder) CreateOrder(
	ctx context.Context,
	req *connect.Request[orderv1.CreateOrderRequest],
) (*connect.Response[orderv1.CreateOrderResponse], error) {
	var out *orderv1.CreateOrderResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateOrder(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) GetOrder(
	ctx context.Context,
	req *connect.Request[orderv1.GetOrderRequest],
) (*connect.Response[orderv1.GetOrderResponse], error) {
	var out *orderv1.GetOrderResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetOrder(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) ListBuyerOrders(
	ctx context.Context,
	req *connect.Request[orderv1.ListBuyerOrdersRequest],
) (*connect.Response[orderv1.ListBuyerOrdersResponse], error) {
	var out *orderv1.ListBuyerOrdersResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListBuyerOrders(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) ListSellerOrders(
	ctx context.Context,
	req *connect.Request[orderv1.ListSellerOrdersRequest],
) (*connect.Response[orderv1.ListSellerOrdersResponse], error) {
	var out *orderv1.ListSellerOrdersResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListSellerOrders(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) UpdateOrderStatus(
	ctx context.Context,
	req *connect.Request[orderv1.UpdateOrderStatusRequest],
) (*connect.Response[orderv1.UpdateOrderStatusResponse], error) {
	var out *orderv1.UpdateOrderStatusResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UpdateOrderStatus(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) CancelOrder(
	ctx context.Context,
	req *connect.Request[orderv1.CancelOrderRequest],
) (*connect.Response[orderv1.CancelOrderResponse], error) {
	var out *orderv1.CancelOrderResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CancelOrder(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) GetSagaState(
	ctx context.Context,
	req *connect.Request[orderv1.GetSagaStateRequest],
) (*connect.Response[orderv1.GetSagaStateResponse], error) {
	var out *orderv1.GetSagaStateResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetSagaState(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) ForceFailSaga(
	ctx context.Context,
	req *connect.Request[orderv1.ForceFailSagaRequest],
) (*connect.Response[orderv1.ForceFailSagaResponse], error) {
	var out *orderv1.ForceFailSagaResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ForceFailSaga(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── RMA Returns ──

func (f *OrderForwarder) CreateReturnRequest(
	ctx context.Context,
	req *connect.Request[orderv1.CreateReturnRequestRequest],
) (*connect.Response[orderv1.CreateReturnRequestResponse], error) {
	var out *orderv1.CreateReturnRequestResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateReturnRequest(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) GetReturnRequest(
	ctx context.Context,
	req *connect.Request[orderv1.GetReturnRequestRequest],
) (*connect.Response[orderv1.GetReturnRequestResponse], error) {
	var out *orderv1.GetReturnRequestResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetReturnRequest(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) UpdateReturnStatus(
	ctx context.Context,
	req *connect.Request[orderv1.UpdateReturnStatusRequest],
) (*connect.Response[orderv1.UpdateReturnStatusResponse], error) {
	var out *orderv1.UpdateReturnStatusResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UpdateReturnStatus(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Shipments & Fulfillment Tracking ──

func (f *OrderForwarder) CreateShipment(
	ctx context.Context,
	req *connect.Request[orderv1.CreateShipmentRequest],
) (*connect.Response[orderv1.CreateShipmentResponse], error) {
	var out *orderv1.CreateShipmentResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateShipment(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *OrderForwarder) GetShipmentTracking(
	ctx context.Context,
	req *connect.Request[orderv1.GetShipmentTrackingRequest],
) (*connect.Response[orderv1.GetShipmentTrackingResponse], error) {
	var out *orderv1.GetShipmentTrackingResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetShipmentTracking(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
