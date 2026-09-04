package edge

import (
	"context"

	"connectrpc.com/connect"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/analytics/v1/analyticsv1connect"
)

// AnalyticsQueryForwarder forwards seller-dashboard read-model queries to
// team-analytics' AnalyticsQueryService. Every RPC is read-only.
type AnalyticsQueryForwarder struct {
	analyticsv1connect.UnimplementedAnalyticsQueryServiceHandler
	client analyticsv1.AnalyticsQueryServiceClient
	edge   *Edge
}

func NewAnalyticsQueryForwarder(client analyticsv1.AnalyticsQueryServiceClient, edge *Edge) *AnalyticsQueryForwarder {
	return &AnalyticsQueryForwarder{client: client, edge: edge}
}

func (f *AnalyticsQueryForwarder) GetSellerFunnel(
	ctx context.Context,
	req *connect.Request[analyticsv1.GetSellerFunnelRequest],
) (*connect.Response[analyticsv1.GetSellerFunnelResponse], error) {
	var out *analyticsv1.GetSellerFunnelResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetSellerFunnel(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AnalyticsQueryForwarder) GetRevenueBreakdown(
	ctx context.Context,
	req *connect.Request[analyticsv1.GetRevenueBreakdownRequest],
) (*connect.Response[analyticsv1.GetRevenueBreakdownResponse], error) {
	var out *analyticsv1.GetRevenueBreakdownResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetRevenueBreakdown(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
