package upstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	auditv1 "github.com/buidangphuc/team-engagement/generated/platform/audit/v1"
)

// Scope team-engagement presents to team-audit (WriteAuditEvent requires a service
// principal holding audit.write).
const auditServicePrincipalScopes = "audit.write"

func auditMetadata() metadata.MD {
	return metadata.Pairs(
		"x-principal-id", servicePrincipalID,
		"x-principal-type", servicePrincipalType,
		"x-principal-scopes", auditServicePrincipalScopes,
	)
}

// auditPrincipalInterceptor replaces any outgoing principal with the service one.
func auditPrincipalInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.NewOutgoingContext(ctx, auditMetadata()), method, req, reply, cc, opts...)
}

// AuditClient writes audit events to team-audit over gRPC.
type AuditClient struct {
	conn    *grpc.ClientConn
	svc     auditv1.AuditServiceClient
	timeout time.Duration
}

// NewAuditClient dials addr (UPSTREAM_AUDIT_ADDR). Empty addr returns (nil, nil).
func NewAuditClient(addr string, callTimeout time.Duration) (*AuditClient, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(auditPrincipalInterceptor))
	if err != nil {
		return nil, fmt.Errorf("dial team-audit at %s: %w", addr, err)
	}
	return &AuditClient{conn: conn, svc: auditv1.NewAuditServiceClient(conn), timeout: callTimeout}, nil
}

// Close releases the underlying connection.
func (c *AuditClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// WriteHideEvent records that actorID hid reviewID (on listingID) for reason.
func (c *AuditClient) WriteHideEvent(ctx context.Context, actorID, reviewID, listingID, reason string) error {
	if c == nil || c.svc == nil {
		return errors.New("audit client not configured")
	}
	callCtx, cancel := boundedCtx(ctx, c.timeout)
	defer cancel()
	_, err := c.svc.WriteAuditEvent(callCtx, &auditv1.WriteAuditEventRequest{
		ActorId:    actorID,
		Action:     "review.hide",
		TargetType: "review",
		TargetId:   reviewID,
		Metadata:   map[string]string{"reason": reason, "listing_id": listingID},
	})
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

// AuditEvent is one generic team-audit event.
// TEST-STUB (implementer replaces): struct shape fixed by tests, WriteEvent unimplemented.
type AuditEvent struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]string
}

// WriteEvent writes ev to team-audit. TEST-STUB: not implemented yet.
func (c *AuditClient) WriteEvent(ctx context.Context, ev AuditEvent) error {
	return errors.New("not implemented")
}
