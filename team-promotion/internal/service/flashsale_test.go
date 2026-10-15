package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/producer"
	"github.com/buidangphuc/team-promotion/internal/repository"
	"github.com/buidangphuc/team-promotion/internal/upstream"
)

// stubFlags is a fixed-answer feature-flag evaluator for tests.
type stubFlags struct{ enabled bool }

func (s stubFlags) BooleanEnabled(_ context.Context, _ string, _ bool) bool { return s.enabled }

func newFlashSvc(t *testing.T, flagEnabled bool) (*FlashSaleService, *capturePublisher) {
	t.Helper()
	repo := repository.NewInMemoryFlashSaleRepository()
	svc := NewFlashSaleService(repo, nil, stubFlags{enabled: flagEnabled}, nil, nil)
	svc.WithAudit(permissiveAudit{}) // existing tests create admin campaigns
	pub := &capturePublisher{}
	svc.emitter = producer.NewEmitter(pub, nil)
	return svc, pub
}

func TestCreateCampaignEmitsEvent(t *testing.T) {
	svc, pub := newFlashSvc(t, true)
	c, err := svc.CreateCampaign(context.Background(), CreateCampaignParams{
		ListingID: "list-1",
		SalePrice: 90000,
		StockCap:  100,
		StartsAt:  time.Now().Add(-time.Hour),
		EndsAt:    time.Now().Add(time.Hour),
		IsAdmin:   true,
	})
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if c.ID == "" {
		t.Fatal("expected generated campaign id")
	}
	if pub.count() != 1 {
		t.Fatalf("expected 1 promotion.events record, got %d", pub.count())
	}
}

func TestGetFlashSaleStockRemaining(t *testing.T) {
	svc, _ := newFlashSvc(t, true)
	ctx := context.Background()
	repo := svc.campaigns.(*repository.InMemoryFlashSaleRepository)
	c, _ := repo.Create(ctx, repository.FlashSaleCampaign{ListingID: "l", StockCap: 100, StockSold: 30})

	remaining, stockCap, err := svc.GetFlashSaleStock(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	if remaining != 70 || stockCap != 100 {
		t.Fatalf("remaining=%d cap=%d, want 70/100", remaining, stockCap)
	}

	// Oversold never goes negative.
	c2, _ := repo.Create(ctx, repository.FlashSaleCampaign{ListingID: "l2", StockCap: 10, StockSold: 25})
	remaining, _, err = svc.GetFlashSaleStock(ctx, c2.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock oversold: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining=%d, want 0 (floored)", remaining)
	}
}

func TestGetActiveFlashSaleWindow(t *testing.T) {
	svc, _ := newFlashSvc(t, true)
	ctx := context.Background()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return now }

	// Active campaign (window brackets now).
	active, _ := svc.CreateCampaign(ctx, CreateCampaignParams{
		ListingID: "live", SalePrice: 1, StockCap: 5, IsAdmin: true,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	// Expired campaign for a different listing.
	_, _ = svc.CreateCampaign(ctx, CreateCampaignParams{
		ListingID: "dead", SalePrice: 1, StockCap: 5, IsAdmin: true,
		StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-time.Hour),
	})

	got, ok, err := svc.GetActiveFlashSale(ctx, "live")
	if err != nil {
		t.Fatalf("GetActiveFlashSale live: %v", err)
	}
	if !ok || got.ID != active.ID {
		t.Fatalf("expected active campaign %s, got ok=%v id=%s", active.ID, ok, got.ID)
	}

	_, ok, err = svc.GetActiveFlashSale(ctx, "dead")
	if err != nil {
		t.Fatalf("GetActiveFlashSale dead: %v", err)
	}
	if ok {
		t.Fatal("expired campaign must not be active")
	}
}

func TestFlashSaleKillSwitchSuppresses(t *testing.T) {
	svc, _ := newFlashSvc(t, false) // flag OFF
	ctx := context.Background()
	now := time.Now()
	svc.nowFn = func() time.Time { return now }
	_, _ = svc.CreateCampaign(ctx, CreateCampaignParams{
		ListingID: "live", SalePrice: 1, StockCap: 5, IsAdmin: true,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})

	_, ok, err := svc.GetActiveFlashSale(ctx, "live")
	if err != nil {
		t.Fatalf("GetActiveFlashSale: %v", err)
	}
	if ok {
		t.Fatal("kill-switch OFF must suppress the active flash sale")
	}
}

// fakeListings is an in-memory team-domain listing client: id -> seller id.
type fakeListings struct {
	owners map[string]string
	err    error
	calls  int
}

func (f *fakeListings) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	seller, ok := f.owners[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), SellerId: seller}}, nil
}

