package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/buidangphuc/team-engagement/internal/repository"
)

var (
	ErrEmptyDisputeID       = errors.New("dispute_id is required")
	ErrEmptyOrderID         = errors.New("order_id is required")
	ErrEmptyClaimantID      = errors.New("claimant_id is required")
	ErrEmptyDefendantID     = errors.New("defendant_id is required")
	ErrEmptyReason          = errors.New("reason is required")
	ErrInvalidDisputeStatus = errors.New("invalid dispute status")
	ErrSameClaimantAndDef   = errors.New("claimant and defendant cannot be the same")
	ErrDisputeClosed        = errors.New("dispute is already closed")
)

type DisputeService struct {
	repo   repository.DisputeRepository
	logger *slog.Logger
}

func NewDisputeService(repo repository.DisputeRepository, logger *slog.Logger) *DisputeService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DisputeService{repo: repo, logger: logger}
}

func (s *DisputeService) CreateDispute(
	ctx context.Context,
	orderID, claimantID, defendantID, reason string,
	evidenceURLs []string,
) (repository.Dispute, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return repository.Dispute{}, ErrEmptyOrderID
	}
	claimantID = strings.TrimSpace(claimantID)
	if claimantID == "" {
		return repository.Dispute{}, ErrEmptyClaimantID
	}
	defendantID = strings.TrimSpace(defendantID)
	if defendantID == "" {
		return repository.Dispute{}, ErrEmptyDefendantID
	}
	if claimantID == defendantID {
		return repository.Dispute{}, ErrSameClaimantAndDef
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return repository.Dispute{}, ErrEmptyReason
	}

	cleanedURLs := make([]string, 0, len(evidenceURLs))
	for _, u := range evidenceURLs {
		if trimmed := strings.TrimSpace(u); trimmed != "" {
			cleanedURLs = append(cleanedURLs, trimmed)
		}
	}

	d := repository.Dispute{
		OrderID:      orderID,
		ClaimantID:   claimantID,
		DefendantID:  defendantID,
		Reason:       reason,
		EvidenceURLs: cleanedURLs,
		Status:       repository.DisputeStatusOpen,
	}

	saved, err := s.repo.CreateDispute(ctx, d)
	if err != nil {
		return repository.Dispute{}, fmt.Errorf("create dispute: %w", err)
	}
	return saved, nil
}

func (s *DisputeService) GetDispute(ctx context.Context, id string) (repository.Dispute, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return repository.Dispute{}, ErrEmptyDisputeID
	}
	return s.repo.GetDispute(ctx, id)
}

func (s *DisputeService) ResolveDispute(
	ctx context.Context,
	id string,
	status repository.DisputeStatus,
	resolution string,
) (repository.Dispute, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return repository.Dispute{}, ErrEmptyDisputeID
	}

	switch status {
	case repository.DisputeStatusResolved, repository.DisputeStatusRejected, repository.DisputeStatusInvestigating:
		// valid status transition
	default:
		return repository.Dispute{}, ErrInvalidDisputeStatus
	}

	existing, err := s.repo.GetDispute(ctx, id)
	if err != nil {
		return repository.Dispute{}, err
	}

	if existing.Status == repository.DisputeStatusResolved || existing.Status == repository.DisputeStatusRejected {
		return repository.Dispute{}, ErrDisputeClosed
	}

	existing.Status = status
	existing.Resolution = strings.TrimSpace(resolution)

	updated, err := s.repo.UpdateDispute(ctx, existing)
	if err != nil {
		return repository.Dispute{}, fmt.Errorf("update dispute: %w", err)
	}
	return updated, nil
}
