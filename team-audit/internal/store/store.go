package store

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-audit/internal/audit"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

func NewEventID() string { return "audit_" + uuid.NewString()[:8] }

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

func (r *Postgres) Append(ctx context.Context, ev audit.Event) (audit.Event, error) {
	if ev.ID == "" {
		ev.ID = NewEventID()
	}
	meta := ev.Metadata
	if meta == nil {
		meta = map[string]string{}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return audit.Event{}, err
	}
	const q = `
		INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING created_at`
	var created time.Time
	if err := r.pool.QueryRow(ctx, q, ev.ID, ev.ActorID, ev.Action, ev.TargetType, ev.TargetID, raw).Scan(&created); err != nil {
		return audit.Event{}, err
	}
	ev.Metadata = meta
	ev.CreatedAt = created
	return ev, nil
}

func (r *Postgres) Query(ctx context.Context, f audit.Filter) ([]audit.Event, int64, error) {
	where := "WHERE 1=1"
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if f.ActorID != "" {
		add("actor_id = $%d", f.ActorID)
	}
	if f.TargetType != "" {
		add("target_type = $%d", f.TargetType)
	}
	if f.TargetID != "" {
		add("target_id = $%d", f.TargetID)
	}
	if f.Action != "" {
		add("action = $%d", f.Action)
	}
	if !f.CreatedAfter.IsZero() {
		add("created_at >= $%d", f.CreatedAfter)
	}
	if !f.CreatedBefore.IsZero() {
		add("created_at < $%d", f.CreatedBefore)
	}

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = audit.DefaultPageSize
	}
	offset := max(f.Offset, 0)
	pageArgs := append(append([]any{}, args...), limit, offset)
	q := "SELECT id, actor_id, action, target_type, target_id, metadata, created_at FROM audit_events " +
		where + fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	rows, err := r.pool.Query(ctx, q, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := []audit.Event{}
	for rows.Next() {
		var (
			ev  audit.Event
			raw []byte
		)
		if err := rows.Scan(&ev.ID, &ev.ActorID, &ev.Action, &ev.TargetType, &ev.TargetID, &raw, &ev.CreatedAt); err != nil {
			return nil, 0, err
		}
		ev.Metadata = map[string]string{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &ev.Metadata); err != nil {
				return nil, 0, err
			}
		}
		events = append(events, ev)
	}
	return events, total, rows.Err()
}
