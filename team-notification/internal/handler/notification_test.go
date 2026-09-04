package handler_test

import (
	"context"
	"testing"
	"time"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/handler"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockRepo struct{}

func (m *mockRepo) CreateNotification(ctx context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error) {
	return n, nil
}

func (m *mockRepo) ListNotifications(ctx context.Context, userID string, limit, offset int) ([]*notificationv1.Notification, int32, error) {
	return []*notificationv1.Notification{
		{
			Id:        "noti_1",
			UserId:    userID,
			Title:     "Đơn hàng đang giao",
			Body:      "Đơn hàng iPhone 15 Pro Max đang được SPX Express vận chuyển",
			Type:      notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER,
			IsRead:    false,
			CreatedAt: timestamppb.New(time.Now()),
		},
	}, 1, nil
}

func (m *mockRepo) MarkAsRead(ctx context.Context, id, userID string) error {
	return nil
}

func (m *mockRepo) GetUnreadCount(ctx context.Context, userID string) (int32, error) {
	return 1, nil
}

func TestNotificationHandler(t *testing.T) {
	h := handler.NewNotificationHandler(&mockRepo{})
	ctx := context.Background()

	t.Run("ListNotifications", func(t *testing.T) {
		res, err := h.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{PageSize: 10, PageNumber: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Notifications) != 1 {
			t.Fatalf("expected 1 notification, got %d", len(res.Notifications))
		}
		if res.TotalUnread != 1 {
			t.Fatalf("expected TotalUnread 1, got %d", res.TotalUnread)
		}
	})

	t.Run("MarkAsRead", func(t *testing.T) {
		res, err := h.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{Id: "noti_1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Success {
			t.Fatalf("expected success true")
		}
	})

	t.Run("GetUnreadCount", func(t *testing.T) {
		res, err := h.GetUnreadCount(ctx, &notificationv1.GetUnreadCountRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.UnreadCount != 1 {
			t.Fatalf("expected unread count 1, got %d", res.UnreadCount)
		}
	})
}
