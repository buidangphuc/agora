package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

type Prefs struct{ q Querier }

func NewPrefs(q Querier) *Prefs { return &Prefs{q: q} }

func (r *Prefs) Get(ctx context.Context, userID string) (*notificationv1.NotificationPrefs, error) {
	var raw []byte
	var freq int
	err := r.q.QueryRow(ctx, `
		SELECT prefs, digest_freq
		FROM notification_prefs
		WHERE user_id = $1`, userID).Scan(&raw, &freq)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPrefsNotFound
	}
	if err != nil {
		return nil, err
	}
	typeEnabled := map[string]bool{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &typeEnabled); err != nil {
			return nil, err
		}
	}
	return &notificationv1.NotificationPrefs{
		TypeEnabled: typeEnabled,
		DigestFreq:  notificationv1.DigestFrequency(freq),
	}, nil
}

func (r *Prefs) Upsert(ctx context.Context, userID string, prefs *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error) {
	raw, err := json.Marshal(prefs.GetTypeEnabled())
	if err != nil {
		return nil, err
	}
	const q = `
		INSERT INTO notification_prefs (user_id, prefs, digest_freq, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET prefs = EXCLUDED.prefs, digest_freq = EXCLUDED.digest_freq, updated_at = NOW()`
	if _, err := r.q.Exec(ctx, q, userID, raw, int(prefs.GetDigestFreq())); err != nil {
		return nil, err
	}
	te := make(map[string]bool, len(prefs.GetTypeEnabled()))
	for k, v := range prefs.GetTypeEnabled() {
		te[k] = v
	}
	return &notificationv1.NotificationPrefs{TypeEnabled: te, DigestFreq: prefs.GetDigestFreq()}, nil
}

func (r *Prefs) ListUsersByDigestFreq(ctx context.Context, freq notificationv1.DigestFrequency) ([]string, error) {
	rows, err := r.q.Query(ctx, `
		SELECT user_id
		FROM notification_prefs
		WHERE digest_freq = $1
		ORDER BY user_id`, int(freq))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
