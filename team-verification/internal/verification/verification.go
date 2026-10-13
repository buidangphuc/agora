// Package verification holds team-verification's application logic. Service
// owns the mock KYC lifecycle: a user submits a document reference (PENDING), a
// reviewer approves/rejects it (VERIFIED / REJECTED), and status lookups report
// the current state plus badge eligibility. It validates input and delegates
// persistence to the store; identity always comes from the rpc layer, never
// guessed. MOCK — only document references (strings) are stored, no documents.
package verification

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-verification/internal/store"
)

// Validation errors, mapped to InvalidArgument by the rpc layer.
var (
	ErrEmptyUser     = errors.New("user id is required")
	ErrEmptyDocType  = errors.New("doc_type is required")
	ErrEmptyDocRef   = errors.New("doc_ref is required")
	ErrEmptyID       = errors.New("submission id is required")
	ErrInvalidReview = errors.New("decision must be 'approve' or 'reject'")
)

// Store errors re-exported so the rpc layer maps them without importing the
// store package directly.
var (
	ErrNotFound        = store.ErrNotFound
	ErrAlreadyReviewed = store.ErrAlreadyReviewed
)

// Service is the KYC use-case boundary.
type Service struct {
	repo store.KycRepository
}

// NewService returns a Service that keeps submissions in repo.
func NewService(repo store.KycRepository) *Service {
	return &Service{repo: repo}
}

// Submit records a new KYC submission for the user in PENDING state. Mock: the
// doc_ref is stored verbatim (an opaque reference), no document is fetched.
func (s *Service) Submit(ctx context.Context, userID, docType, docRef string) (*store.Submission, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	if strings.TrimSpace(docType) == "" {
		return nil, ErrEmptyDocType
	}
	if strings.TrimSpace(docRef) == "" {
		return nil, ErrEmptyDocRef
	}
	sub := &store.Submission{
		ID:      "kyc_" + uuid.NewString(),
		UserID:  userID,
		DocType: docType,
		DocRef:  docRef,
		Status:  store.StatusPending,
	}
	return s.repo.Create(ctx, sub)
}

// GetStatus returns the user's current verification status and whether the
// verified badge should be shown. A user with no submission is reported as
// PENDING with no badge (never verified until they submit and are approved).
func (s *Service) GetStatus(ctx context.Context, userID string) (store.Status, bool, error) {
	if userID == "" {
		return "", false, ErrEmptyUser
	}
	sub, err := s.repo.GetLatestByUser(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return store.StatusPending, false, nil
	}
	if err != nil {
		return "", false, err
	}
	return sub.Status, badgeFor(sub.Status), nil
}

// Get returns a submission by id (NotFound propagates from the store).
func (s *Service) Get(ctx context.Context, id string) (*store.Submission, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	return s.repo.GetByID(ctx, id)
}

// Review applies a reviewer's decision to a submission. Mock admin action:
// "approve" -> VERIFIED, "reject" -> REJECTED, stamping reviewed_at. Returns the
// resulting status. NotFound propagates from the store.
func (s *Service) Review(ctx context.Context, id, decision string) (store.Status, error) {
	if id == "" {
		return "", ErrEmptyID
	}
	var next store.Status
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve", "approved", "verify", "verified":
		next = store.StatusVerified
	case "reject", "rejected", "deny", "denied":
		next = store.StatusRejected
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
func badgeFor(status store.Status) bool {
	return status == store.StatusVerified
}
