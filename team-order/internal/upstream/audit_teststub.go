package upstream

import (
	"context"
	"errors"
	"time"
)

// TEST-STUB: minimal surface so the red tests compile. The implementer replaces this
// whole file with the real audit client.

type AuditClient struct{}

func NewAuditClient(addr string, callTimeout time.Duration) (*AuditClient, error) {
	return nil, errors.New("TEST-STUB: not implemented")
}

func (c *AuditClient) Close() error { return errors.New("TEST-STUB: not implemented") }

func (c *AuditClient) WriteAdminEvent(ctx context.Context, actorID, action, targetType, targetID string, metadata map[string]string) error {
	return errors.New("TEST-STUB: not implemented")
}
