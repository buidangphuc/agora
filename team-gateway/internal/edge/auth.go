package edge

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
)

// AuthForwarder exposes the identity AuthService at the edge (Register/Login/RefreshSession).
// These are public — no Principal is required — so it forwards straight to
// team-identity. Not retried (they are writes / credential-sensitive).
type AuthForwarder struct {
	identityv1connect.UnimplementedAuthServiceHandler
	client identityv1.AuthServiceClient
	edge   *Edge
}

func NewAuthForwarder(client identityv1.AuthServiceClient, edge *Edge) *AuthForwarder {
	return &AuthForwarder{client: client, edge: edge}
}

func (f *AuthForwarder) Register(
	ctx context.Context,
	req *connect.Request[identityv1.RegisterRequest],
) (*connect.Response[identityv1.RegisterResponse], error) {
	var out *identityv1.RegisterResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.Register(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AuthForwarder) Login(
	ctx context.Context,
	req *connect.Request[identityv1.LoginRequest],
) (*connect.Response[identityv1.LoginResponse], error) {
	var out *identityv1.LoginResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.Login(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AuthForwarder) RefreshSession(
	ctx context.Context,
	req *connect.Request[identityv1.RefreshSessionRequest],
) (*connect.Response[identityv1.RefreshSessionResponse], error) {
	var out *identityv1.RefreshSessionResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RefreshSession(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AuthForwarder) CreateHandoffCode(
	ctx context.Context,
	req *connect.Request[identityv1.CreateHandoffCodeRequest],
) (*connect.Response[identityv1.CreateHandoffCodeResponse], error) {
	if p, ok := principalFrom(ctx); !ok || p.ptype == "anonymous" || p.id == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	var out *identityv1.CreateHandoffCodeResponse
	err := f.edge.callWrite(f.edge.outgoingWithSID(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateHandoffCode(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AuthForwarder) PeekHandoffCode(
	ctx context.Context,
	req *connect.Request[identityv1.PeekHandoffCodeRequest],
) (*connect.Response[identityv1.PeekHandoffCodeResponse], error) {
	var out *identityv1.PeekHandoffCodeResponse
	err := f.edge.callRead(f.edge.outgoingWithSID(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.PeekHandoffCode(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AuthForwarder) RedeemHandoffCode(
	ctx context.Context,
	req *connect.Request[identityv1.RedeemHandoffCodeRequest],
) (*connect.Response[identityv1.RedeemHandoffCodeResponse], error) {
	var out *identityv1.RedeemHandoffCodeResponse
	err := f.edge.callWrite(f.edge.outgoingWithSID(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RedeemHandoffCode(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AuthForwarder) BecomeSeller(
	ctx context.Context,
	req *connect.Request[identityv1.BecomeSellerRequest],
) (*connect.Response[identityv1.BecomeSellerResponse], error) {
	p, ok := principalFrom(ctx)
	if !ok || p.ptype == "anonymous" || p.id == "" || p.id == "anonymous" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if p.ptype != "user" {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("user principal required"))
	}
	var out *identityv1.BecomeSellerResponse
	err := f.edge.callWrite(f.edge.outgoingWithSID(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.BecomeSeller(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
