package upstream

import (
	"context"
	"errors"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
)

type stubWriter struct{}

func (stubWriter) WriteAuditEvent(context.Context, auditx.Event) error {
	return errors.New("TEST-STUB: NewAuditWriter not implemented")
}

func NewAuditWriter(_ *grpc.ClientConn) auditx.Writer { return stubWriter{} }
