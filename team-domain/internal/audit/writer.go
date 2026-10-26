package audit

import (
	"context"
	"fmt"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	auditv1 "github.com/buidangphuc/team-domain/generated/platform/audit/v1"
)

type Writer struct {
	client auditv1.AuditServiceClient
}

func NewWriter(client auditv1.AuditServiceClient) *Writer {
	return &Writer{client: client}
}

func (w *Writer) WriteAuditEvent(ctx context.Context, ev auditx.Event) error {
	_, err := w.client.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{
		ActorId:    ev.ActorID,
		Action:     ev.Action,
		TargetType: ev.TargetType,
		TargetId:   ev.TargetID,
		Metadata:   ev.Metadata,
	})
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}
