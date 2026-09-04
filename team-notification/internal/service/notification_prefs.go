// This file holds the notification-preferences use case: reading a user's
// per-type opt-in map + digest cadence (with sensible defaults when unset),
// updating them, and the digest-bundling helper a scheduler would call. It
// mirrors AlertService's layering — validate here, persist in the repository;
// identity always comes from the handler, never guessed.
package service

import (
	"context"
	"errors"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/repository"
)

// ErrNilPrefs is returned when an update carries no preferences payload.
var ErrNilPrefs = errors.New("prefs payload is required")

// defaultEnabledTypes lists the notification types opted into by default. A user
// with no saved preferences receives every real notification type and no digest.
var defaultEnabledTypes = []notificationv1.NotificationType{
	notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER,
	notificationv1.NotificationType_NOTIFICATION_TYPE_PROMOTION,
	notificationv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
	notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT,
	notificationv1.NotificationType_NOTIFICATION_TYPE_PRICE_DROP,
	notificationv1.NotificationType_NOTIFICATION_TYPE_BACK_IN_STOCK,
}

// DefaultPrefs returns the fallback preferences: all real notification types
// enabled and digest delivery off (notifications arrive individually).
func DefaultPrefs() *notificationv1.NotificationPrefs {
	te := make(map[string]bool, len(defaultEnabledTypes))
	for _, t := range defaultEnabledTypes {
		te[t.String()] = true
	}
	return &notificationv1.NotificationPrefs{
		TypeEnabled: te,
		DigestFreq:  notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF,
	}
}

// PrefsService is the notification-preferences use-case boundary.
type PrefsService struct {
	repo repository.NotificationPrefsRepository
}

func NewPrefsService(repo repository.NotificationPrefsRepository) *PrefsService {
	return &PrefsService{repo: repo}
}

// Get returns the user's preferences, or defaults when none have been set.
func (s *PrefsService) Get(ctx context.Context, userID string) (*notificationv1.NotificationPrefs, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	prefs, err := s.repo.Get(ctx, userID)
	if errors.Is(err, repository.ErrPrefsNotFound) {
		return DefaultPrefs(), nil
	}
	if err != nil {
		return nil, err
	}
	return prefs, nil
}

// Update persists the user's preferences and returns the stored value. A nil
// type_enabled map is normalized to empty so callers never see a nil map.
func (s *PrefsService) Update(ctx context.Context, userID string, prefs *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	if prefs == nil {
		return nil, ErrNilPrefs
	}
	if prefs.TypeEnabled == nil {
		prefs.TypeEnabled = map[string]bool{}
	}
	return s.repo.Upsert(ctx, userID, prefs)
}

// DigestBucket is one user's bundle of notifications selected for a digest run.
type DigestBucket struct {
	UserID        string
	Notifications []*notificationv1.Notification
}

// BundleDigest gathers, for every user whose digest cadence matches freq, the
// pending notifications they should receive — honoring each user's per-type
// opt-in prefs. `pending` is the set of undelivered notifications a scheduler
// has collected since the last run. This is the service seam a cron would call;
// wiring the cron itself is out of scope.
func (s *PrefsService) BundleDigest(ctx context.Context, freq notificationv1.DigestFrequency, pending []*notificationv1.Notification) ([]DigestBucket, error) {
	if freq == notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF {
		return nil, nil // OFF users are delivered individually, never digested.
	}
	recipients, err := s.repo.ListUsersByDigestFreq(ctx, freq)
	if err != nil {
		return nil, err
	}
	prefsByUser := make(map[string]*notificationv1.NotificationPrefs, len(recipients))
	for _, uid := range recipients {
		p, err := s.repo.Get(ctx, uid)
		if errors.Is(err, repository.ErrPrefsNotFound) {
			p = DefaultPrefs()
		} else if err != nil {
			return nil, err
		}
		prefsByUser[uid] = p
	}
	return BucketDigest(recipients, prefsByUser, pending), nil
}

// BucketDigest groups pending notifications by recipient for a digest run,
// keeping only recipients in `recipients` and dropping notifications whose type
// the user has turned off. It is a pure function (no I/O) so the bundling logic
// is unit-testable on its own. Users with no surviving notifications are omitted.
func BucketDigest(recipients []string, prefsByUser map[string]*notificationv1.NotificationPrefs, pending []*notificationv1.Notification) []DigestBucket {
	recipientSet := make(map[string]struct{}, len(recipients))
	for _, uid := range recipients {
		recipientSet[uid] = struct{}{}
	}

	byUser := make(map[string][]*notificationv1.Notification)
	for _, n := range pending {
		if n == nil {
			continue
		}
		uid := n.GetUserId()
		if _, ok := recipientSet[uid]; !ok {
			continue
		}
		if !typeEnabled(prefsByUser[uid], n.GetType()) {
			continue
		}
		byUser[uid] = append(byUser[uid], n)
	}

	// Emit in the stable recipient order so callers get deterministic output.
	buckets := make([]DigestBucket, 0, len(byUser))
	for _, uid := range recipients {
		if ns := byUser[uid]; len(ns) > 0 {
			buckets = append(buckets, DigestBucket{UserID: uid, Notifications: ns})
		}
	}
	return buckets
}

// typeEnabled reports whether a user wants notifications of the given type.
// A missing entry means "not opted out" → enabled (mirrors the default-on model).
func typeEnabled(prefs *notificationv1.NotificationPrefs, t notificationv1.NotificationType) bool {
	if prefs == nil {
		return true
	}
	enabled, ok := prefs.GetTypeEnabled()[t.String()]
	if !ok {
		return true
	}
	return enabled
}
