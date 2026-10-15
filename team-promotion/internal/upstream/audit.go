package upstream

// TEST-STUB: compile-only placeholder written by the test-author. The implementer
// replaces this whole file (task 2). Every method deliberately does nothing useful.

import (
	"context"
	"errors"
	"time"
)

// AuditEvent is one generic team-audit event.
type AuditEvent struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]string
}

// AuditClient writes audit events to team-audit over gRPC.
type AuditClient struct{}

// NewAuditClient: TEST-STUB.
func NewAuditClient(addr string, callTimeout time.Duration) (*AuditClient, error) {
	return nil, errors.New("TEST-STUB: not implemented")
}

// Close: TEST-STUB.
func (c *AuditClient) Close() error { return nil }

// WriteEvent: TEST-STUB.
func (c *AuditClient) WriteEvent(ctx context.Context, ev AuditEvent) error {
	return errors.New("TEST-STUB: not implemented")
}
