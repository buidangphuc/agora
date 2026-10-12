// Package engagement holds team-engagement's business rules: reviews and ratings,
// product Q&A, disputes and wishlist collections. It validates input and calls the
// store; it knows nothing of gRPC.
package engagement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/buidangphuc/team-engagement/internal/store"
)

var (
	// ErrEmptyDisputeID is returned for empty dispute id.
	ErrEmptyDisputeID = errors.New("dispute_id is required")
	// ErrEmptyOrderID is returned for empty order id.
	ErrEmptyOrderID = errors.New("order_id is required")
	// ErrEmptyClaimantID is returned for empty claimant id.
	ErrEmptyClaimantID = errors.New("claimant_id is required")
	// ErrEmptyDefendantID is returned for empty defendant id.
	ErrEmptyDefendantID = errors.New("defendant_id is required")
	// ErrEmptyReason is returned for empty reason.
	ErrEmptyReason = errors.New("reason is required")
	// ErrInvalidDisputeStatus is returned for invalid dispute status.
	ErrInvalidDisputeStatus = errors.New("invalid dispute status")
	// ErrSameClaimantAndDef is returned for same claimant and def.
	ErrSameClaimantAndDef = errors.New("claimant and defendant cannot be the same")
	// ErrDisputeClosed is returned for dispute closed.
	ErrDisputeClosed = errors.New("dispute is already closed")
)

// DisputeService is the dispute service type.
type DisputeService struct {
	repo   store.DisputeRepository
	logger *slog.Logger
}

// NewDisputeService returns a new dispute service.
func NewDisputeService(repo store.DisputeRepository, logger *slog.Logger) *DisputeService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DisputeService{repo: repo, logger: logger}
}

// CreateDispute implements create dispute for DisputeService.
func (s *DisputeService) CreateDispute(
	ctx context.Context,
	orderID, claimantID, defendantID, reason string,
	evidenceURLs []string,
) (store.Dispute, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return store.Dispute{}, ErrEmptyOrderID
	}
	claimantID = strings.TrimSpace(claimantID)
	if claimantID == "" {
		return store.Dispute{}, ErrEmptyClaimantID
	}
	defendantID = strings.TrimSpace(defendantID)
	if defendantID == "" {
		return store.Dispute{}, ErrEmptyDefendantID
	}
	if claimantID == defendantID {
		return store.Dispute{}, ErrSameClaimantAndDef
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return store.Dispute{}, ErrEmptyReason
	}

	cleanedURLs := make([]string, 0, len(evidenceURLs))
	for _, u := range evidenceURLs {
		if trimmed := strings.TrimSpace(u); trimmed != "" {
			cleanedURLs = append(cleanedURLs, trimmed)
		}
	}

	d := store.Dispute{
		OrderID:      orderID,
		ClaimantID:   claimantID,
		DefendantID:  defendantID,
		Reason:       reason,
		EvidenceURLs: cleanedURLs,
		Status:       store.DisputeStatusOpen,
	}

	saved, err := s.repo.CreateDispute(ctx, d)
	if err != nil {
		return store.Dispute{}, fmt.Errorf("create dispute: %w", err)
	}
	return saved, nil
}

// GetDispute implements get dispute for DisputeService.
func (s *DisputeService) GetDispute(ctx context.Context, id string) (store.Dispute, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return store.Dispute{}, ErrEmptyDisputeID
	}
	return s.repo.GetDispute(ctx, id)
}

// ResolveDispute implements resolve dispute for DisputeService.
func (s *DisputeService) ResolveDispute(
	ctx context.Context,
	id string,
	status store.DisputeStatus,
	resolution string,
) (store.Dispute, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return store.Dispute{}, ErrEmptyDisputeID
	}

	switch status {
	case store.DisputeStatusResolved, store.DisputeStatusRejected, store.DisputeStatusInvestigating:
		// valid status transition
	default:
		return store.Dispute{}, ErrInvalidDisputeStatus
	}

	existing, err := s.repo.GetDispute(ctx, id)
	if err != nil {
		return store.Dispute{}, err
	}

	if existing.Status == store.DisputeStatusResolved || existing.Status == store.DisputeStatusRejected {
		return store.Dispute{}, ErrDisputeClosed
	}

	existing.Status = status
	existing.Resolution = strings.TrimSpace(resolution)

	updated, err := s.repo.UpdateDispute(ctx, existing)
	if err != nil {
		return store.Dispute{}, fmt.Errorf("update dispute: %w", err)
	}
	return updated, nil
}
