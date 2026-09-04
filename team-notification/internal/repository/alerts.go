package repository

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

// ErrSubscriptionNotFound is returned when a subscription id does not exist for a
// user. Unsubscribe treats it as a no-op (idempotent), so callers usually ignore it.
var ErrSubscriptionNotFound = errors.New("alert subscription not found")

// AlertSubscriptionRepository stores a user's price-drop / back-in-stock alert
// subscriptions. Subscriptions are owned by team-notification (no cross-domain
// write). Both a Postgres and an in-memory implementation satisfy this.
type AlertSubscriptionRepository interface {
	// Create persists a subscription. It is idempotent on (user_id, listing_id,
	// type): subscribing to the same alert twice returns the existing row rather
	// than creating a duplicate.
	Create(ctx context.Context, sub *notificationv1.AlertSubscription) (*notificationv1.AlertSubscription, error)
	// Delete removes a subscription by id for a user. Deleting a missing/foreign
	// subscription is a no-op (idempotent unsubscribe) and returns nil.
	Delete(ctx context.Context, id, userID string) error
	// ListByUser returns all subscriptions owned by a user.
	ListByUser(ctx context.Context, userID string) ([]*notificationv1.AlertSubscription, error)
	// ListByListingAndType returns every subscription matching a listing and alert
	// type. The listing.events consumer uses it to fan a matched event out to all
	// subscribed users.
	ListByListingAndType(ctx context.Context, listingID string, alertType notificationv1.AlertType) ([]*notificationv1.AlertSubscription, error)
}

func newSubscriptionID() string { return "alert_" + uuid.NewString()[:8] }

// ── Postgres implementation ──

type PostgresAlertSubscriptionRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresAlertSubscriptionRepo(pool *pgxpool.Pool) *PostgresAlertSubscriptionRepo {
	return &PostgresAlertSubscriptionRepo{pool: pool}
}

func (r *PostgresAlertSubscriptionRepo) Create(ctx context.Context, sub *notificationv1.AlertSubscription) (*notificationv1.AlertSubscription, error) {
	if sub.Id == "" {
		sub.Id = newSubscriptionID()
	}
	// ON CONFLICT on the (user_id, listing_id, type) unique index makes Subscribe
	// idempotent; RETURNING id yields the existing row's id on a duplicate so the
	// caller always gets the canonical subscription.
	const q = `
		INSERT INTO alert_subscriptions (id, user_id, listing_id, type)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, listing_id, type)
		DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id`
	var id string
	if err := r.pool.QueryRow(ctx, q, sub.Id, sub.UserId, sub.ListingId, int(sub.Type)).Scan(&id); err != nil {
		return nil, err
	}
	sub.Id = id
	return sub, nil
}

func (r *PostgresAlertSubscriptionRepo) Delete(ctx context.Context, id, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM alert_subscriptions WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (r *PostgresAlertSubscriptionRepo) ListByUser(ctx context.Context, userID string) ([]*notificationv1.AlertSubscription, error) {
	rows, err := r.pool.Query(ctx, `
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

func (r *PostgresAlertSubscriptionRepo) ListByListingAndType(ctx context.Context, listingID string, alertType notificationv1.AlertType) ([]*notificationv1.AlertSubscription, error) {
	rows, err := r.pool.Query(ctx, `
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
		var t int
		if err := rows.Scan(&s.Id, &s.UserId, &s.ListingId, &t); err != nil {
			return nil, err
		}
		s.Type = notificationv1.AlertType(t)
		subs = append(subs, &s)
	}
	return subs, rows.Err()
}

// ── In-memory implementation ──

type InMemoryAlertSubscriptionRepo struct {
	mu   sync.Mutex
	byID map[string]*notificationv1.AlertSubscription
}

func NewInMemoryAlertSubscriptionRepo() *InMemoryAlertSubscriptionRepo {
	return &InMemoryAlertSubscriptionRepo{byID: make(map[string]*notificationv1.AlertSubscription)}
}

func (r *InMemoryAlertSubscriptionRepo) Create(_ context.Context, sub *notificationv1.AlertSubscription) (*notificationv1.AlertSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Idempotent on (user_id, listing_id, type): return the existing row if present.
	for _, s := range r.byID {
		if s.UserId == sub.UserId && s.ListingId == sub.ListingId && s.Type == sub.Type {
			return cloneSub(s), nil
		}
	}
	if sub.Id == "" {
		sub.Id = newSubscriptionID()
	}
	stored := cloneSub(sub)
	r.byID[stored.Id] = stored
	return cloneSub(stored), nil
}

func (r *InMemoryAlertSubscriptionRepo) Delete(_ context.Context, id, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byID[id]; ok && s.UserId == userID {
		delete(r.byID, id)
	}
	// Missing/foreign id → no-op (idempotent).
	return nil
}

func (r *InMemoryAlertSubscriptionRepo) ListByUser(_ context.Context, userID string) ([]*notificationv1.AlertSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*notificationv1.AlertSubscription
	for _, s := range r.byID {
		if s.UserId == userID {
			out = append(out, cloneSub(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out, nil
}

func (r *InMemoryAlertSubscriptionRepo) ListByListingAndType(_ context.Context, listingID string, alertType notificationv1.AlertType) ([]*notificationv1.AlertSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*notificationv1.AlertSubscription
	for _, s := range r.byID {
		if s.ListingId == listingID && s.Type == alertType {
			out = append(out, cloneSub(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out, nil
}

func cloneSub(s *notificationv1.AlertSubscription) *notificationv1.AlertSubscription {
	return &notificationv1.AlertSubscription{
		Id:        s.Id,
		UserId:    s.UserId,
		ListingId: s.ListingId,
		Type:      s.Type,
	}
}
