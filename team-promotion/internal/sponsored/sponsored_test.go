package sponsored

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

func newSponsoredSvc() (*Service, *storefake.InMemoryAdCampaignRepository) {
	repo := storefake.NewInMemoryAdCampaignRepository()
	return New(repo, nil, auditx.NewGate(&recordingAuditWriter{}, nil), nil), repo
}

func TestCreateAdCampaignHappyPath(t *testing.T) {
	svc, _ := newSponsoredSvc()
	ctx := t.Context()

	c, err := svc.CreateAdCampaign(ctx, CreateAdCampaignParams{
		SellerID:  "seller-1",
		ListingID: "listing-1",
		Budget:    1000000,
		Bid:       5000,
		IsAdmin:   true,
	})
	if err != nil {
		t.Fatalf("CreateAdCampaign: %v", err)
	}
	if c.ID == "" {
		t.Fatal("expected generated id")
	}
	if c.SellerID != "seller-1" || c.ListingID != "listing-1" {
		t.Fatalf("unexpected campaign: %+v", c)
	}
	if c.Budget != 1000000 || c.Bid != 5000 {
		t.Fatalf("budget/bid not preserved: %+v", c)
	}
	if c.Status != store.AdCampaignStatusActive {
		t.Fatalf("status = %d, want active(%d)", c.Status, store.AdCampaignStatusActive)
	}
}

func TestListSponsoredSlotsBidOrdered(t *testing.T) {
	svc, _ := newSponsoredSvc()
	ctx := t.Context()

	mk := func(seller, listing string, bid int64) {
		if _, err := svc.CreateAdCampaign(ctx, CreateAdCampaignParams{
			SellerID: seller, ListingID: listing, Budget: 10 * bid, Bid: bid, IsAdmin: true,
		}); err != nil {
			t.Fatalf("CreateAdCampaign(%s): %v", listing, err)
		}
	}
	mk("seller-1", "listing-low", 100)
	mk("seller-2", "listing-high", 900)
	mk("seller-3", "listing-mid", 500)

	slots, err := svc.ListSponsoredSlots(ctx, "homepage")
	if err != nil {
		t.Fatalf("ListSponsoredSlots: %v", err)
	}
	want := []string{"listing-high", "listing-mid", "listing-low"}
	if len(slots) != len(want) {
		t.Fatalf("got %d slots, want %d: %v", len(slots), len(want), slots)
	}
	for i, w := range want {
		if slots[i] != w {
			t.Fatalf("slot[%d] = %q, want %q (full: %v)", i, slots[i], w, slots)
		}
	}
}

func TestListSponsoredSlotsEmpty(t *testing.T) {
	svc, _ := newSponsoredSvc()
	ctx := t.Context()

	slots, err := svc.ListSponsoredSlots(ctx, "")
	if err != nil {
		t.Fatalf("ListSponsoredSlots(empty): %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("want no slots, got %v", slots)
	}
}

func TestCreateAdCampaignValidation(t *testing.T) {
	svc, repo := newSponsoredSvc()
	ctx := t.Context()

	cases := []struct {
		name string
		p    CreateAdCampaignParams
	}{
		{"missing seller", CreateAdCampaignParams{SellerID: "", ListingID: "l1", Bid: 1}},
		{"missing listing", CreateAdCampaignParams{SellerID: "s1", ListingID: "", Bid: 1}},
		{"negative budget", CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Budget: -1}},
		{"negative bid", CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: -1}},
	}
	for _, tc := range cases {
		if _, err := svc.CreateAdCampaign(ctx, tc.p); !errors.Is(err, ErrInvalidAdCampaign) {
			t.Fatalf("%s: err = %v, want ErrInvalidAdCampaign", tc.name, err)
		}
	}

	slots, err := svc.ListSponsoredSlots(ctx, "")
	if err != nil {
		t.Fatalf("ListSponsoredSlots: %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("rejected creates leaked campaigns: %v", slots)
	}
	if active, _ := repo.ListActive(ctx, 10); len(active) != 0 {
		t.Fatalf("repo has %d active campaigns after rejects, want 0", len(active))
	}
}

