package upstream

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	auditv1 "github.com/buidangphuc/team-promotion/generated/platform/audit/v1"
)

// AuditEvent is one generic team-audit event.
type AuditEvent struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]string
}

// AuditClient writes audit events to team-audit over gRPC as the team-promotion
// service principal.
type AuditClient struct {
	conn    *grpc.ClientConn
	client  auditv1.AuditServiceClient
	timeout time.Duration
}

// NewAuditClient dials team-audit at addr. An empty addr yields (nil, nil): audit
// is not configured. callTimeout bounds each write (DefaultCallTimeout when <=0).
func NewAuditClient(addr string, callTimeout time.Duration) (*AuditClient, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(servicePrincipalInterceptor()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial audit %s: %w", addr, err)
	}
	return &AuditClient{conn: conn, client: auditv1.NewAuditServiceClient(conn), timeout: callTimeout}, nil
}

// Close releases the connection. Safe on a nil client.
func (c *AuditClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// WriteEvent writes one event: a single bounded attempt, no retry.
func (c *AuditClient) WriteEvent(ctx context.Context, ev AuditEvent) error {
	if c == nil {
		return fmt.Errorf("audit client not configured")
	}
	cctx, cancel := boundedCtx(ctx, c.timeout)
	defer cancel()
	_, err := c.client.WriteAuditEvent(cctx, &auditv1.WriteAuditEventRequest{
		ActorId:    ev.ActorID,
		Action:     ev.Action,
		TargetType: ev.TargetType,
		TargetId:   ev.TargetID,
		Metadata:   ev.Metadata,
	})
	return err
}

// servicePrincipalInterceptor replaces any outgoing metadata with the
// team-promotion service principal; the admin's own principal is never forwarded.
func servicePrincipalInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md := metadata.Pairs(
			"x-principal-id", "service-team-promotion",
			"x-principal-type", "service",
			"x-principal-scopes", "audit.write",
		)
		return invoker(metadata.NewOutgoingContext(ctx, md), method, req, reply, cc, opts...)
	}
}
