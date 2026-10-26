package verification

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-verification/internal/store"
)

var (
	ErrEmptyUser     = errors.New("user id is required")
	ErrEmptyDocType  = errors.New("doc_type is required")
	ErrEmptyDocRef   = errors.New("doc_ref is required")
	ErrEmptyID       = errors.New("submission id is required")
	ErrInvalidReview = errors.New("decision must be 'approve' or 'reject'")
)

type VerificationService struct {
	repo store.KycRepository
}

func NewVerificationService(repo store.KycRepository) *VerificationService {
	return &VerificationService{repo: repo}
}

func (s *VerificationService) Submit(ctx context.Context, userID, docType, docRef string) (*store.Submission, error) {
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

func (s *VerificationService) GetStatus(ctx context.Context, userID string) (store.Status, bool, error) {
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

func (s *VerificationService) Get(ctx context.Context, id string) (*store.Submission, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	return s.repo.GetByID(ctx, id)
}

func NormalizeDecision(decision string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve", "approved", "verify", "verified":
		return "approve", nil
	case "reject", "rejected", "deny", "denied":
		return "reject", nil
	default:
		return "", ErrInvalidReview
	}
}

func (s *VerificationService) Review(ctx context.Context, id, decision string) (store.Status, error) {
	if id == "" {
		return "", ErrEmptyID
	}
	norm, err := NormalizeDecision(decision)
	if err != nil {
		return "", err
	}
	next := store.StatusRejected
	if norm == "approve" {
		next = store.StatusVerified
	}
	sub, err := s.repo.UpdateStatus(ctx, id, next, time.Now().UTC())
	if err != nil {
		return "", err
	}
	return sub.Status, nil
}

func badgeFor(status store.Status) bool {
	return status == store.StatusVerified
}
