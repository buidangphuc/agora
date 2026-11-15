package sponsored

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

var epoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type spyRepo struct {
	store.AdCampaignRepository
	mu      sync.Mutex
	issued  [][]store.ImpressionIssue
	failErr error
}

func (r *spyRepo) IssueImpressions(ctx context.Context, issues []store.ImpressionIssue) error {
	r.mu.Lock()
	r.issued = append(r.issued, append([]store.ImpressionIssue(nil), issues...))
	r.mu.Unlock()
	if r.failErr != nil {
		return r.failErr
	}
	return r.AdCampaignRepository.IssueImpressions(ctx, issues)
}

func (r *spyRepo) issueCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.issued)
}

func (r *spyRepo) tokens() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, batch := range r.issued {
		for _, i := range batch {
			out = append(out, i.Token)
		}
	}
	return out
}

func newServingSvc(t *testing.T) (*Service, *spyRepo, *storefake.InMemoryAdCampaignRepository) {
	t.Helper()
	mem := storefake.NewInMemoryAdCampaignRepository()
	spy := &spyRepo{AdCampaignRepository: mem}
	svc := New(spy, nil, auditx.NewGate(&recordingAuditWriter{}, nil), nil)
	return svc, spy, mem
}

type seed struct {
	id, listing, category string
	bid, budget, spent    int64
	status                int32
	age                   time.Duration
}

func put(t *testing.T, repo store.AdCampaignRepository, s seed) {
	t.Helper()
	if s.budget == 0 {
		s.budget = 100 * s.bid
	}
	if s.status == 0 {
		s.status = store.AdCampaignStatusActive
	}
	if s.listing == "" {
		s.listing = "listing-" + s.id
	}
	if _, err := repo.Create(t.Context(), store.AdCampaign{
		ID: s.id, SellerID: "seller-" + s.id, ListingID: s.listing, CategoryID: s.category,
		Bid: s.bid, Budget: s.budget, Spent: s.spent, Status: s.status, CreatedAt: epoch.Add(-s.age),
	}); err != nil {
		t.Fatalf("seed %s: %v", s.id, err)
	}
}

func campaignIDs(slots []Slot) []string {
	out := make([]string, len(slots))
	for i, s := range slots {
		out[i] = s.CampaignID
	}
	return out
}

func wantIDs(t *testing.T, got []Slot, want ...string) {
	t.Helper()
	g := campaignIDs(got)
	if len(g) != len(want) {
		t.Fatalf("campaign ids = %v, want %v", g, want)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Fatalf("campaign ids = %v, want %v", g, want)
		}
	}
}

func listSlots(t *testing.T, svc *Service, p SlotsParams) []Slot {
	t.Helper()
	res, err := svc.ListSponsoredSlots(t.Context(), p)
	if err != nil {
		t.Fatalf("ListSponsoredSlots(%+v): %v", p, err)
	}
	return res.Slots
}

func TestCreateAdCampaignMinBidAccepted(t *testing.T) {
	repo := storefake.NewInMemoryAdCampaignRepository()
	lg := &fakeListings{owners: map[string]string{"l1": "s1"}, categories: map[string]string{"l1": "cat-c"}}
	svc := New(repo, lg, auditx.NewGate(&recordingAuditWriter{}, nil), nil).WithMinBid(500)
	c, err := svc.CreateAdCampaign(t.Context(), CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: 500, Budget: 5000})
	if err != nil {
		t.Fatalf("CreateAdCampaign: %v", err)
	}
	if c.Status != store.AdCampaignStatusActive || c.CategoryID != "cat-c" || c.Spent != 0 || c.Impressions != 0 || c.Clicks != 0 {
		t.Fatalf("campaign = %+v, want ACTIVE, category cat-c, counters 0", c)
	}
	stored, err := repo.GetByID(t.Context(), c.ID)
	if err != nil || stored.CategoryID != "cat-c" {
		t.Fatalf("stored = %+v err=%v, want category cat-c persisted", stored, err)
	}
}

