package alerts_test

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
	"github.com/buidangphuc/team-notification/internal/alerts"
)

type fakeRepo struct {
	mu   sync.Mutex
	next int
	subs []*notificationv1.AlertSubscription
}

func (r *fakeRepo) Create(_ context.Context, sub *notificationv1.AlertSubscription) (*notificationv1.AlertSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.subs {
		if s.UserId == sub.UserId && s.ListingId == sub.ListingId && s.Type == sub.Type {
			return s, nil
		}
	}
	r.next++
	sub.Id = "alert_" + string(rune('a'+r.next))
	r.subs = append(r.subs, sub)
	return sub, nil
}

func (r *fakeRepo) Delete(_ context.Context, id, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.subs {
		if s.Id == id && s.UserId == userID {
			r.subs = append(r.subs[:i], r.subs[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *fakeRepo) ListByUser(_ context.Context, userID string) ([]*notificationv1.AlertSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*notificationv1.AlertSubscription
	for _, s := range r.subs {
		if s.UserId == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func TestSubscribeIsIdempotent_AlertAndPreferenceRPCsValidateInput(t *testing.T) {
	repo := &fakeRepo{}
	svc := alerts.NewService(repo)
	ctx := context.Background()

	a, err := svc.Subscribe(ctx, "u1", "l1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Subscribe(ctx, "u1", "l1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP)
	if err != nil {
		t.Fatal(err)
	}
	if a.GetId() == "" || a.GetId() != b.GetId() {
		t.Fatalf("expected same id, got %q and %q", a.GetId(), b.GetId())
	}
	list, err := svc.List(ctx, "u1")
	if err != nil || len(list) != 1 {
		t.Fatalf("expected one row, got %d err=%v", len(list), err)
	}
}

func TestSubscribeValidation_AlertAndPreferenceRPCsValidateInput(t *testing.T) {
	repo := &fakeRepo{}
	svc := alerts.NewService(repo)
	ctx := context.Background()

	cases := []struct {
		name    string
		user    string
		listing string
		typ     notificationv1.AlertType
		want    error
	}{
		{"empty user", "", "l1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP, alerts.ErrEmptyUser},
		{"empty listing", "u1", "", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP, alerts.ErrEmptyListing},
		{"unspecified type", "u1", "l1", notificationv1.AlertType_ALERT_TYPE_UNSPECIFIED, alerts.ErrInvalidType},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.Subscribe(ctx, c.user, c.listing, c.typ); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
	if len(repo.subs) != 0 {
		t.Fatalf("no row may be created on validation failure, got %d", len(repo.subs))
	}
}

func TestUnsubscribeValidationAndForeignNoOp_AlertAndPreferenceRPCsValidateInput(t *testing.T) {
	repo := &fakeRepo{}
	svc := alerts.NewService(repo)
	ctx := context.Background()

	sub, err := svc.Subscribe(ctx, "u1", "l1", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Unsubscribe(ctx, "", sub.GetId()); !errors.Is(err, alerts.ErrEmptyUser) {
		t.Fatalf("want ErrEmptyUser, got %v", err)
	}
	if err := svc.Unsubscribe(ctx, "u1", ""); !errors.Is(err, alerts.ErrEmptySubID) {
		t.Fatalf("want ErrEmptySubID, got %v", err)
	}
	if err := svc.Unsubscribe(ctx, "u2", sub.GetId()); err != nil {
		t.Fatalf("foreign unsubscribe must be a no-op success, got %v", err)
	}
	if err := svc.Unsubscribe(ctx, "u1", "missing"); err != nil {
		t.Fatalf("missing unsubscribe must succeed, got %v", err)
	}
	if list, _ := svc.List(ctx, "u1"); len(list) != 1 {
		t.Fatalf("owner's row must survive foreign delete, got %d", len(list))
	}
	if err := svc.Unsubscribe(ctx, "u1", sub.GetId()); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.List(ctx, "u1"); len(list) != 0 {
		t.Fatalf("owner delete must remove row, got %d", len(list))
	}
}

func TestListScopedToUser_AlertAndPreferenceRPCsValidateInput(t *testing.T) {
	svc := alerts.NewService(&fakeRepo{})
	ctx := context.Background()
	if _, err := svc.List(ctx, ""); !errors.Is(err, alerts.ErrEmptyUser) {
		t.Fatalf("want ErrEmptyUser, got %v", err)
	}
	if _, err := svc.Subscribe(ctx, "u1", "l1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP); err != nil {
		t.Fatal(err)
	}
	if list, err := svc.List(ctx, "u2"); err != nil || len(list) != 0 {
		t.Fatalf("u2 must see nothing, got %d err=%v", len(list), err)
	}
}

func TestPackageHasNoGRPCImport_AlertAndPreferenceRPCsValidateInput(t *testing.T) {
	noGRPCImport(t)
}

func noGRPCImport(t *testing.T) {
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
				t.Fatalf("%s imports %s", e.Name(), imp.Path.Value)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("no non-test source files")
	}
}
