package edge

import (
	"context"

	"connectrpc.com/connect"

	sharingv1 "github.com/buidangphuc/team-gateway/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/sharing/v1/sharingv1connect"
)

// SharingForwarder forwards short-link create/resolve calls to team-sharing.
type SharingForwarder struct {
	sharingv1connect.UnimplementedSharingServiceHandler
	client sharingv1.SharingServiceClient
	edge   *Edge
}

func NewSharingForwarder(client sharingv1.SharingServiceClient, edge *Edge) *SharingForwarder {
	return &SharingForwarder{client: client, edge: edge}
}

func (f *SharingForwarder) CreateShareLink(
	ctx context.Context,
	req *connect.Request[sharingv1.CreateShareLinkRequest],
) (*connect.Response[sharingv1.CreateShareLinkResponse], error) {
	var out *sharingv1.CreateShareLinkResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateShareLink(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *SharingForwarder) ResolveShareLink(
	ctx context.Context,
	req *connect.Request[sharingv1.ResolveShareLinkRequest],
) (*connect.Response[sharingv1.ResolveShareLinkResponse], error) {
	var out *sharingv1.ResolveShareLinkResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ResolveShareLink(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
