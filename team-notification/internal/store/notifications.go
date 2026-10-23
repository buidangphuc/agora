package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

type Notifications struct{ q Querier }

func NewNotifications(q Querier) *Notifications { return &Notifications{q: q} }

func (r *Notifications) CreateNotification(ctx context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error) {
	if n.Id == "" {
		n.Id = "noti_" + uuid.NewString()[:8]
	}
	now := time.Now()
	_, err := r.q.Exec(ctx, `
		INSERT INTO notifications (id, user_id, title, body, type, link_url, is_read, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, n.Id, n.UserId, n.Title, n.Body, int(n.Type), n.LinkUrl, n.IsRead, now)
	if err != nil {
		return nil, err
	}
	n.CreatedAt = timestamppb.New(now)
	return n, nil
}

func (r *Notifications) ListNotifications(ctx context.Context, userID string, limit, offset int) ([]*notificationv1.Notification, int32, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, user_id, title, body, type, link_url, is_read, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
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
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()

	var total int32
	if err := r.q.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id = $1", userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	return notis, total, nil
}

func (r *Notifications) MarkAsRead(ctx context.Context, id, userID string) error {
	if id == "" {
		_, err := r.q.Exec(ctx, "UPDATE notifications SET is_read = true WHERE user_id = $1", userID)
		return err
	}
	tag, err := r.q.Exec(ctx, "UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Notifications) GetUnreadCount(ctx context.Context, userID string) (int32, error) {
	var count int32
	err := r.q.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false", userID).Scan(&count)
	return count, err
}
