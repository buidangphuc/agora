package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrReportNotFound = errors.New("report not found")

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

const reportCols = `id, target_type, target_id, reason_code, note, reporter_id, status,
	created_at, resolved_at, resolved_by, resolution_note`

type reportScanner interface{ Scan(dest ...any) error }

func scanReport(s reportScanner) (Report, error) {
	var r Report
	var st string
	if err := s.Scan(&r.ID, &r.TargetType, &r.TargetID, &r.ReasonCode, &r.Note, &r.ReporterID, &st,
		&r.CreatedAt, &r.ResolvedAt, &r.ResolvedBy, &r.ResolutionNote); err != nil {
		return Report{}, err
	}
	r.Status = ReportStatus(st)
	return r, nil
}

// ---- Postgres ----

type PostgresReportRepository struct{ pool *pgxpool.Pool }

func NewPostgresReportRepository(pool *pgxpool.Pool) *PostgresReportRepository {
	return &PostgresReportRepository{pool: pool}
}

func (p *PostgresReportRepository) CreateReport(ctx context.Context, r Report) (Report, error) {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	for attempt := 0; attempt < 5; attempt++ {
		var created *time.Time
		if !r.CreatedAt.IsZero() {
			created = &r.CreatedAt
		}
		row := p.pool.QueryRow(ctx, `INSERT INTO reports
			(id, target_type, target_id, reason_code, note, reporter_id, status, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,'OPEN', COALESCE($7, now()))
			ON CONFLICT (reporter_id, target_type, target_id) WHERE status = 'OPEN' DO NOTHING
			RETURNING `+reportCols,
			r.ID, r.TargetType, r.TargetID, r.ReasonCode, r.Note, r.ReporterID, created)
		got, err := scanReport(row)
		if err == nil {
			return got, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Report{}, err
		}
		// Conflict: return the existing OPEN report (retry if it closed meanwhile).
		got, err = scanReport(p.pool.QueryRow(ctx, `SELECT `+reportCols+` FROM reports
			WHERE reporter_id=$1 AND target_type=$2 AND target_id=$3 AND status='OPEN'`,
			r.ReporterID, r.TargetType, r.TargetID))
		if err == nil {
			return got, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Report{}, err
		}
	}
	return Report{}, errors.New("create report: contention retry limit")
}

func (p *PostgresReportRepository) CountOpenByReporter(ctx context.Context, reporterID string) (int, error) {
	var n int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM reports WHERE reporter_id=$1 AND status='OPEN'`, reporterID).Scan(&n)
	return n, err
}

func (p *PostgresReportRepository) GetReport(ctx context.Context, id string) (Report, error) {
	r, err := scanReport(p.pool.QueryRow(ctx, `SELECT `+reportCols+` FROM reports WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrReportNotFound
	}
	return r, err
}

func (p *PostgresReportRepository) ListReports(ctx context.Context, status ReportStatus, targetID string, offset, limit int) ([]Report, int64, error) {
	var total int64
	if err := p.pool.QueryRow(ctx, `SELECT count(*) FROM reports
		WHERE ($1='' OR status=$1) AND ($2='' OR target_id=$2)`, string(status), targetID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := p.pool.Query(ctx, `SELECT `+reportCols+` FROM reports
		WHERE ($1='' OR status=$1) AND ($2='' OR target_id=$2)
		ORDER BY created_at, id OFFSET $3 LIMIT $4`, string(status), targetID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (p *PostgresReportRepository) CloseReport(ctx context.Context, id, adminID string, status ReportStatus, note string) (bool, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE reports SET status=$2, resolved_at=now(), resolved_by=$3, resolution_note=$4
		WHERE id=$1 AND status='OPEN'`, id, string(status), adminID, note)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reports WHERE id=$1)`, id).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, ErrReportNotFound
	}
	return false, nil
}

func (p *PostgresReportRepository) SweepClosedBefore(ctx context.Context, cutoff time.Time, batch int) (int64, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE reports SET reporter_id=$1, note=''
		WHERE id IN (SELECT id FROM reports
			WHERE status <> 'OPEN' AND resolved_at < $2 AND (reporter_id <> $1 OR note <> '')
			ORDER BY resolved_at, id LIMIT $3)`, ReportRedacted, cutoff, batch)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (p *PostgresReportRepository) HasSellerListing(ctx context.Context, sellerID, listingID string) (bool, error) {
	var ok bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM seller_listings WHERE seller_id=$1 AND listing_id=$2)`,
		sellerID, listingID).Scan(&ok)
	return ok, err
}

var _ ReportRepository = (*PostgresReportRepository)(nil)
