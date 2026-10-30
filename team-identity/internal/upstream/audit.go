// Package upstream holds team-identity's clients for other services' gRPC APIs.
package upstream

import (
	"context"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	auditv1 "github.com/buidangphuc/team-identity/generated/platform/audit/v1"
)

type auditWriter struct {
	client auditv1.AuditServiceClient
}

func NewAuditWriter(conn *grpc.ClientConn) auditx.Writer {
	if conn == nil {
		return nil
	}
	return &auditWriter{client: auditv1.NewAuditServiceClient(conn)}
}

func (w *auditWriter) WriteAuditEvent(ctx context.Context, ev auditx.Event) error {
	_, err := w.client.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{
		ActorId:    ev.ActorID,
		Action:     ev.Action,
		TargetType: ev.TargetType,
		TargetId:   ev.TargetID,
		Metadata:   ev.Metadata,
	})
	return err
}