func TestCreateCampaignOwnership(t *testing.T) {
	owners := map[string]string{"mine": "seller-1", "theirs": "seller-2"}
	cases := []struct {
		name     string
		listings *fakeListings // nil interface when unset
		unset    bool
		params   CreateCampaignParams
		want     codes.Code
	}{
		{"own listing ok", &fakeListings{owners: owners}, false, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, codes.OK},
		{"foreign listing denied", &fakeListings{owners: owners}, false, CreateCampaignParams{ListingID: "theirs", CallerID: "seller-1"}, codes.PermissionDenied},
		{"listing not found", &fakeListings{owners: owners}, false, CreateCampaignParams{ListingID: "ghost", CallerID: "seller-1"}, codes.InvalidArgument},
		{"upstream error", &fakeListings{err: status.Error(codes.Internal, "boom")}, false, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, codes.Unavailable},
		{"upstream down", &fakeListings{err: errors.New("dial tcp: refused")}, false, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, codes.Unavailable},
		{"admin skips lookup", &fakeListings{owners: owners}, false, CreateCampaignParams{ListingID: "theirs", CallerID: "admin-1", IsAdmin: true}, codes.OK},
		{"unset fails closed for seller", nil, true, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, codes.Unavailable},
		{"unset still allows admin", nil, true, CreateCampaignParams{ListingID: "mine", CallerID: "admin-1", IsAdmin: true}, codes.OK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newFlashSvc(t, true)
			if !tc.unset {
				svc.listings = tc.listings
			}
			_, err := svc.CreateCampaign(context.Background(), tc.params)
			if got := status.Code(err); got != tc.want {
				t.Fatalf("code = %v, want %v (err=%v)", got, tc.want, err)
			}
			if tc.params.IsAdmin && tc.listings != nil && tc.listings.calls != 0 {
				t.Fatalf("admin must not trigger an ownership lookup")
			}
		})
	}
}

// ---- promotion-admin-audit -------------------------------------------------

type orderedFlashRepo struct {
	repository.FlashSaleRepository
	log *callLog
}

func (r orderedFlashRepo) Create(ctx context.Context, c repository.FlashSaleCampaign) (repository.FlashSaleCampaign, error) {
	r.log.add("insert")
	return r.FlashSaleRepository.Create(ctx, c)
}

func newAuditedFlashSvc(t *testing.T, w AuditWriter, listings *fakeListings, log *callLog) (*FlashSaleService, *repository.InMemoryFlashSaleRepository, *capturePublisher) {
	t.Helper()
	mem := repository.NewInMemoryFlashSaleRepository()
	var lg upstreamListings
	if listings != nil {
		lg = listings
	}
	svc := NewFlashSaleService(orderedFlashRepo{mem, log}, nil, stubFlags{enabled: true}, lg, nil)
	if w != nil {
		svc.WithAudit(w)
	}
	pub := &capturePublisher{}
	svc.emitter = producer.NewEmitter(pub, nil)
	return svc, mem, pub
}

func adminCampaign(listing string) CreateCampaignParams {
	return CreateCampaignParams{
		ListingID: listing, SalePrice: 90000, StockCap: 10,
		StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour),
		CallerID: "admin-1", IsAdmin: true,
	}
}

// Scenario: Promotion audit precedes the campaign insert and carries the final id.
func TestCreateCampaignAuditBeforeInsertSameID(t *testing.T) {
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	svc, mem, _ := newAuditedFlashSvc(t, w, nil, log)
	c, err := svc.CreateCampaign(context.Background(), adminCampaign("list-1"))
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if got := log.get(); len(got) != 2 || got[0] != "audit" || got[1] != "insert" {
		t.Fatalf("call order = %v, want [audit insert]", got)
	}
	evs := w.got()
	if len(evs) != 1 {
		t.Fatalf("want exactly 1 audit event, got %d", len(evs))
	}
	ev := evs[0]
	if ev.Action != "flash_sale.admin_create" || ev.TargetType != "flash_sale" || ev.ActorID != "admin-1" {
		t.Errorf("event = %+v", ev)
	}
	if len(c.ID) != 32 || ev.TargetID != c.ID {
		t.Errorf("audit target_id = %q, returned id = %q; want identical 32-char id", ev.TargetID, c.ID)
	}
	if len(ev.Metadata) != 1 || ev.Metadata["listing_id"] != "list-1" {
		t.Errorf("metadata = %v, want listing_id only", ev.Metadata)
	}
	stored, err := mem.GetByID(context.Background(), ev.TargetID)
	if err != nil || stored.ListingID != "list-1" {
		t.Errorf("stored campaign under audited id: %+v, %v", stored, err)
	}
}

