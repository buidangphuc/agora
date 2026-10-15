package service

// RED-PHASE STUB (test author). The implementer DELETES this file and moves the
// real declarations into review.go. It exists only so the tests compile.

import (
	"context"
	"errors"
)

// AuditWriter is the slice of internal/upstream.AuditClient the service needs.
type AuditWriter interface {
	WriteHideEvent(ctx context.Context, actorID, reviewID, listingID, reason string) error
}

var (
	ErrOrderUnavailable = errors.New("stub: order verification unavailable")
	ErrOrderNotEligible = errors.New("stub: order not eligible for review")
	ErrReasonTooLong    = errors.New("stub: reason too long")
	ErrAuditUnavailable = errors.New("stub: audit unavailable")
)

// WithAudit wires the audit writer (nil or never called => HideReview is Unavailable).
func (s *ReviewService) WithAudit(w AuditWriter) *ReviewService { return s }

// HideReview is not implemented yet.
func (s *ReviewService) HideReview(ctx context.Context, adminID, reviewID, reason string) error {
	return errors.New("stub: not implemented")
}
