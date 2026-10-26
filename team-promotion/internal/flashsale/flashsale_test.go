package flashsale

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

func newFlashSvc(t *testing.T, flagEnabled bool) (*Service, *eventLog) {
	t.Helper()
	repo := storefake.NewInMemoryFlashSaleRepository()
	events := &eventLog{}
	svc := New(repo, nil, newFlags(t, flagProvider(flagEnabled)), nil, auditx.NewGate(&recordingAuditWriter{}, nil), events.build, nil)
	return svc, events
}

func TestCreateCampaignEmitsEvent(t *testing.T) {
	svc, events := newFlashSvc(t, true)
	ctx := reqctx.WithRequestID(t.Context(), "req-7")
	c, err := svc.CreateCampaign(ctx, CreateCampaignParams{
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
	if events.count() != 1 {
		t.Fatalf("expected 1 outbox row built, got %d", events.count())
	}
	if got := events.campaigns[0]; got.ID != c.ID || got.ListingID != "list-1" {
		t.Fatalf("builder received %+v, want the stored campaign %+v", got, c)
	}
	if got := events.rows[0]; got.AggregateID != c.ID || got.AggregateType != "flash_sale" {
		t.Fatalf("row = %+v, want aggregate flash_sale/%s", got, c.ID)
	}
	if events.reqIDs[0] != "req-7" {
		t.Fatalf("builder context request id = %q, want req-7", events.reqIDs[0])
	}
}

func TestCreateCampaignEventBuilderErrorStoresNothing(t *testing.T) {
	svc, events := newFlashSvc(t, true)
	events.err = errors.New("marshal failed")
	_, err := svc.CreateCampaign(t.Context(), CreateCampaignParams{
		ListingID: "list-1", SalePrice: 1, StockCap: 1, IsAdmin: true,
		StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Fatal("want the builder error, got nil")
	}
	if items, _ := svc.campaigns.ListActive(t.Context(), time.Now(), 10, 0); len(items) != 0 {
		t.Fatalf("repo holds %d campaigns, want 0", len(items))
	}
}

func TestGetFlashSaleStockRemaining(t *testing.T) {
	svc, _ := newFlashSvc(t, true)
	ctx := t.Context()
	repo := svc.campaigns.(*storefake.InMemoryFlashSaleRepository)
	c, _ := repo.Create(ctx, store.FlashSaleCampaign{ListingID: "l", StockCap: 100, StockSold: 30}, noEvent)

	remaining, stockCap, err := svc.GetFlashSaleStock(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	if remaining != 70 || stockCap != 100 {
		t.Fatalf("remaining=%d cap=%d, want 70/100", remaining, stockCap)
	}

	c2, _ := repo.Create(ctx, store.FlashSaleCampaign{ListingID: "l2", StockCap: 10, StockSold: 25}, noEvent)
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
	ctx := t.Context()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return now }

	active, _ := svc.CreateCampaign(ctx, CreateCampaignParams{
		ListingID: "live", SalePrice: 1, StockCap: 5, IsAdmin: true,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
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
	ctx := t.Context()
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
		listings *fakeListings
		params   CreateCampaignParams
		want     error
	}{
		{"own listing ok", &fakeListings{owners: owners}, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, nil},
		{"foreign listing denied", &fakeListings{owners: owners}, CreateCampaignParams{ListingID: "theirs", CallerID: "seller-1"}, listing.ErrNotOwner},
		{"listing not found", &fakeListings{owners: owners}, CreateCampaignParams{ListingID: "ghost", CallerID: "seller-1"}, listing.ErrNotFound},
		{"upstream error", &fakeListings{err: status.Error(codes.Internal, "boom")}, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, listing.ErrLookupFailed},
		{"upstream down", &fakeListings{err: errors.New("dial tcp: refused")}, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, listing.ErrLookupFailed},
		{"admin skips lookup", &fakeListings{owners: owners}, CreateCampaignParams{ListingID: "theirs", CallerID: "admin-1", IsAdmin: true}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newAuditedFlashSvc(t, &recordingAuditWriter{}, tc.listings, &callLog{})
			p := tc.params
			p.SalePrice, p.StockCap = 1, 1
			p.StartsAt, p.EndsAt = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
			_, err := svc.CreateCampaign(t.Context(), p)
			if tc.want == nil && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.params.IsAdmin && tc.listings.calls != 0 {
				t.Fatalf("admin must not trigger an ownership lookup")
			}
		})
	}
}

type orderedFlashRepo struct {
	store.FlashSaleRepository
	log *callLog
}

func (r orderedFlashRepo) Create(ctx context.Context, c store.FlashSaleCampaign, event func(store.FlashSaleCampaign) (outbox.Row, error)) (store.FlashSaleCampaign, error) {
	r.log.add("insert")
	return r.FlashSaleRepository.Create(ctx, c, event)
}

func newAuditedFlashSvc(t *testing.T, w auditx.Writer, listings *fakeListings, log *callLog) (*Service, *storefake.InMemoryFlashSaleRepository, *eventLog) {
	t.Helper()
	mem := storefake.NewInMemoryFlashSaleRepository()
	var lg listing.Getter
	if listings != nil {
		lg = listings
	}
	events := &eventLog{}
	svc := New(orderedFlashRepo{mem, log}, nil, newFlags(t, flagProvider(true)), lg, auditx.NewGate(w, nil), events.build, nil)
	return svc, mem, events
}

func adminCampaign(listingID string) CreateCampaignParams {
	return CreateCampaignParams{
		ListingID: listingID, SalePrice: 90000, StockCap: 10,
		StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour),
		CallerID: "admin-1", IsAdmin: true,
	}
}

func TestCreateCampaignAuditBeforeInsertSameID(t *testing.T) {
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	svc, mem, _ := newAuditedFlashSvc(t, w, nil, log)
	c, err := svc.CreateCampaign(t.Context(), adminCampaign("list-1"))
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
	stored, err := mem.GetByID(t.Context(), ev.TargetID)
	if err != nil || stored.ListingID != "list-1" {
		t.Errorf("stored campaign under audited id: %+v, %v", stored, err)
	}
}

func TestCreateCampaignAdminNoOwnerLookup(t *testing.T) {
	for name, lg := range map[string]*fakeListings{
		"erroring":       {err: errors.New("team-domain down")},
		"not-owner data": {owners: map[string]string{"list-1": "someone-else"}},
	} {
		t.Run(name, func(t *testing.T) {
			w := &recordingAuditWriter{}
			svc, _, _ := newAuditedFlashSvc(t, w, lg, &callLog{})
			if _, err := svc.CreateCampaign(t.Context(), adminCampaign("list-1")); err != nil {
				t.Fatalf("CreateCampaign: %v", err)
			}
			if lg.calls != 0 {
				t.Fatalf("admin path called the listing client %d times", lg.calls)
			}
			if n := len(w.got()); n != 1 {
				t.Fatalf("audit events = %d, want exactly 1", n)
			}
		})
	}
}

func TestCreateCampaignAuditFailureFailsClosed(t *testing.T) {
	for name, cause := range auditFailures {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			w := &recordingAuditWriter{err: cause, log: log}
			svc, mem, events := newAuditedFlashSvc(t, w, nil, log)
			_, err := svc.CreateCampaign(t.Context(), adminCampaign("list-1"))
			assertAuditUnavailable(t, err)
			for _, c := range log.get() {
				if c == "insert" {
					t.Fatal("campaign inserted despite audit failure")
				}
			}
			if items, _ := mem.ListActive(t.Context(), time.Now(), 10, 0); len(items) != 0 {
				t.Fatalf("repo holds %d campaigns, want 0", len(items))
			}
			if events.count() != 0 {
				t.Fatalf("FlashSaleChanged built %d times, want 0", events.count())
			}
		})
	}
}

