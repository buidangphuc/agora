package upstream

import (
	"context"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"

	auditv1 "github.com/buidangphuc/team-order/generated/platform/audit/v1"
)

type AuditWriter struct {
	client auditv1.AuditServiceClient
}

func NewAuditWriter(client auditv1.AuditServiceClient) *AuditWriter {
	return &AuditWriter{client: client}
}

func (w *AuditWriter) WriteAuditEvent(ctx context.Context, ev auditx.Event) error {
	_, err := w.client.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{
		ActorId:    ev.ActorID,
		Action:     ev.Action,
		TargetType: ev.TargetType,
		TargetId:   ev.TargetID,
		Metadata:   ev.Metadata,
	})
	return err
}
