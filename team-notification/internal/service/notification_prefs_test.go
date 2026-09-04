package service_test

import (
	"context"
	"testing"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/repository"
	"github.com/buidangphuc/team-notification/internal/service"
)

func newPrefsService() *service.PrefsService {
	return service.NewPrefsService(repository.NewInMemoryNotificationPrefsRepo())
}

// Get on a user who has never saved preferences returns defaults: every real
// notification type enabled and digest delivery off.
func TestGetReturnsDefaultsWhenUnset(t *testing.T) {
	svc := newPrefsService()

	got, err := svc.Get(context.Background(), "user_a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GetDigestFreq() != notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF {
		t.Fatalf("expected default digest OFF, got %v", got.GetDigestFreq())
	}
	if !got.GetTypeEnabled()[notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER.String()] {
		t.Fatalf("expected ORDER enabled by default")
	}
	if len(got.GetTypeEnabled()) != 6 {
		t.Fatalf("expected 6 default types, got %d", len(got.GetTypeEnabled()))
	}
}

// Update then Get returns the saved value, and one user's preferences never
// leak into another user's (cross-user isolation).
func TestUpdateRoundtripAndIsolation(t *testing.T) {
	svc := newPrefsService()
	ctx := context.Background()

	want := &notificationv1.NotificationPrefs{
		TypeEnabled: map[string]bool{
			notificationv1.NotificationType_NOTIFICATION_TYPE_PROMOTION.String(): false,
			notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER.String():     true,
		},
		DigestFreq: notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY,
	}

	saved, err := svc.Update(ctx, "user_a", want)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if saved.GetDigestFreq() != notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY {
		t.Fatalf("expected WEEKLY, got %v", saved.GetDigestFreq())
	}

	got, err := svc.Get(ctx, "user_a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetDigestFreq() != notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY {
		t.Fatalf("roundtrip digest mismatch: %v", got.GetDigestFreq())
	}
	if got.GetTypeEnabled()[notificationv1.NotificationType_NOTIFICATION_TYPE_PROMOTION.String()] {
		t.Fatalf("expected PROMOTION disabled after update")
	}

	// user_b never set prefs → still defaults, unaffected by user_a's update.
	other, err := svc.Get(ctx, "user_b")
	if err != nil {
		t.Fatalf("get user_b: %v", err)
	}
	if other.GetDigestFreq() != notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF {
		t.Fatalf("cross-user leak: user_b should have default OFF, got %v", other.GetDigestFreq())
	}
}

// Empty user id and nil payload are rejected without panicking.
func TestPrefsInputValidation(t *testing.T) {
	svc := newPrefsService()
	ctx := context.Background()

	if _, err := svc.Get(ctx, ""); err != service.ErrEmptyUser {
		t.Fatalf("expected ErrEmptyUser on empty Get, got %v", err)
	}
	if _, err := svc.Update(ctx, "", &notificationv1.NotificationPrefs{}); err != service.ErrEmptyUser {
		t.Fatalf("expected ErrEmptyUser on empty Update, got %v", err)
	}
	if _, err := svc.Update(ctx, "user_a", nil); err != service.ErrNilPrefs {
		t.Fatalf("expected ErrNilPrefs on nil payload, got %v", err)
	}
}

// BucketDigest is the pure helper the scheduler relies on: it groups pending
// notifications per recipient, drops types the user disabled, drops
// non-recipients, and skips nil records.
func TestBucketDigestHelper(t *testing.T) {
	prefsByUser := map[string]*notificationv1.NotificationPrefs{
		"user_a": {
			TypeEnabled: map[string]bool{
				notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER.String():     true,
				notificationv1.NotificationType_NOTIFICATION_TYPE_PROMOTION.String(): false,
			},
		},
		"user_b": {TypeEnabled: map[string]bool{}}, // empty map → default-on
	}
	recipients := []string{"user_a", "user_b"}

	pending := []*notificationv1.Notification{
		{Id: "n1", UserId: "user_a", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER},
		{Id: "n2", UserId: "user_a", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_PROMOTION}, // dropped: disabled
		{Id: "n3", UserId: "user_b", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT},      // kept: default-on
		{Id: "n4", UserId: "user_c", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER},     // dropped: not a recipient
		nil, // dropped: nil-safe
	}

	buckets := service.BucketDigest(recipients, prefsByUser, pending)
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}

	// Deterministic recipient order: user_a first.
	if buckets[0].UserID != "user_a" || len(buckets[0].Notifications) != 1 {
		t.Fatalf("user_a bucket wrong: %+v", buckets[0])
	}
	if buckets[0].Notifications[0].GetId() != "n1" {
		t.Fatalf("expected only n1 for user_a, got %s", buckets[0].Notifications[0].GetId())
	}
	if buckets[1].UserID != "user_b" || len(buckets[1].Notifications) != 1 {
		t.Fatalf("user_b bucket wrong: %+v", buckets[1])
	}
}

// A user set to digest OFF is never bundled, even with pending notifications.
func TestBundleDigestSkipsOff(t *testing.T) {
	svc := newPrefsService()
	ctx := context.Background()

	buckets, err := svc.BundleDigest(ctx, notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buckets != nil {
		t.Fatalf("expected no buckets for OFF, got %+v", buckets)
	}
}

// BundleDigest queries recipients by cadence and bundles their pending items.
func TestBundleDigestByFrequency(t *testing.T) {
	svc := newPrefsService()
	ctx := context.Background()

	if _, err := svc.Update(ctx, "daily_user", &notificationv1.NotificationPrefs{
		DigestFreq: notificationv1.DigestFrequency_DIGEST_FREQUENCY_DAILY,
	}); err != nil {
		t.Fatalf("update daily_user: %v", err)
	}
	if _, err := svc.Update(ctx, "weekly_user", &notificationv1.NotificationPrefs{
		DigestFreq: notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY,
	}); err != nil {
		t.Fatalf("update weekly_user: %v", err)
	}

	pending := []*notificationv1.Notification{
		{Id: "d1", UserId: "daily_user", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER},
		{Id: "w1", UserId: "weekly_user", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER},
	}

	buckets, err := svc.BundleDigest(ctx, notificationv1.DigestFrequency_DIGEST_FREQUENCY_DAILY, pending)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if len(buckets) != 1 || buckets[0].UserID != "daily_user" {
		t.Fatalf("expected only daily_user bundled, got %+v", buckets)
	}
	if len(buckets[0].Notifications) != 1 || buckets[0].Notifications[0].GetId() != "d1" {
		t.Fatalf("expected d1 for daily_user, got %+v", buckets[0].Notifications)
	}
}
