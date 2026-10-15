package service

// TEST-STUB: compile-only placeholder written by the test-author. The implementer
// replaces this file (task 3) with the real AuditWriter, newCampaignID and the
// WithAudit wiring in voucher.go / flashsale.go / sponsored.go (and deletes the
// WithAudit stubs below).

import (
	"context"

	"github.com/buidangphuc/team-promotion/internal/upstream"
)

// AuditWriter is satisfied by *upstream.AuditClient.
type AuditWriter interface {
	WriteEvent(ctx context.Context, ev upstream.AuditEvent) error
}

// newCampaignID: TEST-STUB (must return a 32-char hex id).
func newCampaignID() string { return "" }

// WithAudit: TEST-STUBs (must store the writer; nil means fail closed).
func (s *VoucherService) WithAudit(w AuditWriter) *VoucherService     { return s }
func (s *FlashSaleService) WithAudit(w AuditWriter) *FlashSaleService { return s }
func (s *SponsoredService) WithAudit(w AuditWriter) *SponsoredService { return s }
