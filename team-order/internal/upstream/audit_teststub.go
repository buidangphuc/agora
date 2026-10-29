package upstream

// TEST-STUB (task 1.4): compile-only placeholder so the tests build red. The
// implementer replaces this file with the real AuditWriter in audit.go and
// deletes this file.

import (
	"context"
	"errors"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"

	auditv1 "github.com/buidangphuc/team-order/generated/platform/audit/v1"
)

type AuditWriter struct{}

func NewAuditWriter(_ auditv1.AuditServiceClient) *AuditWriter { return &AuditWriter{} }

func (*AuditWriter) WriteAuditEvent(_ context.Context, _ auditx.Event) error {
	return errors.New("teststub: AuditWriter not implemented")
}
