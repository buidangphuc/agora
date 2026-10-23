package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

var (
	ErrInvalidReportTarget    = errors.New("target_id must be a listing uuid")
	ErrInvalidReportReason    = errors.New("invalid reason_code")
	ErrReportNoteTooLong      = errors.New("note must be at most 500 characters")
	ErrOwnListing             = errors.New("cannot report your own listing")
	ErrReportCapReached       = errors.New("too many open reports")
	ErrEmptyReportID          = errors.New("report_id is required")
	ErrInvalidReportStatus    = errors.New("status must be ACTIONED or DISMISSED")
	ErrResolutionNoteRequired = errors.New("resolution_note is required")
	ErrResolutionNoteTooLong  = errors.New("resolution_note must be at most 500 characters")
	ErrReportClosed           = errors.New("report is already closed")
)

const (
	maxReportNoteRunes = 500
	maxOpenReports     = 20
)

var validReportReasons = map[string]bool{
	"prohibited_item": true, "counterfeit": true, "scam": true,
	"misleading": true, "inappropriate": true, "other": true,
}

// ReportAuditWriter is the slice of upstream.AuditClient ReportService needs.
type ReportAuditWriter interface {
	WriteReportResolveEvent(ctx context.Context, actorID, reportID, listingID, status, resolutionNote string) error
}

type ReportService struct {
	repo   store.ReportRepository
	audit  ReportAuditWriter
	logger *slog.Logger
}

func NewReportService(repo store.ReportRepository, logger *slog.Logger) *ReportService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReportService{repo: repo, logger: logger}
}

// WithAudit wires the audit writer. Without one (or with a nil client),
// ResolveReport fails closed.
func (s *ReportService) WithAudit(w ReportAuditWriter) *ReportService {
	if c, ok := w.(*upstream.AuditClient); ok && c == nil {
		w = nil
	}
	s.audit = w
	return s
}

func (s *ReportService) CreateReport(ctx context.Context, reporterID, targetID, reasonCode, note string) (store.Report, error) {
	if _, err := uuid.Parse(targetID); err != nil {
		return store.Report{}, ErrInvalidReportTarget
	}
	if !validReportReasons[reasonCode] {
		return store.Report{}, ErrInvalidReportReason
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxReportNoteRunes {
		return store.Report{}, ErrReportNoteTooLong
	}
	own, err := s.repo.HasSellerListing(ctx, reporterID, targetID)
	if err != nil {
		return store.Report{}, fmt.Errorf("check listing owner: %w", err)
	}
	if own {
		return store.Report{}, ErrOwnListing
	}
	n, err := s.repo.CountOpenByReporter(ctx, reporterID)
	if err != nil {
		return store.Report{}, fmt.Errorf("count open reports: %w", err)
	}
	if n >= maxOpenReports {
		// A re-report of an already open pair is exempt from the cap.
		open, _, err := s.repo.ListReports(ctx, store.ReportStatusOpen, targetID, 0, 1000)
		if err != nil {
			return store.Report{}, fmt.Errorf("list open reports: %w", err)
		}
		for _, r := range open {
			if r.ReporterID == reporterID {
				return r, nil
			}
		}
		return store.Report{}, ErrReportCapReached
	}
	got, err := s.repo.CreateReport(ctx, store.Report{
		TargetType: "LISTING", TargetID: targetID, ReasonCode: reasonCode, Note: note, ReporterID: reporterID,
	})
	if err != nil {
		return store.Report{}, fmt.Errorf("create report: %w", err)
	}
	return got, nil
}

func (s *ReportService) ResolveReport(ctx context.Context, adminID, id string, status store.ReportStatus, note string) (store.Report, error) {
	if id == "" {
		return store.Report{}, ErrEmptyReportID
	}
	if status != store.ReportStatusActioned && status != store.ReportStatusDismissed {
		return store.Report{}, ErrInvalidReportStatus
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return store.Report{}, ErrResolutionNoteRequired
	}
	if utf8.RuneCountInString(note) > maxReportNoteRunes {
		return store.Report{}, ErrResolutionNoteTooLong
	}
	existing, err := s.repo.GetReport(ctx, id)
	if err != nil {
		return store.Report{}, err
	}
	if existing.Status == store.ReportStatusActioned || existing.Status == store.ReportStatusDismissed {
		return store.Report{}, ErrReportClosed
	}
	if s.audit == nil {
		return store.Report{}, ErrAuditUnavailable
	}
	if err := s.audit.WriteReportResolveEvent(ctx, adminID, id, existing.TargetID, string(status), note); err != nil {
		return store.Report{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	ok, err := s.repo.CloseReport(ctx, id, adminID, status, note)
	if err != nil {
		return store.Report{}, fmt.Errorf("close report: %w", err)
	}
	if !ok {
		return store.Report{}, ErrReportClosed
	}
	got, err := s.repo.GetReport(ctx, id)
	if err != nil {
		return store.Report{}, fmt.Errorf("reload report: %w", err)
	}
	return got, nil
}

// ListReports passes through to the repository (status "" and targetID "" mean any).
func (s *ReportService) ListReports(ctx context.Context, status store.ReportStatus, targetID string, offset, limit int) ([]store.Report, int64, error) {
	return s.repo.ListReports(ctx, status, targetID, offset, limit)
}