func TestCreateAdCampaignBelowMinBidRejected(t *testing.T) {
	repo := storefake.NewInMemoryAdCampaignRepository()
	lg := &fakeListings{owners: map[string]string{"l1": "s1"}}
	svc := New(repo, lg, auditx.NewGate(&recordingAuditWriter{}, nil), nil).WithMinBid(500)
	_, err := svc.CreateAdCampaign(t.Context(), CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: 499, Budget: 5000})
	if !errors.Is(err, ErrInvalidAdCampaign) {
		t.Fatalf("err = %v, want ErrInvalidAdCampaign", err)
	}
	if rows, _ := repo.ListBySeller(t.Context(), "s1", 10, nil); len(rows) != 0 {
		t.Fatalf("%d rows written, want 0", len(rows))
	}
}

func TestCreateAdCampaignDefaultMinBidIs500(t *testing.T) {
	repo := storefake.NewInMemoryAdCampaignRepository()
	lg := &fakeListings{owners: map[string]string{"l1": "s1"}}
	svc := New(repo, lg, auditx.NewGate(&recordingAuditWriter{}, nil), nil)
	if _, err := svc.CreateAdCampaign(t.Context(), CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: 499, Budget: 5000}); !errors.Is(err, ErrInvalidAdCampaign) {
		t.Fatalf("bid 499 with default minimum: err = %v, want ErrInvalidAdCampaign", err)
	}
	if _, err := svc.CreateAdCampaign(t.Context(), CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: 500, Budget: 5000}); err != nil {
		t.Fatalf("bid 500 with default minimum: %v", err)
	}
}

func TestCreateAdCampaignBudgetBelowBidRejected(t *testing.T) {
	repo := storefake.NewInMemoryAdCampaignRepository()
	lg := &fakeListings{owners: map[string]string{"l1": "s1"}}
	svc := New(repo, lg, auditx.NewGate(&recordingAuditWriter{}, nil), nil).WithMinBid(500)
	_, err := svc.CreateAdCampaign(t.Context(), CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: 1000, Budget: 999})
	if !errors.Is(err, ErrInvalidAdCampaign) {
		t.Fatalf("err = %v, want ErrInvalidAdCampaign", err)
	}
	if rows, _ := repo.ListBySeller(t.Context(), "s1", 10, nil); len(rows) != 0 {
		t.Fatalf("%d rows written, want 0", len(rows))
	}
	if _, err := svc.CreateAdCampaign(t.Context(), CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: 1000, Budget: 1000}); err != nil {
		t.Fatalf("budget equal to bid must be accepted: %v", err)
	}
}

func TestCreateAdCampaignForeignListingDenied(t *testing.T) {
	repo := storefake.NewInMemoryAdCampaignRepository()
	lg := &fakeListings{owners: map[string]string{"l1": "other-seller"}}
	svc := New(repo, lg, auditx.NewGate(&recordingAuditWriter{}, nil), nil).WithMinBid(500)
	_, err := svc.CreateAdCampaign(t.Context(), CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: 500, Budget: 5000})
	if !errors.Is(err, listing.ErrNotOwner) {
		t.Fatalf("err = %v, want listing.ErrNotOwner", err)
	}
	if rows, _ := repo.ListBySeller(t.Context(), "s1", 10, nil); len(rows) != 0 {
		t.Fatalf("%d rows written, want 0", len(rows))
	}
}

func TestListSlotsSearchRanksByBidInCategory(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "b500", category: "C", bid: 500})
	put(t, mem, seed{id: "b900", category: "C", bid: 900})
	put(t, mem, seed{id: "b700", category: "C", bid: 700})
	got := listSlots(t, svc, SlotsParams{Placement: "search", CategoryID: "C", Limit: 2})
	wantIDs(t, got, "b900", "b700")
	if got[0].ListingID != "listing-b900" || got[0].Token == "" {
		t.Fatalf("slot[0] = %+v, want listing-b900 with a token", got[0])
	}
}

func TestListSlotsTieBreaksOldestFirst(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "newer", category: "C", bid: 800, age: time.Hour})
	put(t, mem, seed{id: "older", category: "C", bid: 800, age: 48 * time.Hour})
	wantIDs(t, listSlots(t, svc, SlotsParams{Placement: "search", CategoryID: "C", Limit: 2}), "older", "newer")
}

func TestListSlotsSearchExcludesOtherCategory(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "d-high", category: "D", bid: 1000})
	put(t, mem, seed{id: "c-low", category: "C", bid: 600})
	wantIDs(t, listSlots(t, svc, SlotsParams{Placement: "search", CategoryID: "C", Limit: 4}), "c-low")
}

