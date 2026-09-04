package edge

import (
	"context"

	"connectrpc.com/connect"

	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/promotion/v1/promotionv1connect"
)

// ── VoucherService ──

type VoucherForwarder struct {
	promotionv1connect.UnimplementedVoucherServiceHandler
	client promotionv1.VoucherServiceClient
	edge   *Edge
}

func NewVoucherForwarder(client promotionv1.VoucherServiceClient, edge *Edge) *VoucherForwarder {
	return &VoucherForwarder{client: client, edge: edge}
}

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

func (f *VoucherForwarder) CommitReservation(
	ctx context.Context,
	req *connect.Request[promotionv1.CommitReservationRequest],
) (*connect.Response[promotionv1.CommitReservationResponse], error) {
	var out *promotionv1.CommitReservationResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CommitReservation(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *VoucherForwarder) ReleaseReservation(
	ctx context.Context,
	req *connect.Request[promotionv1.ReleaseReservationRequest],
) (*connect.Response[promotionv1.ReleaseReservationResponse], error) {
	var out *promotionv1.ReleaseReservationResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ReleaseReservation(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── FlashSaleService ──

type FlashSaleForwarder struct {
	promotionv1connect.UnimplementedFlashSaleServiceHandler
	client promotionv1.FlashSaleServiceClient
	edge   *Edge
}

func NewFlashSaleForwarder(client promotionv1.FlashSaleServiceClient, edge *Edge) *FlashSaleForwarder {
	return &FlashSaleForwarder{client: client, edge: edge}
}

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

type SubscriptionForwarder struct {
	promotionv1connect.UnimplementedSubscriptionServiceHandler
	client promotionv1.SubscriptionServiceClient
	edge   *Edge
}

func NewSubscriptionForwarder(client promotionv1.SubscriptionServiceClient, edge *Edge) *SubscriptionForwarder {
	return &SubscriptionForwarder{client: client, edge: edge}
}

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

type SponsoredForwarder struct {
	promotionv1connect.UnimplementedSponsoredServiceHandler
	client promotionv1.SponsoredServiceClient
	edge   *Edge
}

func NewSponsoredForwarder(client promotionv1.SponsoredServiceClient, edge *Edge) *SponsoredForwarder {
	return &SponsoredForwarder{client: client, edge: edge}
}

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
