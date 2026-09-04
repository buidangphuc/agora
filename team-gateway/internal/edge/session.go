package edge

import (
	"context"

	"connectrpc.com/connect"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
)

// SessionForwarder implements the Connect SessionServiceHandler by forwarding to
// the upstream team-identity gRPC service (same endpoint as AuthService).
type SessionForwarder struct {
	identityv1connect.UnimplementedSessionServiceHandler
	client identityv1.SessionServiceClient
	edge   *Edge
}

func NewSessionForwarder(client identityv1.SessionServiceClient, edge *Edge) *SessionForwarder {
	return &SessionForwarder{client: client, edge: edge}
}

func (f *SessionForwarder) ListSessions(
	ctx context.Context,
	req *connect.Request[identityv1.ListSessionsRequest],
) (*connect.Response[identityv1.ListSessionsResponse], error) {
	var out *identityv1.ListSessionsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListSessions(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *SessionForwarder) RevokeSession(
	ctx context.Context,
	req *connect.Request[identityv1.RevokeSessionRequest],
) (*connect.Response[identityv1.RevokeSessionResponse], error) {
	var out *identityv1.RevokeSessionResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RevokeSession(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *SessionForwarder) ListLoginHistory(
	ctx context.Context,
	req *connect.Request[identityv1.ListLoginHistoryRequest],
) (*connect.Response[identityv1.ListLoginHistoryResponse], error) {
	var out *identityv1.ListLoginHistoryResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListLoginHistory(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
