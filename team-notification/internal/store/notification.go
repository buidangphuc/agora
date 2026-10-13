package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

// ErrNotFound is returned by MarkAsRead when no notification with that id
// belongs to the caller (it may not exist, or it may be another user's).
var ErrNotFound = errors.New("notification not found")

// NotificationRepository stores the in-app inbox. Every read and write is scoped
// to a user id.
type NotificationRepository interface {
	CreateNotification(ctx context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error)
	ListNotifications(ctx context.Context, userID string, limit, offset int) ([]*notificationv1.Notification, int32, error)
	MarkAsRead(ctx context.Context, id, userID string) error
	GetUnreadCount(ctx context.Context, userID string) (int32, error)
}

// PostgresNotificationRepo is the NotificationRepository over notifications.
type PostgresNotificationRepo struct {
	pool *pgxpool.Pool
}

// NewPostgresNotificationRepo returns the repository over pool.
func NewPostgresNotificationRepo(pool *pgxpool.Pool) *PostgresNotificationRepo {
	return &PostgresNotificationRepo{pool: pool}
}

// CreateNotification inserts n, assigning an id when it has none.
func (r *PostgresNotificationRepo) CreateNotification(ctx context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error) {
	if n.Id == "" {
		n.Id = "noti_" + uuid.NewString()[:8]
	}
	now := time.Now()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notifications (id, user_id, title, body, type, link_url, is_read, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, n.Id, n.UserId, n.Title, n.Body, int(n.Type), n.LinkUrl, n.IsRead, now)
	if err != nil {
		return nil, err
	}
	n.CreatedAt = timestamppb.New(now)
	return n, nil
}

// ListNotifications returns the user's notifications, newest first, and their unread count.
func (r *PostgresNotificationRepo) ListNotifications(ctx context.Context, userID string, limit, offset int) ([]*notificationv1.Notification, int32, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, body, type, link_url, is_read, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notis []*notificationv1.Notification
	for rows.Next() {
		var n notificationv1.Notification
		var t int32
		var createdAt time.Time
		if err := rows.Scan(&n.Id, &n.UserId, &n.Title, &n.Body, &t, &n.LinkUrl, &n.IsRead, &createdAt); err != nil {
			return nil, 0, err
		}
		n.Type = notificationv1.NotificationType(t)
		n.CreatedAt = timestamppb.New(createdAt)
		notis = append(notis, &n)
	}

	var unread int32
	_ = r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false", userID).Scan(&unread)

	return notis, unread, nil
}

// MarkAsRead marks the user's notification id read, or all of theirs when id is
// empty. A missing or foreign id returns ErrNotFound.
func (r *PostgresNotificationRepo) MarkAsRead(ctx context.Context, id, userID string) error {
	if id == "" {
		_, err := r.pool.Exec(ctx, "UPDATE notifications SET is_read = true WHERE user_id = $1", userID)
		return err
	}
	tag, err := r.pool.Exec(ctx, "UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetUnreadCount returns how many of the user's notifications are unread.
func (r *PostgresNotificationRepo) GetUnreadCount(ctx context.Context, userID string) (int32, error) {
	var count int32
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false", userID).Scan(&count)
	return count, err
}
