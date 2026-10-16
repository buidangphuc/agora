package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-promotion/internal/upstream"
)

// AuditWriter is satisfied by *upstream.AuditClient.
type AuditWriter interface {
	WriteEvent(ctx context.Context, ev upstream.AuditEvent) error
}

// writeAudit writes ev through w, failing closed: a nil writer or any write error
// is Unavailable so the audited action is refused.
func writeAudit(ctx context.Context, w AuditWriter, ev upstream.AuditEvent) error {
	if w == nil {
		return status.Error(codes.Unavailable, "audit trail not configured")
	}
	if err := w.WriteEvent(ctx, ev); err != nil {
		return status.Error(codes.Unavailable, "audit trail unavailable")
	}
	return nil
}

// newCampaignID returns a random 128-bit id as 32 hex chars, pre-generated so the
// audit event can carry the id the campaign is stored under.
func newCampaignID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(errors.New("promotion: crypto/rand unavailable: " + err.Error()))
	}
	return hex.EncodeToString(b[:])
}
