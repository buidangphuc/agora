// Package auditx gates a privileged action behind a synchronous audit write.
package auditx

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

// Message is the client-visible text of the error returned when the audit
// trail cannot be written.
const Message = "audit trail unavailable"

const writeTimeout = 2 * time.Second

// Event is one audit record.
type Event struct {
	ActorID, Action, TargetType, TargetID string
	Metadata                              map[string]string
}

// Writer persists an audit event.
type Writer interface {
	WriteAuditEvent(ctx context.Context, ev Event) error
}

// Gate runs actions only after their audit event is written.
type Gate struct {
	w      Writer
	logger *slog.Logger
}

// NewGate returns a Gate. A nil writer makes every Do fail closed.
func NewGate(w Writer, logger *slog.Logger) *Gate {
	if logger == nil {
		logger = slog.Default()
	}
	return &Gate{w: w, logger: logger}
}

// Do writes ev, then runs action. If the write fails, times out or no writer
// is configured, action does not run and Do returns codes.Unavailable with
// Message. The action's own error is returned unchanged.
func (g *Gate) Do(ctx context.Context, ev Event, action func(context.Context) error) error {
	if g.w == nil {
		g.logger.ErrorContext(ctx, "audit write skipped: no writer", "action", ev.Action, "target_id", ev.TargetID)
		return status.Error(codes.Unavailable, Message)
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := g.w.WriteAuditEvent(wctx, ev); err != nil {
		g.logger.ErrorContext(ctx, "audit write failed", "action", ev.Action, "target_id", ev.TargetID, "error", err.Error())
		return status.Error(codes.Unavailable, Message)
	}
	return action(ctx)
}

// Dial connects to the audit service, sending serviceID and scopes as the
// service principal on every call. An empty address returns a nil connection.
func Dial(cfg config.Audit, serviceID string, scopes ...string) (*grpc.ClientConn, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, nil
	}
	joined := strings.Join(scopes, ",")
	inject := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx,
			auth.MDPrincipalID, serviceID,
			auth.MDPrincipalType, "service",
			auth.MDPrincipalScopes, joined)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	return grpc.NewClient(cfg.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(inject))
}
