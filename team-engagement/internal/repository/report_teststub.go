package repository

// TEST-STUB: compile-only skeleton written by the test author so report_test.go
// builds and fails on behaviour. The implementer DELETES this file and puts the
// real code in report.go (same names and signatures).

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrReportNotFound = errors.New("report not found")

var errReportStub = errors.New("report repository not implemented")

type ReportStatus string

const (
	ReportStatusOpen      ReportStatus = "OPEN"
	ReportStatusActioned  ReportStatus = "ACTIONED"
	ReportStatusDismissed ReportStatus = "DISMISSED"
)

// ReportRedacted is the pseudonymised reporter_id value.
const ReportRedacted = "redacted"

type Report struct {
	ID             string
	TargetType     string
	TargetID       string
	ReasonCode     string
	Note           string
	ReporterID     string
	Status         ReportStatus
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	ResolvedBy     string
	ResolutionNote string
}

type ReportRepository interface {
	// CreateReport stores r as OPEN (id and created_at assigned by the repo when
	// empty) or returns the reporter's existing OPEN report for the same target.
	CreateReport(ctx context.Context, r Report) (Report, error)
	CountOpenByReporter(ctx context.Context, reporterID string) (int, error)
	GetReport(ctx context.Context, id string) (Report, error)
	// ListReports: status "" means any; targetID "" means any; oldest first
	// (created_at, id); returns the page and the total matching count.
	ListReports(ctx context.Context, status ReportStatus, targetID string, offset, limit int) ([]Report, int64, error)
	// CloseReport is a compare-and-set on status OPEN; false when already closed.
	CloseReport(ctx context.Context, id, adminID string, status ReportStatus, note string) (bool, error)
	// SweepClosedBefore pseudonymises at most batch closed, not yet redacted
	// reports with resolved_at < cutoff and returns how many it changed.
	SweepClosedBefore(ctx context.Context, cutoff time.Time, batch int) (int64, error)
	HasSellerListing(ctx context.Context, sellerID, listingID string) (bool, error)
}

type PostgresReportRepository struct{ pool *pgxpool.Pool }

func NewPostgresReportRepository(pool *pgxpool.Pool) *PostgresReportRepository {
	return &PostgresReportRepository{pool: pool}
}

type InMemoryReportRepository struct{}

func NewInMemoryReportRepository() *InMemoryReportRepository { return &InMemoryReportRepository{} }

// SeedReport inserts r verbatim (test seam, in-memory only).
func (r *InMemoryReportRepository) SeedReport(Report) {}

// SeedSellerListing records that sellerID owns listingID (test seam, in-memory only).
func (r *InMemoryReportRepository) SeedSellerListing(sellerID, listingID string) {}

func (*PostgresReportRepository) CreateReport(context.Context, Report) (Report, error) {
	return Report{}, errReportStub
}
func (*PostgresReportRepository) CountOpenByReporter(context.Context, string) (int, error) {
	return 0, errReportStub
}
func (*PostgresReportRepository) GetReport(context.Context, string) (Report, error) {
	return Report{}, errReportStub
}
func (*PostgresReportRepository) ListReports(context.Context, ReportStatus, string, int, int) ([]Report, int64, error) {
	return nil, 0, errReportStub
}
func (*PostgresReportRepository) CloseReport(context.Context, string, string, ReportStatus, string) (bool, error) {
	return false, errReportStub
}
func (*PostgresReportRepository) SweepClosedBefore(context.Context, time.Time, int) (int64, error) {
	return 0, errReportStub
}
func (*PostgresReportRepository) HasSellerListing(context.Context, string, string) (bool, error) {
	return false, errReportStub
}

func (*InMemoryReportRepository) CreateReport(context.Context, Report) (Report, error) {
	return Report{}, errReportStub
}
func (*InMemoryReportRepository) CountOpenByReporter(context.Context, string) (int, error) {
	return 0, errReportStub
}
func (*InMemoryReportRepository) GetReport(context.Context, string) (Report, error) {
	return Report{}, errReportStub
}
func (*InMemoryReportRepository) ListReports(context.Context, ReportStatus, string, int, int) ([]Report, int64, error) {
	return nil, 0, errReportStub
}
func (*InMemoryReportRepository) CloseReport(context.Context, string, string, ReportStatus, string) (bool, error) {
	return false, errReportStub
}
func (*InMemoryReportRepository) SweepClosedBefore(context.Context, time.Time, int) (int64, error) {
	return 0, errReportStub
}
func (*InMemoryReportRepository) HasSellerListing(context.Context, string, string) (bool, error) {
	return false, errReportStub
}

var (
	_ ReportRepository = (*PostgresReportRepository)(nil)
	_ ReportRepository = (*InMemoryReportRepository)(nil)
)
