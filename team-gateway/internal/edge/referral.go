package edge

import (
	"context"

	"connectrpc.com/connect"

	referralv1 "github.com/buidangphuc/team-gateway/generated/platform/referral/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/referral/v1/referralv1connect"
)

// ReferralForwarder forwards referral-code lifecycle calls to team-referral.
type ReferralForwarder struct {
	referralv1connect.UnimplementedReferralServiceHandler
	client referralv1.ReferralServiceClient
	edge   *Edge
}

func NewReferralForwarder(client referralv1.ReferralServiceClient, edge *Edge) *ReferralForwarder {
	return &ReferralForwarder{client: client, edge: edge}
}

func (f *ReferralForwarder) CreateReferralCode(
	ctx context.Context,
	req *connect.Request[referralv1.CreateReferralCodeRequest],
) (*connect.Response[referralv1.CreateReferralCodeResponse], error) {
	var out *referralv1.CreateReferralCodeResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateReferralCode(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ReferralForwarder) GetMyReferral(
	ctx context.Context,
	req *connect.Request[referralv1.GetMyReferralRequest],
) (*connect.Response[referralv1.GetMyReferralResponse], error) {
	var out *referralv1.GetMyReferralResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetMyReferral(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ReferralForwarder) RedeemReferral(
	ctx context.Context,
	req *connect.Request[referralv1.RedeemReferralRequest],
) (*connect.Response[referralv1.RedeemReferralResponse], error) {
	var out *referralv1.RedeemReferralResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RedeemReferral(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ReferralForwarder) ListReferralRewards(
	ctx context.Context,
	req *connect.Request[referralv1.ListReferralRewardsRequest],
) (*connect.Response[referralv1.ListReferralRewardsResponse], error) {
	var out *referralv1.ListReferralRewardsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListReferralRewards(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
