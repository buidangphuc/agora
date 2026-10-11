package edge

import (
	"context"

	"connectrpc.com/connect"

	auditv1 "github.com/buidangphuc/team-gateway/generated/platform/audit/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/audit/v1/auditv1connect"
)

// AuditForwarder forwards append-only audit-log calls to team-audit.
type AuditForwarder struct {
	auditv1connect.UnimplementedAuditServiceHandler
	client auditv1.AuditServiceClient
	edge   *Edge
}

func NewAuditForwarder(client auditv1.AuditServiceClient, edge *Edge) *AuditForwarder {
	return &AuditForwarder{client: client, edge: edge}
}

// WriteAuditEvent is intentionally NOT forwarded: audit events are written by
// services over gRPC with a service principal, never by a browser. The embedded
// UnimplementedAuditServiceHandler answers `unimplemented` (HTTP 501) for every
// caller.

func (f *AuditForwarder) QueryAuditLog(
	ctx context.Context,
	req *connect.Request[auditv1.QueryAuditLogRequest],
) (*connect.Response[auditv1.QueryAuditLogResponse], error) {
	var out *auditv1.QueryAuditLogResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.QueryAuditLog(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
