package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-sharing/internal/repository"
	"github.com/buidangphuc/team-sharing/internal/service"
)

func newService() *service.ShareService {
	return service.NewShareService(repository.NewInMemoryShareLinkRepo())
}

// Create then Resolve returns the same target and UTM, and a non-empty short
// code plus synthesized OG metadata.
func TestCreateResolveRoundtrip(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	utm := map[string]string{"utm_source": "zalo", "utm_medium": "social"}
	created, err := svc.CreateShareLink(ctx, "listing", "listing_123", utm)
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
	if !errors.Is(err, service.ErrShareLinkNotFound) {
		t.Fatalf("expected ErrShareLinkNotFound, got %v", err)
	}
}

// Each resolve bumps click_count by one.
func TestResolveIncrementsClickCount(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateShareLink(ctx, "storefront", "shop_9", nil)
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

	if _, err := svc.CreateShareLink(ctx, "", "id", nil); !errors.Is(err, service.ErrEmptyTargetType) {
		t.Fatalf("expected ErrEmptyTargetType, got %v", err)
	}
	if _, err := svc.CreateShareLink(ctx, "listing", "", nil); !errors.Is(err, service.ErrEmptyTargetID) {
		t.Fatalf("expected ErrEmptyTargetID, got %v", err)
	}
	if _, err := svc.ResolveShareLink(ctx, ""); !errors.Is(err, service.ErrEmptyShortCode) {
		t.Fatalf("expected ErrEmptyShortCode, got %v", err)
	}
}

// nil UTM is accepted and resolves to an empty (non-nil) map — anonymous/empty
// input is handled without panicking.
func TestCreateWithNilUTM(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateShareLink(ctx, "listing", "l1", nil)
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
