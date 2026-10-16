package service

// TEST-STUB (task 3, red): compile-only placeholders so report_test.go builds.
// The implementer deletes this file and provides report.go with the real service.

import (
	"context"
	"errors"
	"log/slog"

	"github.com/buidangphuc/team-engagement/internal/repository"
)

var (
	ErrInvalidReportTarget      = errors.New("target_id must be a listing uuid")
	ErrInvalidReportReason      = errors.New("invalid reason_code")
	ErrReportNoteTooLong        = errors.New("note must be at most 500 characters")
	ErrOwnListing               = errors.New("cannot report your own listing")
	ErrReportCapReached         = errors.New("too many open reports")
	ErrEmptyReportID            = errors.New("report_id is required")
	ErrInvalidReportStatus      = errors.New("status must be ACTIONED or DISMISSED")
	ErrResolutionNoteRequired   = errors.New("resolution_note is required")
	ErrResolutionNoteTooLong    = errors.New("resolution_note must be at most 500 characters")
	ErrReportClosed             = errors.New("report is already closed")
	errReportStubNotImplemented = errors.New("report service not implemented")
)

type ReportService struct{}

func NewReportService(_ repository.ReportRepository, _ *slog.Logger) *ReportService {
	return &ReportService{}
}

func (s *ReportService) WithAudit(_ AuditWriter) *ReportService { return s }

func (s *ReportService) CreateReport(_ context.Context, reporterID, targetID, reasonCode, note string) (repository.Report, error) {
	return repository.Report{}, errReportStubNotImplemented
}

func (s *ReportService) ResolveReport(_ context.Context, adminID, id string, status repository.ReportStatus, note string) (repository.Report, error) {
	return repository.Report{}, errReportStubNotImplemented
}
