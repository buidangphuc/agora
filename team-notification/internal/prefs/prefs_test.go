package prefs_test

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"sync"
	"testing"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/prefs"
	"github.com/buidangphuc/team-notification/internal/store"
)

type fakeRepo struct {
	mu   sync.Mutex
	rows map[string]*notificationv1.NotificationPrefs
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[string]*notificationv1.NotificationPrefs{}}
}

func (r *fakeRepo) Get(_ context.Context, userID string) (*notificationv1.NotificationPrefs, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.rows[userID]
	if !ok {
		return nil, store.ErrPrefsNotFound
	}
	return p, nil
}

func (r *fakeRepo) Upsert(_ context.Context, userID string, p *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[userID] = p
	return p, nil
}

func (r *fakeRepo) ListUsersByDigestFreq(_ context.Context, freq notificationv1.DigestFrequency) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for id, p := range r.rows {
		if p.GetDigestFreq() == freq {
			out = append(out, id)
		}
	}
	return out, nil
}

func newService() (*prefs.Service, *fakeRepo) {
	repo := newFakeRepo()
	return prefs.NewService(repo), repo
}

func TestGetReturnsDefaultsWhenUnset_PreferencesDefaultAndRoundTripPerUser(t *testing.T) {
	svc, _ := newService()

	got, err := svc.Get(context.Background(), "user_a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GetDigestFreq() != notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF {
		t.Fatalf("expected default digest OFF, got %v", got.GetDigestFreq())
	}
	if len(got.GetTypeEnabled()) != 6 {
		t.Fatalf("expected 6 default types, got %d", len(got.GetTypeEnabled()))
	}
	for k, v := range got.GetTypeEnabled() {
		if !v {
			t.Fatalf("expected %s enabled by default", k)
		}
	}
	if !got.GetTypeEnabled()[notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT.String()] {
		t.Fatalf("expected CHAT enabled by default")
	}
	if _, ok := got.GetTypeEnabled()[notificationv1.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED.String()]; ok {
		t.Fatalf("UNSPECIFIED must not be a default type")
	}
}

func TestDefaultPrefsAreFreshCopies_PreferencesDefaultAndRoundTripPerUser(t *testing.T) {
	a := prefs.DefaultPrefs()
	a.TypeEnabled[notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER.String()] = false
	b := prefs.DefaultPrefs()
	if !b.GetTypeEnabled()[notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER.String()] {
		t.Fatalf("mutating one default leaked into the next")
	}
}

func TestUpdateRoundtripAndIsolation_PreferencesDefaultAndRoundTripPerUser(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()

	want := &notificationv1.NotificationPrefs{
		TypeEnabled: map[string]bool{
			notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT.String():  false,
			notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER.String(): true,
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
	if got.GetTypeEnabled()[notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT.String()] {
		t.Fatalf("expected CHAT disabled after update")
	}

	other, err := svc.Get(ctx, "user_b")
	if err != nil {
		t.Fatalf("get user_b: %v", err)
	}
	if other.GetDigestFreq() != notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF {
		t.Fatalf("cross-user leak: user_b digest %v", other.GetDigestFreq())
	}
	if !other.GetTypeEnabled()[notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT.String()] {
		t.Fatalf("cross-user leak: user_b CHAT disabled")
	}
}

func TestPrefsInputValidation_AMissingPrefsPayloadIsRejected(t *testing.T) {
	svc, repo := newService()
	ctx := context.Background()

	if _, err := svc.Get(ctx, ""); !errors.Is(err, prefs.ErrEmptyUser) {
		t.Fatalf("expected ErrEmptyUser on empty Get, got %v", err)
	}
	if _, err := svc.Update(ctx, "", &notificationv1.NotificationPrefs{}); !errors.Is(err, prefs.ErrEmptyUser) {
		t.Fatalf("expected ErrEmptyUser on empty Update, got %v", err)
	}
	if _, err := svc.Update(ctx, "user_a", nil); !errors.Is(err, prefs.ErrNilPrefs) {
		t.Fatalf("expected ErrNilPrefs on nil payload, got %v", err)
	}
	if len(repo.rows) != 0 {
		t.Fatalf("rejected updates must store nothing, got %d rows", len(repo.rows))
	}
	got, err := svc.Get(ctx, "user_a")
	if err != nil {
		t.Fatalf("get after rejection: %v", err)
	}
	if got.GetDigestFreq() != notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF || len(got.GetTypeEnabled()) != 6 {
		t.Fatalf("expected defaults after rejected update, got %+v", got)
	}
}

func TestSentinelErrorsAreDistinct_AMissingPrefsPayloadIsRejected(t *testing.T) {
	if prefs.ErrNilPrefs == nil || prefs.ErrEmptyUser == nil {
		t.Fatalf("sentinels must be non-nil")
	}
	if errors.Is(prefs.ErrNilPrefs, prefs.ErrEmptyUser) || errors.Is(prefs.ErrEmptyUser, prefs.ErrNilPrefs) {
		t.Fatalf("sentinels must be distinct")
	}
}

func TestBucketDigestHelper_PreferencesDefaultAndRoundTripPerUser(t *testing.T) {
	prefsByUser := map[string]*notificationv1.NotificationPrefs{
		"user_a": {
			TypeEnabled: map[string]bool{
				notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER.String():     true,
				notificationv1.NotificationType_NOTIFICATION_TYPE_PROMOTION.String(): false,
			},
		},
		"user_b": {TypeEnabled: map[string]bool{}},
	}
	recipients := []string{"user_a", "user_b"}
	pending := []*notificationv1.Notification{
		{Id: "n1", UserId: "user_a", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER},
		{Id: "n2", UserId: "user_a", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_PROMOTION},
		{Id: "n3", UserId: "user_b", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT},
		{Id: "n4", UserId: "user_c", Type: notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER},
		nil,
	}

	buckets := prefs.BucketDigest(recipients, prefsByUser, pending)
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}
	if buckets[0].UserID != "user_a" || len(buckets[0].Notifications) != 1 || buckets[0].Notifications[0].GetId() != "n1" {
		t.Fatalf("user_a bucket wrong: %+v", buckets[0])
	}
	if buckets[1].UserID != "user_b" || len(buckets[1].Notifications) != 1 || buckets[1].Notifications[0].GetId() != "n3" {
		t.Fatalf("user_b bucket wrong: %+v", buckets[1])
	}
}

func TestBundleDigestSkipsOff_PreferencesDefaultAndRoundTripPerUser(t *testing.T) {
	svc, _ := newService()
	buckets, err := svc.BundleDigest(context.Background(), notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buckets != nil {
		t.Fatalf("expected no buckets for OFF, got %+v", buckets)
	}
}

func TestBundleDigestByFrequency_PreferencesDefaultAndRoundTripPerUser(t *testing.T) {
	svc, _ := newService()
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

func TestPackageHasNoGRPCImport_AMissingPrefsPayloadIsRejected(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, "google.golang.org/grpc") {
				t.Fatalf("%s imports %s; domain packages must not depend on grpc", e.Name(), imp.Path.Value)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("package prefs has no non-test source files")
	}
}
