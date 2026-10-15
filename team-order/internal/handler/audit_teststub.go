package handler

import "context"

// TEST-STUB: minimal surface so the red tests compile. The implementer replaces this
// file (AuditWriter + WithAuditWriter move into order.go with real behaviour).

// AuditWriter writes one admin audit event.
type AuditWriter interface {
	WriteAdminEvent(ctx context.Context, actorID, action, targetType, targetID string, metadata map[string]string) error
}

// WithAuditWriter is a TEST-STUB no-op.
func WithAuditWriter(w AuditWriter) Option { return func(*OrderHandler) {} }
