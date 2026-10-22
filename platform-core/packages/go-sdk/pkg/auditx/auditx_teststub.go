package auditx

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

// TEST-STUB: compile-only placeholder for task 1.4; the implementer replaces it with auditx.go and deletes this file.
const Message = "audit trail unavailable"

type Event struct {
	ActorID, Action, TargetType, TargetID string
	Metadata                              map[string]string
}

type Writer interface {
	WriteAuditEvent(ctx context.Context, ev Event) error
}

type Gate struct{}

func NewGate(w Writer, logger *slog.Logger) *Gate { return &Gate{} }

func (g *Gate) Do(ctx context.Context, ev Event, action func(context.Context) error) error {
	return nil
}

func Dial(cfg config.Audit, serviceID string, scopes ...string) (*grpc.ClientConn, error) {
	return nil, nil
}
