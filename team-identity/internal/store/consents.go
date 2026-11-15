package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ConsentInput struct {
	Purpose string
	Granted bool
	Version string
	Source  string
}

type Consent struct {
	ID         int64
	UserID     string
	Purpose    string
	Granted    bool
	Version    string
	Source     string
	RecordedAt time.Time
}

const insertConsentSQL = `INSERT INTO consents (user_id, purpose, granted, version, source) VALUES ($1, $2, $3, $4, $5)`

func insertConsents(ctx context.Context, tx pgx.Tx, userID string, in []ConsentInput) error {
	for _, c := range in {
		if _, err := tx.Exec(ctx, insertConsentSQL, userID, c.Purpose, c.Granted, c.Version, c.Source); err != nil {
			return fmt.Errorf("insert consent %q: %w", c.Purpose, err)
		}
	}
	return nil
}

func (r *PostgresUserRepository) AppendConsents(ctx context.Context, userID string, in []ConsentInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := insertConsents(ctx, tx, userID, in); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func encodeConsentCursor(c Consent) string {
	raw := strconv.FormatInt(c.RecordedAt.UnixMicro(), 10) + ":" + strconv.FormatInt(c.ID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeConsentCursor(cursor string) (time.Time, int64, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor: %w", err)
	}
	ts, id, ok := strings.Cut(string(b), ":")
	if !ok {
		return time.Time{}, 0, fmt.Errorf("invalid cursor")
	}
	micro, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor: %w", err)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor: %w", err)
	}
	return time.UnixMicro(micro), n, nil
}

func (r *PostgresUserRepository) ListConsents(ctx context.Context, userID, cursor string, limit int) ([]Consent, string, error) {
	if limit <= 0 {
		limit = 20
	}
	const base = `SELECT id, user_id, purpose, granted, version, source, recorded_at FROM consents WHERE user_id = $1`
	const tail = ` ORDER BY recorded_at DESC, id DESC LIMIT `
	var rows pgx.Rows
	var err error
	if cursor == "" {
		rows, err = r.pool.Query(ctx, base+tail+strconv.Itoa(limit+1), userID)
	} else {
		at, id, derr := decodeConsentCursor(cursor)
		if derr != nil {
			return nil, "", derr
		}
		rows, err = r.pool.Query(ctx, base+` AND (recorded_at, id) < ($2, $3)`+tail+strconv.Itoa(limit+1), userID, at, id)
	}
	if err != nil {
		return nil, "", fmt.Errorf("list consents: %w", err)
	}
	defer rows.Close()
	var out []Consent
	for rows.Next() {
		var c Consent
		if err := rows.Scan(&c.ID, &c.UserID, &c.Purpose, &c.Granted, &c.Version, &c.Source, &c.RecordedAt); err != nil {
			return nil, "", fmt.Errorf("scan consent: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("list consents: %w", err)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = encodeConsentCursor(out[limit-1])
	}
	return out, next, nil
}
