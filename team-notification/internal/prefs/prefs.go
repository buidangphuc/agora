package prefs

import (
	"context"
	"errors"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/store"
)

var (
	ErrNilPrefs  = errors.New("prefs payload is required")
	ErrEmptyUser = errors.New("user id is required")
)

type Repository interface {
	Get(ctx context.Context, userID string) (*notificationv1.NotificationPrefs, error)
	Upsert(ctx context.Context, userID string, p *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error)
	ListUsersByDigestFreq(ctx context.Context, freq notificationv1.DigestFrequency) ([]string, error)
}

var defaultEnabledTypes = []notificationv1.NotificationType{
	notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER,
	notificationv1.NotificationType_NOTIFICATION_TYPE_PROMOTION,
	notificationv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
	notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT,
	notificationv1.NotificationType_NOTIFICATION_TYPE_PRICE_DROP,
	notificationv1.NotificationType_NOTIFICATION_TYPE_BACK_IN_STOCK,
}

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

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context, userID string) (*notificationv1.NotificationPrefs, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	prefs, err := s.repo.Get(ctx, userID)
	if errors.Is(err, store.ErrPrefsNotFound) {
		return DefaultPrefs(), nil
	}
	if err != nil {
		return nil, err
	}
	return prefs, nil
}

func (s *Service) Update(ctx context.Context, userID string, prefs *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error) {
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

type DigestBucket struct {
	UserID        string
	Notifications []*notificationv1.Notification
}

func (s *Service) BundleDigest(ctx context.Context, freq notificationv1.DigestFrequency, pending []*notificationv1.Notification) ([]DigestBucket, error) {
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
		if errors.Is(err, store.ErrPrefsNotFound) {
			p = DefaultPrefs()
		} else if err != nil {
			return nil, err
		}
		prefsByUser[uid] = p
	}
	return BucketDigest(recipients, prefsByUser, pending), nil
}

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

	buckets := make([]DigestBucket, 0, len(byUser))
	for _, uid := range recipients {
		if ns := byUser[uid]; len(ns) > 0 {
			buckets = append(buckets, DigestBucket{UserID: uid, Notifications: ns})
		}
	}
	return buckets
}

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
