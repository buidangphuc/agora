package listing_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

const sfBase = "http://localhost:9000/listing-images"

func newSF(seed ...store.Storefront) (*listing.StorefrontService, *storetest.InMemoryStorefrontRepository) {
	repo := storetest.NewInMemoryStorefrontRepository(seed...)
	return listing.NewStorefrontService(repo, sfBase), repo
}

func upsertSF(svc *listing.StorefrontService, sf store.Storefront) (store.Storefront, error) {
	if sf.Slug == "" {
		sf.Slug = "shop-a"
	}
	return svc.Upsert(context.Background(), sf, "seller-a")
}

func storedSF(t *testing.T, repo *storetest.InMemoryStorefrontRepository) store.Storefront {
	t.Helper()
	got, err := repo.GetBySeller(context.Background(), "seller-a")
	if err != nil {
		t.Fatalf("GetBySeller: %v", err)
	}
	return got
}

func assertNotStored(t *testing.T, repo *storetest.InMemoryStorefrontRepository) {
	t.Helper()
	if _, err := repo.GetBySeller(context.Background(), "seller-a"); !errors.Is(err, store.ErrStorefrontNotFound) {
		t.Fatalf("expected nothing stored, GetBySeller err = %v", err)
	}
}

// Scenario: Valid tagline and theme are saved
func TestUpsertStoresTrimmedTaglineAndTheme(t *testing.T) {
	svc, repo := newSF()
	out, err := upsertSF(svc, store.Storefront{Tagline: "  Hoa tươi mỗi ngày  ", Theme: "festive"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if out.Tagline != "Hoa tươi mỗi ngày" || out.Theme != "festive" {
		t.Fatalf("response = %q/%q", out.Tagline, out.Theme)
	}
	got := storedSF(t, repo)
	if got.Tagline != "Hoa tươi mỗi ngày" || got.Theme != "festive" {
		t.Fatalf("stored = %q/%q", got.Tagline, got.Theme)
	}
}

// Scenario: Valid tagline and theme are saved (every allowed theme)
func TestUpsertAcceptsAllowedThemes(t *testing.T) {
	for _, th := range []string{"default", "dark", "festive"} {
		svc, _ := newSF()
		if _, err := upsertSF(svc, store.Storefront{Theme: th}); err != nil {
			t.Errorf("theme %q rejected: %v", th, err)
		}
	}
}

// Scenario: Tagline of exactly 120 runes is accepted (boundary)
func TestUpsertTagline120RunesAccepted(t *testing.T) {
	svc, repo := newSF()
	tag := strings.Repeat("é", 120)
	if _, err := upsertSF(svc, store.Storefront{Tagline: tag}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := storedSF(t, repo).Tagline; got != tag {
		t.Fatalf("stored tagline runes = %d", len([]rune(got)))
	}
}

// Scenario: Tagline over 120 runes is rejected
func TestUpsertTagline121RunesRejected(t *testing.T) {
	svc, repo := newSF()
	_, err := upsertSF(svc, store.Storefront{Tagline: strings.Repeat("é", 121)})
	if !errors.Is(err, listing.ErrTaglineInvalid) {
		t.Fatalf("err = %v, want ErrTaglineInvalid", err)
	}
	assertNotStored(t, repo)
}

// Scenario: Tagline over 120 runes is rejected (length is measured after trimming)
func TestUpsertTaglineLengthMeasuredAfterTrim(t *testing.T) {
	svc, _ := newSF()
	if _, err := upsertSF(svc, store.Storefront{Tagline: "  " + strings.Repeat("é", 120) + "  "}); err != nil {
		t.Fatalf("120 runes plus padding rejected: %v", err)
	}
}

// Scenario: Tagline with a control character is rejected
func TestUpsertTaglineControlCharRejected(t *testing.T) {
	for _, tag := range []string{"line1\nline2", "nul\x00byte", "tab\there"} {
		svc, repo := newSF()
		_, err := upsertSF(svc, store.Storefront{Tagline: tag})
		if !errors.Is(err, listing.ErrTaglineInvalid) {
			t.Errorf("tagline %q: err = %v, want ErrTaglineInvalid", tag, err)
		}
		assertNotStored(t, repo)
	}
}

// Scenario: Unknown theme is rejected
func TestUpsertUnknownThemeRejected(t *testing.T) {
	svc, repo := newSF()
	_, err := upsertSF(svc, store.Storefront{Theme: "neon"})
	if !errors.Is(err, listing.ErrThemeInvalid) {
		t.Fatalf("err = %v, want ErrThemeInvalid", err)
	}
	assertNotStored(t, repo)
}

// Scenario: Unknown theme is rejected (case-sensitive set)
func TestUpsertThemeIsCaseSensitive(t *testing.T) {
	svc, _ := newSF()
	if _, err := upsertSF(svc, store.Storefront{Theme: "Dark"}); !errors.Is(err, listing.ErrThemeInvalid) {
		t.Fatalf("err = %v, want ErrThemeInvalid", err)
	}
}

// Scenario: Empty theme is accepted as default
func TestUpsertEmptyThemeAccepted(t *testing.T) {
	svc, repo := newSF()
	if _, err := upsertSF(svc, store.Storefront{Theme: ""}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := storedSF(t, repo).Theme; got != "" {
		t.Fatalf("stored theme = %q, want empty", got)
	}
}

// Scenario: Banner from the upload bucket is saved
func TestUpsertBannerFromBucketAccepted(t *testing.T) {
	svc, repo := newSF()
	b := sfBase + "/3f2a.png"
	if _, err := upsertSF(svc, store.Storefront{BannerURL: b}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := storedSF(t, repo).BannerURL; got != b {
		t.Fatalf("stored banner = %q", got)
	}
}

// Scenario: Banner from the upload bucket is saved (trailing slash on the injected base)
func TestUpsertBannerBaseTrailingSlashTolerated(t *testing.T) {
	repo := storetest.NewInMemoryStorefrontRepository()
	svc := listing.NewStorefrontService(repo, sfBase+"/")
	if _, err := upsertSF(svc, store.Storefront{BannerURL: sfBase + "/a.png"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
}

// Scenario: Banner from the upload bucket is saved (2048-byte boundary)
func TestUpsertBanner2048BytesBoundary(t *testing.T) {
	svc, _ := newSF()
	ok := sfBase + "/" + strings.Repeat("a", 2048-len(sfBase)-1)
	if len(ok) != 2048 {
		t.Fatalf("fixture length %d", len(ok))
	}
	if _, err := upsertSF(svc, store.Storefront{BannerURL: ok}); err != nil {
		t.Fatalf("2048-byte banner rejected: %v", err)
	}
	svc2, repo2 := newSF()
	if _, err := upsertSF(svc2, store.Storefront{BannerURL: ok + "a"}); !errors.Is(err, listing.ErrBannerInvalid) {
		t.Fatalf("2049-byte banner err = %v, want ErrBannerInvalid", err)
	}
	assertNotStored(t, repo2)
}

// Scenario: External banner URL is rejected
func TestUpsertExternalBannerRejected(t *testing.T) {
	svc, repo := newSF()
	_, err := upsertSF(svc, store.Storefront{BannerURL: "https://evil.example/pixel.png"})
	if !errors.Is(err, listing.ErrBannerInvalid) {
		t.Fatalf("err = %v, want ErrBannerInvalid", err)
	}
	assertNotStored(t, repo)
}

// Scenario: External banner URL is rejected (prefix must end at a path boundary)
func TestUpsertBannerLookalikePrefixRejected(t *testing.T) {
	svc, _ := newSF()
	for _, b := range []string{sfBase, sfBase + "/", sfBase + "-evil/a.png", sfBase + ".evil.example/a.png"} {
		if _, err := upsertSF(svc, store.Storefront{BannerURL: b}); !errors.Is(err, listing.ErrBannerInvalid) {
			t.Errorf("banner %q: err = %v, want ErrBannerInvalid", b, err)
		}
	}
}

// Scenario: Non-http scheme, query string or fragment in banner is rejected
func TestUpsertBannerSchemeQueryFragmentRejected(t *testing.T) {
	for _, b := range []string{"javascript:alert(1)", sfBase + "/a.png?x=1", sfBase + "/a.png#f"} {
		svc, repo := newSF()
		_, err := upsertSF(svc, store.Storefront{BannerURL: b})
		if !errors.Is(err, listing.ErrBannerInvalid) {
			t.Errorf("banner %q: err = %v, want ErrBannerInvalid", b, err)
		}
		assertNotStored(t, repo)
	}
}

// Scenario: Banner can be cleared
func TestUpsertEmptyBannerClears(t *testing.T) {
	svc, repo := newSF(store.Storefront{SellerID: "seller-a", Slug: "shop-a", BannerURL: sfBase + "/old.png"})
	if _, err := upsertSF(svc, store.Storefront{BannerURL: ""}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := storedSF(t, repo).BannerURL; got != "" {
		t.Fatalf("stored banner = %q, want empty", got)
	}
}

// Scenario: A legacy banner already stored does not block saving other fields
func TestUpsertUnchangedLegacyBannerAccepted(t *testing.T) {
	legacy := "https://old.example/banner.png?v=2"
	svc, repo := newSF(store.Storefront{SellerID: "seller-a", Slug: "shop-a", BannerURL: legacy})
	if _, err := upsertSF(svc, store.Storefront{BannerURL: legacy, Tagline: "New line"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got := storedSF(t, repo)
	if got.Tagline != "New line" || got.BannerURL != legacy {
		t.Fatalf("stored = %q / %q", got.Tagline, got.BannerURL)
	}
}

// Scenario: External banner URL is rejected (exemption only for the caller's own stored value)
func TestUpsertLegacyExemptionDoesNotCoverOtherBanner(t *testing.T) {
	svc, repo := newSF(store.Storefront{SellerID: "seller-a", Slug: "shop-a", BannerURL: "https://old.example/b.png"})
	_, err := upsertSF(svc, store.Storefront{BannerURL: "https://other.example/b.png"})
	if !errors.Is(err, listing.ErrBannerInvalid) {
		t.Fatalf("err = %v, want ErrBannerInvalid", err)
	}
	if got := storedSF(t, repo).BannerURL; got != "https://old.example/b.png" {
		t.Fatalf("stored banner changed to %q", got)
	}
}

// Scenario: External banner URL is rejected (another seller's stored banner gives no exemption)
func TestUpsertOtherSellersBannerGivesNoExemption(t *testing.T) {
	svc, _ := newSF(store.Storefront{SellerID: "seller-b", Slug: "shop-b", BannerURL: "https://old.example/b.png"})
	_, err := upsertSF(svc, store.Storefront{BannerURL: "https://old.example/b.png"})
	if !errors.Is(err, listing.ErrBannerInvalid) {
		t.Fatalf("err = %v, want ErrBannerInvalid", err)
	}
}

// Scenario: Existing display name and slug rules still apply (validation order: tagline before theme)
func TestUpsertValidationOrderTaglineBeforeTheme(t *testing.T) {
	svc, _ := newSF()
	_, err := upsertSF(svc, store.Storefront{Tagline: "a\nb", Theme: "neon"})
	if !errors.Is(err, listing.ErrTaglineInvalid) {
		t.Fatalf("err = %v, want ErrTaglineInvalid first", err)
	}
	_, err = upsertSF(svc, store.Storefront{BannerURL: "https://evil.example/x.png", Theme: "neon"})
	if !errors.Is(err, listing.ErrBannerInvalid) {
		t.Fatalf("err = %v, want ErrBannerInvalid before theme", err)
	}
}
