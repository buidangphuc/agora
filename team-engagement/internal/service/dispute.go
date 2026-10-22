package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/upstream"
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
	ErrInvalidTransition    = errors.New("invalid dispute status transition")
	ErrRulingRequired       = errors.New("ruling is required to resolve a return-linked dispute")
	ErrRulingNotAllowed     = errors.New("ruling is only allowed when resolving")
)

type DisputeService struct {
	repo   repository.DisputeRepository
	audit  AuditWriter
	logger *slog.Logger
}

// WithAudit wires the audit writer. Without one, ResolveDispute fails closed.
func (s *DisputeService) WithAudit(w AuditWriter) *DisputeService {
	s.audit = w
	return s
}

func NewDisputeService(repo repository.DisputeRepository, logger *slog.Logger) *DisputeService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DisputeService{repo: repo, logger: logger}
}

func (s *DisputeService) CreateDispute(
	ctx context.Context,
	orderID, claimantID, defendantID, returnID, reason string,
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
		ReturnID:     strings.TrimSpace(returnID),
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

func (s *DisputeService) ListOrderDisputes(ctx context.Context, orderID string) ([]repository.Dispute, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, ErrEmptyOrderID
	}
	return s.repo.ListByOrder(ctx, orderID)
}

func (s *DisputeService) ListDisputes(
	ctx context.Context,
	status repository.DisputeStatus,
	pageSize int,
	cursor string,
) ([]repository.Dispute, string, error) {
	return s.repo.ListByStatus(ctx, status, pageSize, cursor)
}

func allowedTransition(from, to repository.DisputeStatus) bool {
	switch from {
	case repository.DisputeStatusOpen:
		return to == repository.DisputeStatusInvestigating || to == repository.DisputeStatusRejected
	case repository.DisputeStatusInvestigating:
		return to == repository.DisputeStatusInvestigating ||
			to == repository.DisputeStatusResolved ||
			to == repository.DisputeStatusRejected
	}
	return false
}

func rulingLabel(r engagementv1.DisputeRuling) string {
	if r == engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(r.String(), "DISPUTE_RULING_"))
}

func (s *DisputeService) ResolveDispute(
	ctx context.Context,
	actorID string,
	id string,
	status repository.DisputeStatus,
	ruling engagementv1.DisputeRuling,
	resolution string,
) (repository.Dispute, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return repository.Dispute{}, ErrEmptyDisputeID
	}

	switch status {
	case repository.DisputeStatusResolved, repository.DisputeStatusRejected, repository.DisputeStatusInvestigating:
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
	if !allowedTransition(existing.Status, status) {
		return repository.Dispute{}, ErrInvalidTransition
	}
	if ruling != engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED && status != repository.DisputeStatusResolved {
		return repository.Dispute{}, ErrRulingNotAllowed
	}
	if status == repository.DisputeStatusResolved && existing.ReturnID != "" &&
		ruling == engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED {
		return repository.Dispute{}, ErrRulingRequired
	}

	prev := existing.Status
	existing.Status = status
	existing.Resolution = strings.TrimSpace(resolution)
	existing.Ruling = ruling

	if s.audit == nil {
		return repository.Dispute{}, ErrAuditUnavailable
	}
	if err := s.audit.WriteEvent(ctx, upstream.AuditEvent{
		ActorID:    actorID,
		Action:     "dispute.resolve",
		TargetType: "dispute",
		TargetID:   id,
		Metadata: map[string]string{
			"resolution": existing.Resolution,
			"order_id":   existing.OrderID,
			"status":     strings.ToLower(string(status)),
			"ruling":     rulingLabel(ruling),
			"return_id":  existing.ReturnID,
		},
	}); err != nil {
		return repository.Dispute{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}

	if status == repository.DisputeStatusInvestigating {
		updated, err := s.repo.UpdateDispute(ctx, existing)
		if err != nil {
			return repository.Dispute{}, fmt.Errorf("update dispute: %w", err)
		}
		return updated, nil
	}

	factStatus := engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED
	if status == repository.DisputeStatusRejected {
		factStatus = engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED
	}
	updated, err := s.repo.ResolveWithEvent(ctx, existing, prev, &engagementv1.DisputeResolved{
		DisputeId:  id,
		OrderId:    existing.OrderID,
		ReturnId:   existing.ReturnID,
		Status:     factStatus,
		Ruling:     ruling,
		ResolverId: actorID,
		ResolvedAt: timestamppb.New(time.Now().UTC()),
	})
	if err != nil {
		return repository.Dispute{}, fmt.Errorf("resolve dispute: %w", err)
	}
	return updated, nil
}
