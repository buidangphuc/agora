package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

type NotificationRepository interface {
	CreateNotification(ctx context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error)
	ListNotifications(ctx context.Context, userID string, limit, offset int) ([]*notificationv1.Notification, int32, error)
	MarkAsRead(ctx context.Context, id, userID string) error
	GetUnreadCount(ctx context.Context, userID string) (int32, error)
}

type PostgresNotificationRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresNotificationRepo(pool *pgxpool.Pool) *PostgresNotificationRepo {
	return &PostgresNotificationRepo{pool: pool}
}

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
		var t int
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

func (r *PostgresNotificationRepo) MarkAsRead(ctx context.Context, id, userID string) error {
	if id == "" {
		_, err := r.pool.Exec(ctx, "UPDATE notifications SET is_read = true WHERE user_id = $1", userID)
		return err
	}
	_, err := r.pool.Exec(ctx, "UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2", id, userID)
	return err
}

func (r *PostgresNotificationRepo) GetUnreadCount(ctx context.Context, userID string) (int32, error) {
	var count int32
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false", userID).Scan(&count)
	return count, err
}