func TestListSlotsSearchRequiresCategory(t *testing.T) {
	svc, spy, mem := newServingSvc(t)
	put(t, mem, seed{id: "a", category: "C", bid: 900})
	put(t, mem, seed{id: "home-only", category: "", bid: 800})
	res, err := svc.ListSponsoredSlots(t.Context(), SlotsParams{Placement: "search", Limit: 4})
	if err != nil {
		t.Fatalf("ListSponsoredSlots: %v", err)
	}
	if len(res.Slots) != 0 || spy.issueCalls() != 0 {
		t.Fatalf("slots = %v, issue calls = %d; want none", res.Slots, spy.issueCalls())
	}
}

func TestListSlotsHomeIgnoresCategory(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "a", category: "A", bid: 600})
	put(t, mem, seed{id: "b", category: "B", bid: 950})
	put(t, mem, seed{id: "c", category: "C", bid: 800})
	put(t, mem, seed{id: "none", category: "", bid: 700})
	wantIDs(t, listSlots(t, svc, SlotsParams{Placement: "home", CategoryID: "A", Limit: 2}), "b", "c")
}

func TestListSlotsDedupesListing(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "dup-high", listing: "L1", category: "C", bid: 900})
	put(t, mem, seed{id: "dup-low", listing: "L1", category: "C", bid: 600})
	put(t, mem, seed{id: "other", listing: "L2", category: "C", bid: 500})
	got := listSlots(t, svc, SlotsParams{Placement: "search", CategoryID: "C", Limit: 4})
	wantIDs(t, got, "dup-high", "other")
}

func TestListSlotsDedupedListingStillFillsLimit(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "x1", listing: "L1", category: "C", bid: 900})
	put(t, mem, seed{id: "x2", listing: "L1", category: "C", bid: 890})
	put(t, mem, seed{id: "x3", listing: "L1", category: "C", bid: 880})
	put(t, mem, seed{id: "y", listing: "L2", category: "C", bid: 500})
	wantIDs(t, listSlots(t, svc, SlotsParams{Placement: "search", CategoryID: "C", Limit: 2}), "x1", "y")
}

func TestListSlotsLimitBounds(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	for i := 0; i < 6; i++ {
		put(t, mem, seed{id: fmt.Sprintf("c%d", i), category: "C", bid: int64(600 + i)})
	}
	if got := listSlots(t, svc, SlotsParams{Placement: "home", Limit: 4}); len(got) != 4 {
		t.Fatalf("limit 4 returned %d slots, want 4", len(got))
	}
	if got := listSlots(t, svc, SlotsParams{Placement: "home", Limit: 1}); len(got) != 1 {
		t.Fatalf("limit 1 returned %d slots, want 1", len(got))
	}
	if got := listSlots(t, svc, SlotsParams{Placement: "home", Limit: 0}); len(got) != 2 {
		t.Fatalf("limit 0 returned %d slots, want default 2", len(got))
	}
	for _, lim := range []int32{5, -1} {
		if _, err := svc.ListSponsoredSlots(t.Context(), SlotsParams{Placement: "home", Limit: lim}); !errors.Is(err, ErrInvalidAdCampaign) {
			t.Fatalf("limit %d: err = %v, want ErrInvalidAdCampaign", lim, err)
		}
	}
}

func TestListSlotsUnknownPlacementRejected(t *testing.T) {
	svc, spy, mem := newServingSvc(t)
	put(t, mem, seed{id: "a", category: "C", bid: 900})
	_, err := svc.ListSponsoredSlots(t.Context(), SlotsParams{Placement: "sidebar", CategoryID: "C", Limit: 2})
	if !errors.Is(err, ErrInvalidAdCampaign) {
		t.Fatalf("err = %v, want ErrInvalidAdCampaign", err)
	}
	if spy.issueCalls() != 0 {
		t.Fatalf("rejected call recorded impressions")
	}
}

