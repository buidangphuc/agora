package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-sharing/internal/sharelink"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

func (r *Postgres) Create(ctx context.Context, link *sharelink.ShareLink) (*sharelink.ShareLink, error) {
	utm := link.UTM
	if utm == nil {
		utm = map[string]string{}
	}
	raw, err := json.Marshal(utm)
	if err != nil {
		return nil, err
	}
	const q = `
		INSERT INTO share_links
			(short_code, target_type, target_id, utm, og_title, og_description, og_image_url, created_by, click_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, NOW())
		ON CONFLICT (short_code) DO NOTHING
		RETURNING created_at`
	var createdAt time.Time
	err = r.pool.QueryRow(ctx, q,
		link.ShortCode, link.TargetType, link.TargetID, raw,
		link.OgTitle, link.OgDescription, link.OgImageURL, link.CreatedBy,
	).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, sharelink.ErrShortCodeExists
	}
	if err != nil {
		return nil, err
	}
	stored := *link
	stored.UTM = make(map[string]string, len(utm))
	for k, v := range utm {
		stored.UTM[k] = v
	}
	stored.ClickCount = 0
	stored.CreatedAt = createdAt
	return &stored, nil
}

func (r *Postgres) Resolve(ctx context.Context, shortCode string) (*sharelink.ShareLink, error) {
	const q = `
		UPDATE share_links
		SET click_count = click_count + 1
		WHERE short_code = $1
		RETURNING short_code, target_type, target_id, utm, og_title, og_description, og_image_url, created_by, click_count, created_at`
	var (
		link sharelink.ShareLink
		raw  []byte
	)
	err := r.pool.QueryRow(ctx, q, shortCode).Scan(
		&link.ShortCode, &link.TargetType, &link.TargetID, &raw,
		&link.OgTitle, &link.OgDescription, &link.OgImageURL, &link.CreatedBy,
		&link.ClickCount, &link.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, sharelink.ErrShareLinkNotFound
	}
	if err != nil {
		return nil, err
	}
	link.UTM = map[string]string{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &link.UTM); err != nil {
			return nil, err
		}
	}
	return &link, nil
}
