package edge

import (
	"context"

	"connectrpc.com/connect"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
)

// AuthForwarder exposes the identity AuthService at the edge (Register/Login).
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
