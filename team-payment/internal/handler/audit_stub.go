package handler

// TEST-STUB: minimal symbols so the red tests compile; the implementer moves these into
// payment.go (real interface + option + RefundPayment admin branch) and deletes this file.

import "context"

type AuditWriter interface {
	WriteAdminRefund(ctx context.Context, actorID, paymentID, orderID string, amount int64) error
}

func WithAuditWriter(w AuditWriter) Option { return func(*PaymentHandler) {} }
