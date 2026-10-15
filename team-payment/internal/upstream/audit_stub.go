package upstream

// TEST-STUB: minimal signatures so the red tests compile; the implementer replaces this
// whole file with the real internal/upstream/audit.go (and deletes this one).

import (
	"context"
	"errors"
	"time"
)

type AuditClient struct{}

func NewAuditClient(addr string, callTimeout time.Duration) (*AuditClient, error) {
	return nil, nil
}

func (c *AuditClient) Close() error { return nil }

func (c *AuditClient) WriteAdminRefund(ctx context.Context, actorID, paymentID, orderID string, amount int64) error {
	return errors.New("TEST-STUB: not implemented")
}
