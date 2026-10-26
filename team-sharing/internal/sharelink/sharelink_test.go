package sharelink_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/buidangphuc/team-sharing/internal/sharelink"
)

func newService() *sharelink.Service {
	return sharelink.NewService(newFakeRepo())
}

// Create then Resolve returns the same target and UTM, and a non-empty short
// code plus synthesized OG metadata.
func TestCreateResolveRoundtrip(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	utm := map[string]string{"utm_source": "zalo", "utm_medium": "social"}
	created, err := svc.CreateShareLink(ctx, "", "listing", "listing_123", utm)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ShortCode == "" {
		t.Fatalf("expected a non-empty short code")
	}

	got, err := svc.ResolveShareLink(ctx, created.ShortCode)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.TargetType != "listing" || got.TargetID != "listing_123" {
		t.Fatalf("target mismatch: %+v", got)
	}
	if got.UTM["utm_source"] != "zalo" || got.UTM["utm_medium"] != "social" {
		t.Fatalf("utm mismatch: %+v", got.UTM)
	}
	if got.OgTitle == "" || got.OgDescription == "" || got.OgImageURL == "" {
		t.Fatalf("expected OG meta to be populated: %+v", got)
	}
}

// Resolving an unknown short code returns ErrShareLinkNotFound (the handler maps
// this to gRPC NotFound).
func TestResolveUnknownReturnsNotFound(t *testing.T) {
	svc := newService()

	_, err := svc.ResolveShareLink(context.Background(), "does_not_exist")
	if !errors.Is(err, sharelink.ErrShareLinkNotFound) {
		t.Fatalf("expected ErrShareLinkNotFound, got %v", err)
	}
}

// Each resolve bumps click_count by one.
func TestResolveIncrementsClickCount(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateShareLink(ctx, "", "storefront", "shop_9", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for want := int64(1); want <= 3; want++ {
		got, err := svc.ResolveShareLink(ctx, created.ShortCode)
		if err != nil {
			t.Fatalf("resolve #%d: %v", want, err)
		}
		if got.ClickCount != want {
			t.Fatalf("expected click_count %d after %d resolves, got %d", want, want, got.ClickCount)
		}
	}
}

// target_type, target_id and short_code are validated before any repository call.
func TestCreateAndResolveValidation(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	if _, err := svc.CreateShareLink(ctx, "", "", "id", nil); !errors.Is(err, sharelink.ErrEmptyTargetType) {
		t.Fatalf("expected ErrEmptyTargetType, got %v", err)
	}
	if _, err := svc.CreateShareLink(ctx, "", "listing", "", nil); !errors.Is(err, sharelink.ErrEmptyTargetID) {
		t.Fatalf("expected ErrEmptyTargetID, got %v", err)
	}
	if _, err := svc.ResolveShareLink(ctx, ""); !errors.Is(err, sharelink.ErrEmptyShortCode) {
		t.Fatalf("expected ErrEmptyShortCode, got %v", err)
	}
}

// nil UTM is accepted and resolves to an empty (non-nil) map — anonymous/empty
// input is handled without panicking.
func TestCreateWithNilUTM(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateShareLink(ctx, "", "listing", "l1", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.ResolveShareLink(ctx, created.ShortCode)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.UTM == nil {
		t.Fatalf("expected non-nil empty UTM map")
	}
	if len(got.UTM) != 0 {
		t.Fatalf("expected empty UTM, got %+v", got.UTM)
	}
}

func TestCreateShareLinkRecordsCreator(t *testing.T) {
	ctx := context.Background()
	svc := sharelink.NewService(newFakeRepo())

	owned, err := svc.CreateShareLink(ctx, "user-7", "listing", "l1", nil)
	if err != nil || owned.CreatedBy != "user-7" {
		t.Fatalf("owned link: %+v err=%v", owned, err)
	}
	anon, err := svc.CreateShareLink(ctx, "", "listing", "l1", nil)
	if err != nil || anon.CreatedBy != "" {
		t.Fatalf("anonymous link: %+v err=%v", anon, err)
	}
}

type fakeRepo struct {
	mu       sync.Mutex
	links    map[string]*sharelink.ShareLink
	attempts int
	collide  int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{links: map[string]*sharelink.ShareLink{}}
}

func clone(l *sharelink.ShareLink) *sharelink.ShareLink {
	c := *l
	c.UTM = map[string]string{}
	for k, v := range l.UTM {
		c.UTM[k] = v
	}
	return &c
}

func (r *fakeRepo) Create(_ context.Context, l *sharelink.ShareLink) (*sharelink.ShareLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts++
	if r.collide < 0 || r.attempts <= r.collide {
		return nil, sharelink.ErrShortCodeExists
	}
	if _, ok := r.links[l.ShortCode]; ok {
		return nil, sharelink.ErrShortCodeExists
	}
	r.links[l.ShortCode] = clone(l)
	return clone(l), nil
}

func (r *fakeRepo) Resolve(_ context.Context, code string) (*sharelink.ShareLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.links[code]
	if !ok {
		return nil, sharelink.ErrShareLinkNotFound
	}
	l.ClickCount++
	return clone(l), nil
}

func TestCreateGivesUpAfterFiveCollisions(t *testing.T) {
	repo := newFakeRepo()
	repo.collide = -1
	svc := sharelink.NewService(repo)

	_, err := svc.CreateShareLink(context.Background(), "", "listing", "l1", nil)
	if err == nil {
		t.Fatalf("expected an error after repeated collisions")
	}
	if errors.Is(err, sharelink.ErrShareLinkNotFound) || errors.Is(err, sharelink.ErrEmptyTargetType) ||
		errors.Is(err, sharelink.ErrEmptyTargetID) || errors.Is(err, sharelink.ErrEmptyShortCode) {
		t.Fatalf("expected a non-domain error, got %v", err)
	}
	if repo.attempts != 5 {
		t.Fatalf("expected exactly 5 store attempts, got %d", repo.attempts)
	}
}

func TestCreateRetriesOnCollisionThenSucceeds(t *testing.T) {
	repo := newFakeRepo()
	repo.collide = 4
	svc := sharelink.NewService(repo)

	got, err := svc.CreateShareLink(context.Background(), "", "listing", "l1", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if repo.attempts != 5 || len(got.ShortCode) != 7 {
		t.Fatalf("attempts=%d code=%q", repo.attempts, got.ShortCode)
	}
}
