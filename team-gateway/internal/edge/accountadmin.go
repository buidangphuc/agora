package edge

import (
	"context"

	"connectrpc.com/connect"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
)

// AccountAdminForwarder forwards admin user-listing and enforcement calls to
// team-identity. SetUserStatus is a write and is never retried.
type AccountAdminForwarder struct {
	identityv1connect.UnimplementedAccountAdminServiceHandler
	client identityv1.AccountAdminServiceClient
	edge   *Edge
}

func NewAccountAdminForwarder(client identityv1.AccountAdminServiceClient, edge *Edge) *AccountAdminForwarder {
	return &AccountAdminForwarder{client: client, edge: edge}
}

func (f *AccountAdminForwarder) ListUsers(
	ctx context.Context,
	req *connect.Request[identityv1.ListUsersRequest],
) (*connect.Response[identityv1.ListUsersResponse], error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	var out *identityv1.ListUsersResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListUsers(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AccountAdminForwarder) SetUserStatus(
	ctx context.Context,
	req *connect.Request[identityv1.SetUserStatusRequest],
) (*connect.Response[identityv1.SetUserStatusResponse], error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	var out *identityv1.SetUserStatusResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SetUserStatus(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
