// Package upstream holds team-verification's outbound gRPC clients.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	auditv1 "github.com/buidangphuc/team-verification/generated/platform/audit/v1"
)

const (
	servicePrincipalID     = "service-team-verification"
	servicePrincipalType   = "service"
	servicePrincipalScopes = "audit.write" // WriteAuditEvent requires a service principal holding audit.write
	// DefaultCallTimeout bounds one audit call when none is configured.
	DefaultCallTimeout = 2 * time.Second
	deadlineMargin     = 500 * time.Millisecond
)

func auditMetadata() metadata.MD {
	return metadata.Pairs(
		"x-principal-id", servicePrincipalID,
		"x-principal-type", servicePrincipalType,
		"x-principal-scopes", servicePrincipalScopes,
	)
}

// auditPrincipalInterceptor replaces any outgoing principal with the service one.
func auditPrincipalInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.NewOutgoingContext(ctx, auditMetadata()), method, req, reply, cc, opts...)
}

// boundedCtx limits one call to timeout, cut down to the remaining inbound
// deadline minus a margin.
func boundedCtx(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl) - deadlineMargin; rem < timeout {
			if rem < 0 {
				rem = 0
			}
			timeout = rem
		}
	}
	return context.WithTimeout(ctx, timeout)
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

// WriteKycReviewEvent records that actorID reviewed submissionID (owned by userID) with decision.
func (c *AuditClient) WriteKycReviewEvent(ctx context.Context, actorID, submissionID, userID, decision string) error {
	if c == nil || c.svc == nil {
		return errors.New("audit client not configured")
	}
	callCtx, cancel := boundedCtx(ctx, c.timeout)
	defer cancel()
	_, err := c.svc.WriteAuditEvent(callCtx, &auditv1.WriteAuditEventRequest{
		ActorId:    actorID,
		Action:     "kyc.review",
		TargetType: "kyc_submission",
		TargetId:   submissionID,
		Metadata:   map[string]string{"decision": decision, "user_id": userID},
	})
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}
