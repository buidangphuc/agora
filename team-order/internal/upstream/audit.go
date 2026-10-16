package upstream

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	auditv1 "github.com/buidangphuc/team-order/generated/platform/audit/v1"
)

const auditScope = "audit.write"

// AuditClient writes admin audit events to team-audit as team-order's own service
// principal with exactly the scope audit.write. The call is a single attempt bounded
// by the configured timeout; the caller decides what a failure means (fail closed).
type AuditClient struct {
	conn    *grpc.ClientConn
	client  auditv1.AuditServiceClient
	timeout time.Duration
}

// NewAuditClient opens a lazy gRPC client to team-audit at addr. An empty addr
// returns (nil, nil): the audited admin actions then fail closed.
func NewAuditClient(addr string, callTimeout time.Duration) (*AuditClient, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial audit %s: %w", addr, err)
	}
	return &AuditClient{conn: conn, client: auditv1.NewAuditServiceClient(conn), timeout: callTimeout}, nil
}

// Close releases the connection; safe on a nil client.
func (c *AuditClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// WriteAdminEvent records one admin action. The outgoing metadata is replaced
// wholesale: the admin's principal and token are never forwarded.
func (c *AuditClient) WriteAdminEvent(ctx context.Context, actorID, action, targetType, targetID string, meta map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		mdPrincipalIDKey, servicePrincipalID,
		mdPrincipalTypeKey, servicePrincipalType,
		mdPrincipalScopesKey, auditScope,
	))
	_, err := c.client.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{
		ActorId:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetId:   targetID,
		Metadata:   meta,
	}, grpc.WaitForReady(false))
	return err
}
