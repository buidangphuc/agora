// Package service holds team-verification's application logic. VerificationService
// owns the mock KYC lifecycle: a user submits a document reference (PENDING), a
// reviewer approves/rejects it (VERIFIED / REJECTED), and status lookups report
// the current state plus badge eligibility. It validates input and delegates
// persistence to the repository; identity always comes from the handler, never
// guessed. MOCK — only document references (strings) are stored, no documents.
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-verification/internal/repository"
)

// Validation errors surfaced to the handler, which maps them to gRPC codes.
var (
	ErrEmptyUser     = errors.New("user id is required")
	ErrEmptyDocType  = errors.New("doc_type is required")
	ErrEmptyDocRef   = errors.New("doc_ref is required")
	ErrEmptyID       = errors.New("submission id is required")
	ErrInvalidReview = errors.New("decision must be 'approve' or 'reject'")
)

// VerificationService is the KYC use-case boundary.
type VerificationService struct {
	repo repository.KycRepository
}

func NewVerificationService(repo repository.KycRepository) *VerificationService {
	return &VerificationService{repo: repo}
}

// Submit records a new KYC submission for the user in PENDING state. Mock: the
// doc_ref is stored verbatim (an opaque reference), no document is fetched.
func (s *VerificationService) Submit(ctx context.Context, userID, docType, docRef string) (*repository.Submission, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	if strings.TrimSpace(docType) == "" {
		return nil, ErrEmptyDocType
	}
	if strings.TrimSpace(docRef) == "" {
		return nil, ErrEmptyDocRef
	}
	sub := &repository.Submission{
		ID:      "kyc_" + uuid.NewString(),
		UserID:  userID,
		DocType: docType,
		DocRef:  docRef,
		Status:  repository.StatusPending,
	}
	return s.repo.Create(ctx, sub)
}

// GetStatus returns the user's current verification status and whether the
// verified badge should be shown. A user with no submission is reported as
// PENDING with no badge (never verified until they submit and are approved).
func (s *VerificationService) GetStatus(ctx context.Context, userID string) (repository.Status, bool, error) {
	if userID == "" {
		return "", false, ErrEmptyUser
	}
	sub, err := s.repo.GetLatestByUser(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.StatusPending, false, nil
	}
	if err != nil {
		return "", false, err
	}
	return sub.Status, badgeFor(sub.Status), nil
}

// Review applies a reviewer's decision to a submission. Mock admin action:
// "approve" -> VERIFIED, "reject" -> REJECTED, stamping reviewed_at. Returns the
// resulting status. NotFound propagates from the repository.
func (s *VerificationService) Review(ctx context.Context, id, decision string) (repository.Status, error) {
	if id == "" {
		return "", ErrEmptyID
	}
	var next repository.Status
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve", "approved", "verify", "verified":
		next = repository.StatusVerified
	case "reject", "rejected", "deny", "denied":
		next = repository.StatusRejected
	default:
		return "", ErrInvalidReview
	}
	sub, err := s.repo.UpdateStatus(ctx, id, next, time.Now().UTC())
	if err != nil {
		return "", err
	}
	return sub.Status, nil
}

// badgeFor reports whether the verified badge is earned. badge == VERIFIED.
func badgeFor(status repository.Status) bool {
	return status == repository.StatusVerified
}
