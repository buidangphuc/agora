package edge

import (
	"context"

	"connectrpc.com/connect"

	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/promotion/v1/promotionv1connect"
)

// ── VoucherService ──

// VoucherForwarder serves its Connect handler RPCs by forwarding them to the upstream gRPC client.
type VoucherForwarder struct {
	promotionv1connect.UnimplementedVoucherServiceHandler
	client promotionv1.VoucherServiceClient
	edge   *Edge
}

// NewVoucherForwarder returns the forwarder that serves its RPCs from the given upstream client.
func NewVoucherForwarder(client promotionv1.VoucherServiceClient, edge *Edge) *VoucherForwarder {
	return &VoucherForwarder{client: client, edge: edge}
}

// CreateVoucher forwards the CreateVoucher RPC to the upstream service.
func (f *VoucherForwarder) CreateVoucher(
	ctx context.Context,
	req *connect.Request[promotionv1.CreateVoucherRequest],
) (*connect.Response[promotionv1.CreateVoucherResponse], error) {
	var out *promotionv1.CreateVoucherResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateVoucher(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// GetVoucher forwards the GetVoucher RPC to the upstream service.
func (f *VoucherForwarder) GetVoucher(
	ctx context.Context,
	req *connect.Request[promotionv1.GetVoucherRequest],
) (*connect.Response[promotionv1.GetVoucherResponse], error) {
	var out *promotionv1.GetVoucherResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetVoucher(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ListVouchers forwards the ListVouchers RPC to the upstream service.
func (f *VoucherForwarder) ListVouchers(
	ctx context.Context,
	req *connect.Request[promotionv1.ListVouchersRequest],
) (*connect.Response[promotionv1.ListVouchersResponse], error) {
	var out *promotionv1.ListVouchersResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListVouchers(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ValidateAndReserve forwards the ValidateAndReserve RPC to the upstream service.
func (f *VoucherForwarder) ValidateAndReserve(
	ctx context.Context,
	req *connect.Request[promotionv1.ValidateAndReserveRequest],
) (*connect.Response[promotionv1.ValidateAndReserveResponse], error) {
	var out *promotionv1.ValidateAndReserveResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ValidateAndReserve(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// CommitReservation and ReleaseReservation are intentionally NOT forwarded: they
// are saga RPCs called by team-order over gRPC with a service principal, never by
// a browser. The embedded UnimplementedVoucherServiceHandler answers
// `unimplemented` (HTTP 501) for every caller. ValidateAndReserve stays routed
// because the checkout preview uses it.

// ── FlashSaleService ──

// FlashSaleForwarder serves its Connect handler RPCs by forwarding them to the upstream gRPC client.
type FlashSaleForwarder struct {
	promotionv1connect.UnimplementedFlashSaleServiceHandler
	client promotionv1.FlashSaleServiceClient
	edge   *Edge
}

// NewFlashSaleForwarder returns the forwarder that serves its RPCs from the given upstream client.
func NewFlashSaleForwarder(client promotionv1.FlashSaleServiceClient, edge *Edge) *FlashSaleForwarder {
	return &FlashSaleForwarder{client: client, edge: edge}
}

// CreateCampaign forwards the CreateCampaign RPC to the upstream service.
func (f *FlashSaleForwarder) CreateCampaign(
	ctx context.Context,
	req *connect.Request[promotionv1.CreateCampaignRequest],
) (*connect.Response[promotionv1.CreateCampaignResponse], error) {
	var out *promotionv1.CreateCampaignResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateCampaign(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// GetActiveFlashSale forwards the GetActiveFlashSale RPC to the upstream service.
func (f *FlashSaleForwarder) GetActiveFlashSale(
	ctx context.Context,
	req *connect.Request[promotionv1.GetActiveFlashSaleRequest],
) (*connect.Response[promotionv1.GetActiveFlashSaleResponse], error) {
	var out *promotionv1.GetActiveFlashSaleResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetActiveFlashSale(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ListActiveCampaigns forwards the ListActiveCampaigns RPC to the upstream service.
func (f *FlashSaleForwarder) ListActiveCampaigns(
	ctx context.Context,
	req *connect.Request[promotionv1.ListActiveCampaignsRequest],
) (*connect.Response[promotionv1.ListActiveCampaignsResponse], error) {
	var out *promotionv1.ListActiveCampaignsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListActiveCampaigns(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// GetFlashSaleStock forwards the GetFlashSaleStock RPC to the upstream service.
func (f *FlashSaleForwarder) GetFlashSaleStock(
	ctx context.Context,
	req *connect.Request[promotionv1.GetFlashSaleStockRequest],
) (*connect.Response[promotionv1.GetFlashSaleStockResponse], error) {
	var out *promotionv1.GetFlashSaleStockResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetFlashSaleStock(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── SubscriptionService (seller subscription plans + entitlements) ──

// SubscriptionForwarder serves its Connect handler RPCs by forwarding them to the upstream gRPC client.
type SubscriptionForwarder struct {
	promotionv1connect.UnimplementedSubscriptionServiceHandler
	client promotionv1.SubscriptionServiceClient
	edge   *Edge
}

// NewSubscriptionForwarder returns the forwarder that serves its RPCs from the given upstream client.
func NewSubscriptionForwarder(client promotionv1.SubscriptionServiceClient, edge *Edge) *SubscriptionForwarder {
	return &SubscriptionForwarder{client: client, edge: edge}
}

// ListPlans forwards the ListPlans RPC to the upstream service.
func (f *SubscriptionForwarder) ListPlans(
	ctx context.Context,
	req *connect.Request[promotionv1.ListPlansRequest],
) (*connect.Response[promotionv1.ListPlansResponse], error) {
	var out *promotionv1.ListPlansResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListPlans(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// Subscribe forwards the Subscribe RPC to the upstream service.
func (f *SubscriptionForwarder) Subscribe(
	ctx context.Context,
	req *connect.Request[promotionv1.SubscribeRequest],
) (*connect.Response[promotionv1.SubscribeResponse], error) {
	var out *promotionv1.SubscribeResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.Subscribe(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// GetEntitlements forwards the GetEntitlements RPC to the upstream service.
func (f *SubscriptionForwarder) GetEntitlements(
	ctx context.Context,
	req *connect.Request[promotionv1.GetEntitlementsRequest],
) (*connect.Response[promotionv1.GetEntitlementsResponse], error) {
	var out *promotionv1.GetEntitlementsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetEntitlements(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── SponsoredService ──

// SponsoredForwarder serves its Connect handler RPCs by forwarding them to the upstream gRPC client.
type SponsoredForwarder struct {
	promotionv1connect.UnimplementedSponsoredServiceHandler
	client promotionv1.SponsoredServiceClient
	edge   *Edge
}

// NewSponsoredForwarder returns the forwarder that serves its RPCs from the given upstream client.
func NewSponsoredForwarder(client promotionv1.SponsoredServiceClient, edge *Edge) *SponsoredForwarder {
	return &SponsoredForwarder{client: client, edge: edge}
}

// CreateAdCampaign forwards the CreateAdCampaign RPC to the upstream service.
func (f *SponsoredForwarder) CreateAdCampaign(
	ctx context.Context,
	req *connect.Request[promotionv1.CreateAdCampaignRequest],
) (*connect.Response[promotionv1.CreateAdCampaignResponse], error) {
	var out *promotionv1.CreateAdCampaignResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateAdCampaign(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ListSponsoredSlots forwards the ListSponsoredSlots RPC to the upstream service.
func (f *SponsoredForwarder) ListSponsoredSlots(
	ctx context.Context,
	req *connect.Request[promotionv1.ListSponsoredSlotsRequest],
) (*connect.Response[promotionv1.ListSponsoredSlotsResponse], error) {
	var out *promotionv1.ListSponsoredSlotsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListSponsoredSlots(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
