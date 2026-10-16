// Package upstream holds team-identity's clients for other services' gRPC APIs.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	auditv1 "github.com/buidangphuc/team-identity/generated/platform/audit/v1"
)

// Identity calls team-audit as itself, never as the admin whose request it is serving.
const (
	servicePrincipalID     = "service-team-identity"
	servicePrincipalType   = "service"
	servicePrincipalScopes = "audit.write"
)

// ErrAuditNotConfigured is returned when no audit upstream is configured; callers
// must refuse the change (fail closed).
var ErrAuditNotConfigured = errors.New("audit upstream not configured")

// AuditClient writes audit events to team-audit. A nil *AuditClient means "not
// configured": Close is a no-op and writes fail closed.
type AuditClient struct {
	conn    *grpc.ClientConn
	client  auditv1.AuditServiceClient
	timeout time.Duration
}

// NewAuditClient dials addr lazily. An empty addr yields (nil, nil).
func NewAuditClient(addr string, timeout time.Duration) (*AuditClient, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial audit %s: %w", addr, err)
	}
	return &AuditClient{conn: conn, client: auditv1.NewAuditServiceClient(conn), timeout: timeout}, nil
}

// Close releases the connection; safe on a nil client.
func (c *AuditClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// WriteUserSetStatus records a user.set_status event. note and until are omitted
// from the metadata when empty/nil.
func (c *AuditClient) WriteUserSetStatus(ctx context.Context, actorID, userID, status, reasonCode, note string, until *time.Time) error {
	if c == nil {
		return ErrAuditNotConfigured
	}
	meta := map[string]string{"status": status, "reason_code": reasonCode}
	if note != "" {
		meta["note"] = note
	}
	if until != nil {
		meta["until"] = until.UTC().Format(time.RFC3339)
	}

	md := metadata.MD{}
	if old, ok := metadata.FromOutgoingContext(ctx); ok {
		md = old.Copy()
	}
	md.Set("x-principal-id", servicePrincipalID)
	md.Set("x-principal-type", servicePrincipalType)
	md.Set("x-principal-scopes", servicePrincipalScopes)
	ctx = metadata.NewOutgoingContext(ctx, md)

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	_, err := c.client.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{
		ActorId:    actorID,
		Action:     "user.set_status",
		TargetType: "user",
		TargetId:   userID,
		Metadata:   meta,
	})
	return err
}
