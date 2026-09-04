package repository

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrShareLinkNotFound is returned when a short code has no stored link.
var ErrShareLinkNotFound = errors.New("share link not found")

// ErrShortCodeExists is returned by Create when the short code is already taken,
// letting the service retry with a freshly generated code.
var ErrShortCodeExists = errors.New("short code already exists")

// ShareLink is one stored short link: the immutable target + UTM stamp + Open
// Graph preview metadata, plus a running resolve counter.
type ShareLink struct {
	ShortCode     string
	TargetType    string
	TargetID      string
	UTM           map[string]string
	OgTitle       string
	OgDescription string
	OgImageURL    string
	ClickCount    int64
	CreatedAt     time.Time
}

// ShareLinkRepository stores short links and resolves them. One row per short
// code; owned by team-sharing. Both a Postgres and an in-memory implementation
// satisfy this so the service is unit-testable without a live DB.
type ShareLinkRepository interface {
	// Create persists a new link. Returns ErrShortCodeExists if the short code
	// is already taken (the service retries with a new code).
	Create(ctx context.Context, link *ShareLink) (*ShareLink, error)
	// Resolve looks up a link by short code, atomically increments its
	// click_count, and returns the post-increment row. Returns
	// ErrShareLinkNotFound when the code is unknown.
	Resolve(ctx context.Context, shortCode string) (*ShareLink, error)
}

// ── Postgres implementation ──

type PostgresShareLinkRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresShareLinkRepo(pool *pgxpool.Pool) *PostgresShareLinkRepo {
	return &PostgresShareLinkRepo{pool: pool}
}

func (r *PostgresShareLinkRepo) Create(ctx context.Context, link *ShareLink) (*ShareLink, error) {
	raw, err := json.Marshal(nonNilUTM(link.UTM))
	if err != nil {
		return nil, err
	}
	const q = `
		INSERT INTO share_links
			(short_code, target_type, target_id, utm, og_title, og_description, og_image_url, click_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, NOW())
		ON CONFLICT (short_code) DO NOTHING
		RETURNING created_at`
	var createdAt time.Time
	err = r.pool.QueryRow(ctx, q,
		link.ShortCode, link.TargetType, link.TargetID, raw,
		link.OgTitle, link.OgDescription, link.OgImageURL,
	).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING returned no row → the code was taken.
		return nil, ErrShortCodeExists
	}
	if err != nil {
		return nil, err
	}
	stored := cloneLink(link)
	stored.ClickCount = 0
	stored.CreatedAt = createdAt
	return stored, nil
}

func (r *PostgresShareLinkRepo) Resolve(ctx context.Context, shortCode string) (*ShareLink, error) {
	const q = `
		UPDATE share_links
		SET click_count = click_count + 1
		WHERE short_code = $1
		RETURNING short_code, target_type, target_id, utm, og_title, og_description, og_image_url, click_count, created_at`
	var (
		link ShareLink
		raw  []byte
	)
	err := r.pool.QueryRow(ctx, q, shortCode).Scan(
		&link.ShortCode, &link.TargetType, &link.TargetID, &raw,
		&link.OgTitle, &link.OgDescription, &link.OgImageURL,
		&link.ClickCount, &link.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShareLinkNotFound
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

// ── In-memory implementation ──

type InMemoryShareLinkRepo struct {
	mu     sync.Mutex
	byCode map[string]*ShareLink
}

func NewInMemoryShareLinkRepo() *InMemoryShareLinkRepo {
	return &InMemoryShareLinkRepo{byCode: make(map[string]*ShareLink)}
}

func (r *InMemoryShareLinkRepo) Create(_ context.Context, link *ShareLink) (*ShareLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byCode[link.ShortCode]; ok {
		return nil, ErrShortCodeExists
	}
	stored := cloneLink(link)
	stored.ClickCount = 0
	stored.CreatedAt = time.Now().UTC()
	r.byCode[link.ShortCode] = stored
	return cloneLink(stored), nil
}

func (r *InMemoryShareLinkRepo) Resolve(_ context.Context, shortCode string) (*ShareLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.byCode[shortCode]
	if !ok {
		return nil, ErrShareLinkNotFound
	}
	stored.ClickCount++
	return cloneLink(stored), nil
}

func nonNilUTM(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func cloneLink(l *ShareLink) *ShareLink {
	utm := make(map[string]string, len(l.UTM))
	for k, v := range l.UTM {
		utm[k] = v
	}
	c := *l
	c.UTM = utm
	return &c
}
