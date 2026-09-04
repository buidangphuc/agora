package edge

import (
	"context"

	"connectrpc.com/connect"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
)

// AddressForwarder implements Connect AddressServiceHandler by forwarding to team-identity.
type AddressForwarder struct {
	identityv1connect.UnimplementedAddressServiceHandler
	client identityv1.AddressServiceClient
	edge   *Edge
}

func NewAddressForwarder(client identityv1.AddressServiceClient, edge *Edge) *AddressForwarder {
	return &AddressForwarder{client: client, edge: edge}
}

func (f *AddressForwarder) ListAddresses(
	ctx context.Context,
	req *connect.Request[identityv1.ListAddressesRequest],
) (*connect.Response[identityv1.ListAddressesResponse], error) {
	var out *identityv1.ListAddressesResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListAddresses(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AddressForwarder) CreateAddress(
	ctx context.Context,
	req *connect.Request[identityv1.CreateAddressRequest],
) (*connect.Response[identityv1.CreateAddressResponse], error) {
	var out *identityv1.CreateAddressResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateAddress(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AddressForwarder) UpdateAddress(
	ctx context.Context,
	req *connect.Request[identityv1.UpdateAddressRequest],
) (*connect.Response[identityv1.UpdateAddressResponse], error) {
	var out *identityv1.UpdateAddressResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UpdateAddress(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AddressForwarder) DeleteAddress(
	ctx context.Context,
	req *connect.Request[identityv1.DeleteAddressRequest],
) (*connect.Response[identityv1.DeleteAddressResponse], error) {
	var out *identityv1.DeleteAddressResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.DeleteAddress(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AddressForwarder) SetDefaultAddress(
	ctx context.Context,
	req *connect.Request[identityv1.SetDefaultAddressRequest],
) (*connect.Response[identityv1.SetDefaultAddressResponse], error) {
	var out *identityv1.SetDefaultAddressResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SetDefaultAddress(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