// Scenario: Admin flash sale makes no owner lookup.
func TestCreateCampaignAdminNoOwnerLookup(t *testing.T) {
	for name, lg := range map[string]*fakeListings{
		"nil client":     nil,
		"erroring":       {err: errors.New("team-domain down")},
		"not-owner data": {owners: map[string]string{"list-1": "someone-else"}},
	} {
		t.Run(name, func(t *testing.T) {
			w := &recordingAuditWriter{}
			svc, _, _ := newAuditedFlashSvc(t, w, lg, &callLog{})
			if _, err := svc.CreateCampaign(context.Background(), adminCampaign("list-1")); err != nil {
				t.Fatalf("CreateCampaign: %v", err)
			}
			if lg != nil && lg.calls != 0 {
				t.Fatalf("admin path called the listing client %d times", lg.calls)
			}
			if n := len(w.got()); n != 1 {
				t.Fatalf("audit events = %d, want exactly 1", n)
			}
		})
	}
}

// Scenario: Promotion audit failure emits nothing for a flash sale.
func TestCreateCampaignAuditFailureFailsClosed(t *testing.T) {
	cases := map[string]AuditWriter{"nil writer": nil}
	for name, cause := range auditFailures {
		cases[name] = &recordingAuditWriter{err: cause}
	}
	for name, w := range cases {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			if rw, ok := w.(*recordingAuditWriter); ok {
				rw.log = log
			}
			svc, mem, pub := newAuditedFlashSvc(t, w, nil, log)
			_, err := svc.CreateCampaign(context.Background(), adminCampaign("list-1"))
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("code = %v, want Unavailable (err=%v)", status.Code(err), err)
			}
			for _, c := range log.get() {
				if c == "insert" {
					t.Fatal("campaign inserted despite audit failure")
				}
			}
			if items, _ := mem.ListActive(context.Background(), time.Now(), 10, 0); len(items) != 0 {
				t.Fatalf("repo holds %d campaigns, want 0", len(items))
			}
			if pub.count() != 0 {
				t.Fatalf("FlashSaleChanged emitted %d times, want 0", pub.count())
			}
		})
	}
}

// Scenario: Seller creates a flash sale on their own listing (never audited, never
// dependent on audit availability).
func TestCreateCampaignSellerNotAudited(t *testing.T) {
	owners := map[string]string{"mine": "seller-1"}
	for name, w := range map[string]*recordingAuditWriter{"writer wired": {}, "writer failing": {err: errors.New("down")}} {
		t.Run(name, func(t *testing.T) {
			svc, _, _ := newAuditedFlashSvc(t, w, &fakeListings{owners: owners}, &callLog{})
			p := adminCampaign("mine")
			p.CallerID, p.IsAdmin = "seller-1", false
			if _, err := svc.CreateCampaign(context.Background(), p); err != nil {
				t.Fatalf("seller campaign: %v", err)
			}
			if n := len(w.got()); n != 0 {
				t.Fatalf("seller path wrote %d audit events, want 0", n)
			}
		})
	}
	svc, _, _ := newAuditedFlashSvc(t, nil, &fakeListings{owners: owners}, &callLog{})
	p := adminCampaign("mine")
	p.CallerID, p.IsAdmin = "seller-1", false
	if _, err := svc.CreateCampaign(context.Background(), p); err != nil {
		t.Fatalf("seller campaign with nil audit: %v", err)
	}
}

// Scenario: Invalid campaign writes no event (service half).
func TestCreateCampaignInvalidNotAudited(t *testing.T) {
	w := &recordingAuditWriter{}
	svc, _, _ := newAuditedFlashSvc(t, w, nil, &callLog{})
	if _, err := svc.CreateCampaign(context.Background(), adminCampaign("  ")); !errors.Is(err, ErrInvalidCampaign) {
		t.Fatalf("err = %v, want ErrInvalidCampaign", err)
	}
	if n := len(w.got()); n != 0 {
		t.Fatalf("invalid campaign wrote %d events", n)
	}
}

// Scenario: Promotion audit precedes the campaign insert and carries the final id
// (the generated id is a fresh 32-char hex string per call).
func TestNewCampaignIDIsUnique32Hex(t *testing.T) {
	a, b := newCampaignID(), newCampaignID()
	if len(a) != 32 || len(b) != 32 || a == b {
		t.Fatalf("newCampaignID() = %q, %q; want two distinct 32-char ids", a, b)
	}
	for _, r := range a + b {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("non-hex char %q in id", r)
		}
	}
}

// upstreamListings mirrors upstream.ListingGetter for the nil-interface dance.
type upstreamListings = upstream.ListingGetter
