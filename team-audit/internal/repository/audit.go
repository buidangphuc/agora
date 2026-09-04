// Package repository holds team-audit's persistence layer. Audit events are
// append-only and immutable: the interface deliberately exposes no update or
// delete. Both a Postgres and an in-memory implementation satisfy it, so the
// service and its tests run identically with or without a live database.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
)

// QueryFilter narrows an audit-log query. An empty ActorID or TargetType means
// "do not filter on that field". Limit/Offset drive newest-first pagination.
type QueryFilter struct {
	ActorID    string
	TargetType string
	Limit      int
	Offset     int
}

// AuditRepository stores immutable audit events. Append-only: there is no update
// or delete — an audit trail you can rewrite is not an audit trail.
type AuditRepository interface {
	// Append persists a new event, assigning id + created_at when unset, and
	// returns the stored event.
	Append(ctx context.Context, ev *auditv1.AuditEvent) (*auditv1.AuditEvent, error)
	// Query returns events matching the filter, newest first, together with the
	// total count of matching events (ignoring Limit/Offset) so callers can page.
	Query(ctx context.Context, f QueryFilter) ([]*auditv1.AuditEvent, int64, error)
}

func newEventID() string { return "audit_" + uuid.NewString()[:8] }

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	return limit
}

// ── Postgres implementation ──

type PostgresAuditRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresAuditRepo(pool *pgxpool.Pool) *PostgresAuditRepo {
	return &PostgresAuditRepo{pool: pool}
}

func (r *PostgresAuditRepo) Append(ctx context.Context, ev *auditv1.AuditEvent) (*auditv1.AuditEvent, error) {
	if ev.Id == "" {
		ev.Id = newEventID()
	}
	raw, err := json.Marshal(nonNilMeta(ev.GetMetadata()))
	if err != nil {
		return nil, err
	}
	// No ON CONFLICT / RETURNING-on-update: audit_events is append-only, so every
	// call inserts a fresh immutable row. The server owns created_at.
	const q = `
		INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING created_at`
	var created time.Time
	if err := r.pool.QueryRow(ctx, q, ev.Id, ev.ActorId, ev.Action, ev.TargetType, ev.TargetId, raw).Scan(&created); err != nil {
		return nil, err
	}
	ev.CreatedAt = timestamppb.New(created)
	return ev, nil
}

func (r *PostgresAuditRepo) Query(ctx context.Context, f QueryFilter) ([]*auditv1.AuditEvent, int64, error) {
	where := "WHERE 1=1"
	args := []any{}
	if f.ActorID != "" {
		args = append(args, f.ActorID)
		where += fmt.Sprintf(" AND actor_id = $%d", len(args))
	}
	if f.TargetType != "" {
		args = append(args, f.TargetType)
		where += fmt.Sprintf(" AND target_type = $%d", len(args))
	}

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := normalizeLimit(f.Limit)
	pageArgs := append(append([]any{}, args...), limit, f.Offset)
	// Newest-first, with id as a stable tiebreaker for events sharing a timestamp.
	q := "SELECT id, actor_id, action, target_type, target_id, metadata, created_at FROM audit_events " +
		where + fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	rows, err := r.pool.Query(ctx, q, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []*auditv1.AuditEvent
	for rows.Next() {
		var (
			ev      auditv1.AuditEvent
			raw     []byte
			created time.Time
		)
		if err := rows.Scan(&ev.Id, &ev.ActorId, &ev.Action, &ev.TargetType, &ev.TargetId, &raw, &created); err != nil {
			return nil, 0, err
		}
		meta := map[string]string{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &meta); err != nil {
				return nil, 0, err
			}
		}
		ev.Metadata = meta
		ev.CreatedAt = timestamppb.New(created)
		events = append(events, &ev)
	}
	return events, total, rows.Err()
}

// ── In-memory implementation ──

// InMemoryAuditRepo keeps events in insertion order. Newest-first ordering is
// derived by walking the slice in reverse, so tests get deterministic paging
// without depending on wall-clock timestamp resolution.
type InMemoryAuditRepo struct {
	mu     sync.Mutex
	events []*auditv1.AuditEvent
}

func NewInMemoryAuditRepo() *InMemoryAuditRepo {
	return &InMemoryAuditRepo{}
}

func (r *InMemoryAuditRepo) Append(_ context.Context, ev *auditv1.AuditEvent) (*auditv1.AuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ev.Id == "" {
		ev.Id = newEventID()
	}
	if ev.CreatedAt == nil {
		ev.CreatedAt = timestamppb.Now()
	}
	stored := cloneEvent(ev)
	r.events = append(r.events, stored)
	return cloneEvent(stored), nil
}

func (r *InMemoryAuditRepo) Query(_ context.Context, f QueryFilter) ([]*auditv1.AuditEvent, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Collect matches newest-first (reverse insertion order).
	var matched []*auditv1.AuditEvent
	for i := len(r.events) - 1; i >= 0; i-- {
		ev := r.events[i]
		if f.ActorID != "" && ev.ActorId != f.ActorID {
			continue
		}
		if f.TargetType != "" && ev.TargetType != f.TargetType {
			continue
		}
		matched = append(matched, ev)
	}
	total := int64(len(matched))

	// `matched` is already newest-first (reverse insertion order), which for an
	// append-only log is chronological order — deterministic even when several
	// events share a wall-clock instant. The Postgres path gets the same result
	// from ORDER BY created_at DESC, id DESC.

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(matched) {
		return nil, total, nil
	}
	end := offset + normalizeLimit(f.Limit)
	if end > len(matched) {
		end = len(matched)
	}
	page := make([]*auditv1.AuditEvent, 0, end-offset)
	for _, ev := range matched[offset:end] {
		page = append(page, cloneEvent(ev))
	}
	return page, total, nil
}

func nonNilMeta(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func cloneEvent(ev *auditv1.AuditEvent) *auditv1.AuditEvent {
	meta := make(map[string]string, len(ev.GetMetadata()))
	for k, v := range ev.GetMetadata() {
		meta[k] = v
	}
	return &auditv1.AuditEvent{
		Id:         ev.Id,
		ActorId:    ev.ActorId,
		Action:     ev.Action,
		TargetType: ev.TargetType,
		TargetId:   ev.TargetId,
		Metadata:   meta,
		CreatedAt:  ev.CreatedAt,
	}
}