func TestCreateCampaignSellerNotAudited(t *testing.T) {
	owners := map[string]string{"mine": "seller-1"}
	for name, w := range map[string]*recordingAuditWriter{"writer wired": {}, "writer failing": {err: errors.New("down")}} {
		t.Run(name, func(t *testing.T) {
			svc, _, _ := newAuditedFlashSvc(t, w, &fakeListings{owners: owners}, &callLog{})
			p := adminCampaign("mine")
			p.CallerID, p.IsAdmin = "seller-1", false
			if _, err := svc.CreateCampaign(t.Context(), p); err != nil {
				t.Fatalf("seller campaign: %v", err)
			}
			if n := len(w.got()); n != 0 {
				t.Fatalf("seller path wrote %d audit events, want 0", n)
			}
		})
	}
}

func TestCreateCampaignInvalidNotAudited(t *testing.T) {
	w := &recordingAuditWriter{}
	svc, _, _ := newAuditedFlashSvc(t, w, nil, &callLog{})
	if _, err := svc.CreateCampaign(t.Context(), adminCampaign("  ")); !errors.Is(err, ErrInvalidCampaign) {
		t.Fatalf("err = %v, want ErrInvalidCampaign", err)
	}
	if n := len(w.got()); n != 0 {
		t.Fatalf("invalid campaign wrote %d events", n)
	}
}

func TestNewCampaignIDIsUnique32Hex(t *testing.T) {
	svc, _, _ := newAuditedFlashSvc(t, &recordingAuditWriter{}, nil, &callLog{})
	a, err := svc.CreateCampaign(t.Context(), adminCampaign("list-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateCampaign(t.Context(), adminCampaign("list-b"))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.ID) != 32 || len(b.ID) != 32 || a.ID == b.ID {
		t.Fatalf("ids = %q, %q; want two distinct 32-char ids", a.ID, b.ID)
	}
	for _, r := range a.ID + b.ID {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("non-hex char %q in id", r)
		}
	}
}

func TestFlagErrorFailsOpen(t *testing.T) {
	ev := newFlags(t, erroringProvider{})
	if !ev.BooleanEnabled(t.Context(), FlagFlashSaleEnabled, true) {
		t.Fatal("a provider error must return the default true")
	}
	if ev.BooleanEnabled(t.Context(), FlagFlashSaleEnabled, false) {
		t.Fatal("a provider error must return the caller default false")
	}
}

func TestFlagNameIsFlashSaleEnabled(t *testing.T) {
	if FlagFlashSaleEnabled != "flash-sale-enabled" {
		t.Fatalf("FlagFlashSaleEnabled = %q, want flash-sale-enabled", FlagFlashSaleEnabled)
	}
}
