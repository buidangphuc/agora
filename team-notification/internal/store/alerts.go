package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

type Alerts struct{ q Querier }

func NewAlerts(q Querier) *Alerts { return &Alerts{q: q} }

func (r *Alerts) Create(ctx context.Context, sub *notificationv1.AlertSubscription) (*notificationv1.AlertSubscription, error) {
	if sub.Id == "" {
		sub.Id = "alert_" + uuid.NewString()[:8]
	}
	const q = `
		INSERT INTO alert_subscriptions (id, user_id, listing_id, type)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, listing_id, type)
		DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id`
	var id string
	if err := r.q.QueryRow(ctx, q, sub.Id, sub.UserId, sub.ListingId, int(sub.Type)).Scan(&id); err != nil {
		return nil, err
	}
	sub.Id = id
	return sub, nil
}

func (r *Alerts) Delete(ctx context.Context, id, userID string) error {
	_, err := r.q.Exec(ctx, `DELETE FROM alert_subscriptions WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (r *Alerts) ListByUser(ctx context.Context, userID string) ([]*notificationv1.AlertSubscription, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, user_id, listing_id, type
		FROM alert_subscriptions
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func (r *Alerts) ListByListingAndType(ctx context.Context, listingID string, alertType notificationv1.AlertType) ([]*notificationv1.AlertSubscription, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, user_id, listing_id, type
		FROM alert_subscriptions
		WHERE listing_id = $1 AND type = $2`, listingID, int(alertType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func scanSubscriptions(rows pgx.Rows) ([]*notificationv1.AlertSubscription, error) {
	var subs []*notificationv1.AlertSubscription
	for rows.Next() {
		var s notificationv1.AlertSubscription
		var t int32
		if err := rows.Scan(&s.Id, &s.UserId, &s.ListingId, &t); err != nil {
			return nil, err
		}
		s.Type = notificationv1.AlertType(t)
		subs = append(subs, &s)
	}
	return subs, rows.Err()
}
