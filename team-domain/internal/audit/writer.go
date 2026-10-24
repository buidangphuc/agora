package audit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	auditv1 "github.com/buidangphuc/team-domain/generated/platform/audit/v1"
)

const (
	servicePrincipalID         = "service-team-domain"
	servicePrincipalType       = "service"
	auditServicePrincipalScope = "audit.write"
)

func auditMetadata() metadata.MD {
	return metadata.Pairs(
		"x-principal-id", servicePrincipalID,
		"x-principal-type", servicePrincipalType,
		"x-principal-scopes", auditServicePrincipalScope,
	)
}

// auditPrincipalInterceptor replaces any outgoing principal with the service one.
func auditPrincipalInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.NewOutgoingContext(ctx, auditMetadata()), method, req, reply, cc, opts...)
}

// AuditClient writes audit events to team-audit over gRPC (single attempt, bounded).
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

// Close releases the underlying connection. Nil-safe.
func (c *AuditClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// WriteListingAdminUpdate records an admin update of listingID.
func (c *AuditClient) WriteListingAdminUpdate(ctx context.Context, actorID, listingID, sellerID, status string) error {
	return c.write(ctx, "listing.admin_update", actorID, listingID,
		map[string]string{"seller_id": sellerID, "status": status})
}

// WriteListingAdminDelete records an admin delete of listingID.
func (c *AuditClient) WriteListingAdminDelete(ctx context.Context, actorID, listingID, sellerID string) error {
	return c.write(ctx, "listing.admin_delete", actorID, listingID,
		map[string]string{"seller_id": sellerID})
}

// WriteListingModerate records an admin moderation decision on listingID.
func (c *AuditClient) WriteListingModerate(ctx context.Context, actorID, listingID, sellerID, decision, reasonCode, note, fromStatus, toStatus string) error {
	return c.write(ctx, "listing.moderate", actorID, listingID, map[string]string{
		"seller_id": sellerID, "decision": decision, "reason_code": reasonCode, "note": note,
		"from_status": fromStatus, "to_status": toStatus,
	})
}

func (c *AuditClient) write(ctx context.Context, action, actorID, listingID string, md map[string]string) error {
	if c == nil || c.svc == nil {
		return errors.New("audit client not configured")
	}
	callCtx, cancel := boundedCtx(ctx, c.timeout)
	defer cancel()
	_, err := c.svc.WriteAuditEvent(callCtx, &auditv1.WriteAuditEventRequest{
		ActorId:    actorID,
		Action:     action,
		TargetType: "listing",
		TargetId:   listingID,
		Metadata:   md,
	})
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}