func TestCreateAdCampaignCaps(t *testing.T) {
	ctx := t.Context()
	svc, repo := newSponsoredSvc()

	if _, err := svc.CreateAdCampaign(ctx, CreateAdCampaignParams{
		SellerID: "s1", ListingID: "l1", Bid: DefaultMaxAdBid, Budget: DefaultMaxAdBudget, IsAdmin: true,
	}); err != nil {
		t.Fatalf("at-limit: %v", err)
	}
	for name, p := range map[string]CreateAdCampaignParams{
		"bid above cap":    {SellerID: "s1", ListingID: "l2", Bid: DefaultMaxAdBid + 1, IsAdmin: true},
		"budget above cap": {SellerID: "s1", ListingID: "l3", Budget: DefaultMaxAdBudget + 1, IsAdmin: true},
		"huge bid":         {SellerID: "s1", ListingID: "l4", Bid: 1 << 62, IsAdmin: true},
	} {
		if _, err := svc.CreateAdCampaign(ctx, p); !errors.Is(err, ErrInvalidAdCampaign) {
			t.Fatalf("%s: err = %v, want ErrInvalidAdCampaign", name, err)
		}
	}
	if active, _ := repo.ListActive(ctx, 10); len(active) != 1 {
		t.Fatalf("repo has %d campaigns, want 1", len(active))
	}

	svc.WithLimits(10, 20)
	if _, err := svc.CreateAdCampaign(ctx, CreateAdCampaignParams{SellerID: "s1", ListingID: "l5", Bid: 11, IsAdmin: true}); !errors.Is(err, ErrInvalidAdCampaign) {
		t.Fatalf("custom bid cap: %v", err)
	}
	svc.WithLimits(0, 0) // non-positive keeps the configured value, never unbounded
	if _, err := svc.CreateAdCampaign(ctx, CreateAdCampaignParams{SellerID: "s1", ListingID: "l5", Bid: 11, IsAdmin: true}); !errors.Is(err, ErrInvalidAdCampaign) {
		t.Fatalf("zero limits must not unbound: %v", err)
	}
}

func TestCreateAdCampaignRequiresOwnership(t *testing.T) {
	owners := map[string]string{"mine": "s1", "theirs": "s2"}
	cases := []struct {
		name     string
		listings *fakeListings
		listing  string
		want     error
	}{
		{"own listing ok", &fakeListings{owners: owners}, "mine", nil},
		{"foreign listing denied", &fakeListings{owners: owners}, "theirs", listing.ErrNotOwner},
		{"listing not found", &fakeListings{owners: owners}, "ghost", listing.ErrNotFound},
		{"lookup failed", &fakeListings{err: errors.New("dial tcp: refused")}, "mine", listing.ErrLookupFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := storefake.NewInMemoryAdCampaignRepository()
			svc := New(repo, tc.listings, auditx.NewGate(&recordingAuditWriter{}, nil), nil)
			_, err := svc.CreateAdCampaign(t.Context(), CreateAdCampaignParams{SellerID: "s1", ListingID: tc.listing, Bid: 1})
			stored, _ := repo.ListActive(t.Context(), 10)
			if tc.want == nil {
				if err != nil || len(stored) != 1 {
					t.Fatalf("err = %v, stored = %d, want nil and 1", err, len(stored))
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(stored) != 0 {
				t.Fatalf("stored %d campaigns despite failed ownership check", len(stored))
			}
		})
	}
}

type orderedAdRepo struct {
	store.AdCampaignRepository
	log *callLog
}

func (r orderedAdRepo) Create(ctx context.Context, c store.AdCampaign) (store.AdCampaign, error) {
	r.log.add("insert")
	return r.AdCampaignRepository.Create(ctx, c)
}

func newAuditedAdSvc(t *testing.T, w auditx.Writer, listings *fakeListings, log *callLog) (*Service, *storefake.InMemoryAdCampaignRepository) {
	t.Helper()
	mem := storefake.NewInMemoryAdCampaignRepository()
	var lg listing.Getter
	if listings != nil {
		lg = listings
	}
	svc := New(orderedAdRepo{mem, log}, lg, auditx.NewGate(w, nil), nil).WithLimits(1000, 5000)
	return svc, mem
}

func adminAd(listingID string, budget, bid int64) CreateAdCampaignParams {
	return CreateAdCampaignParams{SellerID: "admin-1", ListingID: listingID, Budget: budget, Bid: bid, IsAdmin: true}
}

func TestCreateAdCampaignAuditBeforeInsertSameID(t *testing.T) {
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	svc, mem := newAuditedAdSvc(t, w, nil, log)
	c, err := svc.CreateAdCampaign(t.Context(), adminAd("list-1", 5000, 100))
	if err != nil {
		t.Fatalf("CreateAdCampaign: %v", err)
	}
	if got := log.get(); len(got) != 2 || got[0] != "audit" || got[1] != "insert" {
		t.Fatalf("call order = %v, want [audit insert]", got)
	}
	evs := w.got()
	if len(evs) != 1 {
		t.Fatalf("want exactly 1 audit event, got %d", len(evs))
	}
	ev := evs[0]
	if ev.Action != "ad_campaign.admin_create" || ev.TargetType != "ad_campaign" || ev.ActorID != "admin-1" {
		t.Errorf("event = %+v", ev)
	}
	if len(c.ID) != 32 || ev.TargetID != c.ID {
		t.Errorf("audit target_id = %q, returned id = %q; want identical 32-char id", ev.TargetID, c.ID)
	}
	m := ev.Metadata
	if len(m) != 3 || m["seller_id"] != c.SellerID || m["seller_id"] != "admin-1" || m["listing_id"] != "list-1" || m["budget"] != "5000" {
		t.Errorf("metadata = %v, want seller_id(=stored), listing_id, budget=5000", m)
	}
	if stored, err := mem.GetByID(t.Context(), ev.TargetID); err != nil || stored.ListingID != "list-1" {
		t.Errorf("stored campaign under audited id: %+v, %v", stored, err)
	}
}

func TestCreateAdCampaignAdminNoOwnerLookup(t *testing.T) {
	lg := &fakeListings{err: errors.New("team-domain down")}
	w := &recordingAuditWriter{}
	svc, _ := newAuditedAdSvc(t, w, lg, &callLog{})
	if _, err := svc.CreateAdCampaign(t.Context(), adminAd("list-1", 10, 1)); err != nil {
		t.Fatalf("CreateAdCampaign: %v", err)
	}
	if lg.calls != 0 || len(w.got()) != 1 {
		t.Fatalf("listing calls = %d, audit events = %d; want 0 and 1", lg.calls, len(w.got()))
	}
}

func TestCreateAdCampaignAuditFailureFailsClosed(t *testing.T) {
	for name, cause := range auditFailures {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			w := &recordingAuditWriter{err: cause, log: log}
			svc, mem := newAuditedAdSvc(t, w, nil, log)
			_, err := svc.CreateAdCampaign(t.Context(), adminAd("list-1", 10, 1))
			assertAuditUnavailable(t, err)
			for _, c := range log.get() {
				if c == "insert" {
					t.Fatal("campaign inserted despite audit failure")
				}
			}
			if active, _ := mem.ListActive(t.Context(), 10); len(active) != 0 {
				t.Fatalf("repo holds %d campaigns, want 0", len(active))
			}
		})
	}
}