func TestListSlotsLegacyContextOnly(t *testing.T) {
	svc, spy, mem := newServingSvc(t)
	put(t, mem, seed{id: "low", listing: "L-low", category: "C", bid: 600})
	put(t, mem, seed{id: "dup-high", listing: "L-dup", category: "D", bid: 900})
	put(t, mem, seed{id: "dup-low", listing: "L-dup", category: "D", bid: 700})
	put(t, mem, seed{id: "none", listing: "L-none", category: "", bid: 800})
	res, err := svc.ListSponsoredSlots(t.Context(), SlotsParams{ContextStr: "homepage"})
	if err != nil {
		t.Fatalf("ListSponsoredSlots: %v", err)
	}
	want := []string{"L-dup", "L-none", "L-low"}
	if fmt.Sprint(res.ListingIDs) != fmt.Sprint(want) {
		t.Fatalf("listing ids = %v, want %v", res.ListingIDs, want)
	}
	if len(res.Slots) != 0 || spy.issueCalls() != 0 {
		t.Fatalf("legacy call returned slots %v / recorded %d impression batches", res.Slots, spy.issueCalls())
	}
	for _, id := range []string{"low", "dup-high", "none"} {
		if c, _ := mem.GetByID(t.Context(), id); c.Impressions != 0 {
			t.Fatalf("campaign %s impressions = %d, want 0", id, c.Impressions)
		}
	}
}

func TestListSlotsLegacyCapsAtFifty(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	for i := 0; i < 60; i++ {
		put(t, mem, seed{id: fmt.Sprintf("c%02d", i), category: "C", bid: int64(500 + i)})
	}
	res, err := svc.ListSponsoredSlots(t.Context(), SlotsParams{})
	if err != nil {
		t.Fatalf("ListSponsoredSlots: %v", err)
	}
	if len(res.ListingIDs) != 50 || res.ListingIDs[0] != "listing-c59" {
		t.Fatalf("got %d ids starting %v, want 50 starting listing-c59", len(res.ListingIDs), res.ListingIDs[:1])
	}
}

func TestListSlotsIssuesUnguessableTokens(t *testing.T) {
	svc, spy, mem := newServingSvc(t)
	put(t, mem, seed{id: "a", category: "C", bid: 900})
	put(t, mem, seed{id: "b", category: "C", bid: 800})
	got := listSlots(t, svc, SlotsParams{Placement: "search", CategoryID: "C", Limit: 2})
	if len(got) != 2 {
		t.Fatalf("got %d slots, want 2", len(got))
	}
	if got[0].Token == got[1].Token {
		t.Fatalf("tokens not distinct: %q", got[0].Token)
	}
	for _, s := range got {
		raw, err := base64.RawURLEncoding.DecodeString(s.Token)
		if err != nil || len(raw) < 16 {
			t.Fatalf("token %q: decoded %d bytes err=%v, want base64url of at least 16 bytes", s.Token, len(raw), err)
		}
	}
	if spy.issueCalls() != 1 {
		t.Fatalf("issue calls = %d, want one batched call", spy.issueCalls())
	}
	issued := spy.tokens()
	if len(issued) != 2 || issued[0] != got[0].Token || issued[1] != got[1].Token {
		t.Fatalf("persisted tokens %v differ from served %v", issued, []string{got[0].Token, got[1].Token})
	}
}

func TestServeEmptyWritesNothing(t *testing.T) {
	svc, spy, mem := newServingSvc(t)
	put(t, mem, seed{id: "a", category: "C", bid: 900})
	if got := listSlots(t, svc, SlotsParams{Placement: "home", Limit: 2}); len(got) != 1 {
		t.Fatalf("positive control served %d slots, want 1", len(got))
	}
	before := spy.issueCalls()
	if before != 1 {
		t.Fatalf("positive control issue calls = %d, want 1", before)
	}
	if got := listSlots(t, svc, SlotsParams{Placement: "search", CategoryID: "OTHER", Limit: 2}); len(got) != 0 {
		t.Fatalf("empty case served %v", got)
	}
	if spy.issueCalls() != before {
		t.Fatalf("empty result wrote impressions: calls %d -> %d", before, spy.issueCalls())
	}
}

func TestServeFailedTokenInsertFailsCallUnavailable(t *testing.T) {
	svc, spy, mem := newServingSvc(t)
	put(t, mem, seed{id: "a", category: "C", bid: 900})
	spy.failErr = errors.New("insert ad_impressions: connection reset")
	res, err := svc.ListSponsoredSlots(t.Context(), SlotsParams{Placement: "home", Limit: 2})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v, want gRPC Unavailable", err)
	}
	if len(res.Slots) != 0 || len(res.ListingIDs) != 0 {
		t.Fatalf("failed call returned %+v, want nothing", res)
	}
}
