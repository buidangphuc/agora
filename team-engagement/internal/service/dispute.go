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
	"github.com/buidangphuc/team-engagement/internal/store"
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
	repo   store.DisputeRepository
	audit  AuditWriter
	logger *slog.Logger
}

// WithAudit wires the audit writer. Without one, ResolveDispute fails closed.
func (s *DisputeService) WithAudit(w AuditWriter) *DisputeService {
	s.audit = w
	return s
}

func NewDisputeService(repo store.DisputeRepository, logger *slog.Logger) *DisputeService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DisputeService{repo: repo, logger: logger}
}

func (s *DisputeService) CreateDispute(
	ctx context.Context,
	orderID, claimantID, defendantID, returnID, reason string,
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
		ReturnID:     strings.TrimSpace(returnID),
		EvidenceURLs: cleanedURLs,
		Status:       store.DisputeStatusOpen,
	}

	saved, err := s.repo.CreateDispute(ctx, d)
	if err != nil {
		return store.Dispute{}, fmt.Errorf("create dispute: %w", err)
	}
	return saved, nil
}

func (s *DisputeService) GetDispute(ctx context.Context, id string) (store.Dispute, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return store.Dispute{}, ErrEmptyDisputeID
	}
	return s.repo.GetDispute(ctx, id)
}

func (s *DisputeService) ListOrderDisputes(ctx context.Context, orderID string) ([]store.Dispute, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, ErrEmptyOrderID
	}
	return s.repo.ListByOrder(ctx, orderID)
}

func (s *DisputeService) ListDisputes(
	ctx context.Context,
	status store.DisputeStatus,
	pageSize int,
	cursor string,
) ([]store.Dispute, string, error) {
	return s.repo.ListByStatus(ctx, status, pageSize, cursor)
}

func allowedTransition(from, to store.DisputeStatus) bool {
	switch from {
	case store.DisputeStatusOpen:
		return to == store.DisputeStatusInvestigating || to == store.DisputeStatusRejected
	case store.DisputeStatusInvestigating:
		return to == store.DisputeStatusInvestigating ||
			to == store.DisputeStatusResolved ||
			to == store.DisputeStatusRejected
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
	status store.DisputeStatus,
	ruling engagementv1.DisputeRuling,
	resolution string,
) (store.Dispute, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return store.Dispute{}, ErrEmptyDisputeID
	}

	switch status {
	case store.DisputeStatusResolved, store.DisputeStatusRejected, store.DisputeStatusInvestigating:
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
	if !allowedTransition(existing.Status, status) {
		return store.Dispute{}, ErrInvalidTransition
	}
	if ruling != engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED && status != store.DisputeStatusResolved {
		return store.Dispute{}, ErrRulingNotAllowed
	}
	if status == store.DisputeStatusResolved && existing.ReturnID != "" &&
		ruling == engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED {
		return store.Dispute{}, ErrRulingRequired
	}

	prev := existing.Status
	existing.Status = status
	existing.Resolution = strings.TrimSpace(resolution)
	existing.Ruling = ruling

	if s.audit == nil {
		return store.Dispute{}, ErrAuditUnavailable
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
		return store.Dispute{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}

	if status == store.DisputeStatusInvestigating {
		updated, err := s.repo.UpdateDispute(ctx, existing)
		if err != nil {
			return store.Dispute{}, fmt.Errorf("update dispute: %w", err)
		}
		return updated, nil
	}

	factStatus := engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED
	if status == store.DisputeStatusRejected {
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
		return store.Dispute{}, fmt.Errorf("resolve dispute: %w", err)
	}
	return updated, nil
}