func TestCreateAdCampaignBudgetBoundaryAudit(t *testing.T) {
	w := &recordingAuditWriter{}
	svc, mem := newAuditedAdSvc(t, w, nil, &callLog{})
	if _, err := svc.CreateAdCampaign(t.Context(), adminAd("l-max", 5000, 10)); err != nil {
		t.Fatalf("budget at max: %v", err)
	}
	if n := len(w.got()); n != 1 {
		t.Fatalf("at max: audit events = %d, want 1", n)
	}
	if _, err := svc.CreateAdCampaign(t.Context(), adminAd("l-over", 5001, 10)); !errors.Is(err, ErrInvalidAdCampaign) {
		t.Fatalf("budget over max: err = %v, want ErrInvalidAdCampaign", err)
	}
	if n := len(w.got()); n != 1 {
		t.Fatalf("over max must not be audited: events = %d, want still 1", n)
	}
	if active, _ := mem.ListActive(t.Context(), 10); len(active) != 1 {
		t.Fatalf("repo holds %d campaigns, want 1", len(active))
	}
}

func TestCreateAdCampaignSellerNotAudited(t *testing.T) {
	owners := map[string]string{"mine": "seller-1"}
	p := CreateAdCampaignParams{SellerID: "seller-1", ListingID: "mine", Budget: 100, Bid: 10}
	for name, w := range map[string]*recordingAuditWriter{"writer wired": {}, "writer failing": {err: errors.New("down")}} {
		t.Run(name, func(t *testing.T) {
			svc, _ := newAuditedAdSvc(t, w, &fakeListings{owners: owners}, &callLog{})
			if _, err := svc.CreateAdCampaign(t.Context(), p); err != nil {
				t.Fatalf("seller ad campaign: %v", err)
			}
			if n := len(w.got()); n != 0 {
				t.Fatalf("seller path wrote %d audit events, want 0", n)
			}
		})
	}
}
