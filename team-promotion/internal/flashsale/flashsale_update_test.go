package flashsale

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

type updateRepo struct {
	*storefake.InMemoryFlashSaleRepository
	log       *callLog
	mu        sync.Mutex
	over      map[string]store.FlashSaleCampaign
	cancelled map[string]bool
	startsOn  func(id string)
	storeErr  error
}

func (r *updateRepo) GetByID(ctx context.Context, id string) (store.FlashSaleCampaign, error) {
	r.mu.Lock()
	if c, ok := r.over[id]; ok {
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()
	return r.InMemoryFlashSaleRepository.GetByID(ctx, id)
}

func (r *updateRepo) UpdateIfScheduled(ctx context.Context, id string, c store.FlashSaleCampaign, now time.Time) (store.FlashSaleCampaign, error) {
	r.log.add("store")
	if r.startsOn != nil {
		r.startsOn(id)
	}
	if r.storeErr != nil {
		return store.FlashSaleCampaign{}, r.storeErr
	}
	cur, err := r.GetByID(ctx, id)
	if err != nil {
		return store.FlashSaleCampaign{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancelled[id] || cur.StartsAt.IsZero() || !cur.StartsAt.After(now) {
		return store.FlashSaleCampaign{}, store.ErrCampaignNotScheduled
	}
	cur.SalePrice, cur.StockCap, cur.StartsAt, cur.EndsAt = c.SalePrice, c.StockCap, c.StartsAt, c.EndsAt
	r.over[id] = cur
	return cur, nil
}

type updListings struct {
	price int64
	err   error
	calls int
}

func (f *updListings) GetListing(_ context.Context, _ *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Price: f.price, SellerId: "seller-1"}}, nil
}

type updateEnv struct {
	svc      *Service
	repo     *updateRepo
	audit    *recordingAuditWriter
	log      *callLog
	listings *updListings
}

func newUpdateEnv(t *testing.T) updateEnv {
	t.Helper()
	log := &callLog{}
	repo := &updateRepo{
		InMemoryFlashSaleRepository: storefake.NewInMemoryFlashSaleRepository(),
		log:                         log,
		over:                        map[string]store.FlashSaleCampaign{},
		cancelled:                   map[string]bool{},
	}
	aw := &recordingAuditWriter{log: log}
	ls := &updListings{price: 100000}
	events := &eventLog{}
	svc := New(repo, nil, newFlags(t, flagProvider(true)), ls, auditx.NewGate(aw, nil), events.build, nil)
	svc.nowFn = (&fakeClock{t: svcT0}).now
	return updateEnv{svc: svc, repo: repo, audit: aw, log: log, listings: ls}
}

func (e updateEnv) seed(t *testing.T, startsAt, endsAt time.Time) store.FlashSaleCampaign {
	t.Helper()
	c, err := e.repo.Create(t.Context(), store.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 80000, StockCap: 10, StartsAt: startsAt, EndsAt: endsAt,
	}, noEvent)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return c
}

func (e updateEnv) seedScheduled(t *testing.T) store.FlashSaleCampaign {
	t.Helper()
	return e.seed(t, svcT0.Add(time.Hour), svcT0.Add(2*time.Hour))
}

func (e updateEnv) params(c store.FlashSaleCampaign) UpdateCampaignParams {
	return UpdateCampaignParams{
		CampaignID: c.ID,
		SalePrice:  90000,
		StockCap:   50,
		StartsAt:   svcT0.Add(3 * time.Hour),
		EndsAt:     svcT0.Add(5 * time.Hour),
		ActorID:    "admin-1",
	}
}

func (e updateEnv) assertUnchanged(t *testing.T, want store.FlashSaleCampaign) {
	t.Helper()
	got, err := e.repo.GetByID(t.Context(), want.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != want {
		t.Fatalf("stored campaign = %+v, want unchanged %+v", got, want)
	}
}

func (e updateEnv) assertNoAudit(t *testing.T) {
	t.Helper()
	if n := len(e.audit.got()); n != 0 {
		t.Fatalf("audit events = %d, want 0", n)
	}
}

func TestUpdateCampaignEditScheduled(t *testing.T) {
	e := newUpdateEnv(t)
	c := e.seedScheduled(t)
	p := e.params(c)

	got, err := e.svc.UpdateCampaign(t.Context(), p)
	if err != nil {
		t.Fatalf("UpdateCampaign: %v", err)
	}
	if got.ID != c.ID || got.SalePrice != 90000 || got.StockCap != 50 || !got.StartsAt.Equal(p.StartsAt) || !got.EndsAt.Equal(p.EndsAt) {
		t.Fatalf("returned = %+v, want new values on %s", got, c.ID)
	}
	if got.ListingID != c.ListingID {
		t.Fatalf("listing_id = %q, want %q", got.ListingID, c.ListingID)
	}
	stored, err := e.repo.GetByID(t.Context(), c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.SalePrice != 90000 || stored.StockCap != 50 || !stored.StartsAt.Equal(p.StartsAt) || !stored.EndsAt.Equal(p.EndsAt) {
		t.Fatalf("stored = %+v, want new values", stored)
	}
	if campaignStatus(stored, svcT0) != store.CampaignStatusScheduled {
		t.Fatalf("status = %q, want scheduled", campaignStatus(stored, svcT0))
	}
	evs := e.audit.got()
	if len(evs) != 1 {
		t.Fatalf("audit events = %d, want 1", len(evs))
	}
	if ev := evs[0]; ev.Action != "campaign.update" || ev.TargetType != "flash_sale" || ev.TargetID != c.ID || ev.ActorID != "admin-1" {
		t.Fatalf("audit event = %+v, want campaign.update flash_sale/%s by admin-1", ev, c.ID)
	}
}

func TestUpdateCampaignRefusedWhenStarted(t *testing.T) {
	cases := map[string]func(e updateEnv, t *testing.T) store.FlashSaleCampaign{
		"active": func(e updateEnv, t *testing.T) store.FlashSaleCampaign {
			return e.seed(t, svcT0.Add(-time.Hour), svcT0.Add(time.Hour))
		},
		"ended": func(e updateEnv, t *testing.T) store.FlashSaleCampaign {
			return e.seed(t, svcT0.Add(-2*time.Hour), svcT0.Add(-time.Hour))
		},
		"starts exactly now": func(e updateEnv, t *testing.T) store.FlashSaleCampaign {
			return e.seed(t, svcT0, svcT0.Add(time.Hour))
		},
		"cancelled": func(e updateEnv, t *testing.T) store.FlashSaleCampaign {
			c := e.seedScheduled(t)
			e.repo.cancelled[c.ID] = true
			return c
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			e := newUpdateEnv(t)
			c := seed(e, t)

			_, err := e.svc.UpdateCampaign(t.Context(), e.params(c))
			if !errors.Is(err, store.ErrCampaignNotScheduled) {
				t.Fatalf("err = %v, want store.ErrCampaignNotScheduled", err)
			}
			e.assertUnchanged(t, c)
		})
	}
}

func TestUpdateCampaignValidation(t *testing.T) {
	cases := []struct {
		name  string
		field string
		edit  func(p *UpdateCampaignParams)
	}{
		{"price equal to listing price", "sale_price", func(p *UpdateCampaignParams) { p.SalePrice = 100000 }},
		{"price above listing price", "sale_price", func(p *UpdateCampaignParams) { p.SalePrice = 100001 }},
		{"price zero", "sale_price", func(p *UpdateCampaignParams) { p.SalePrice = 0 }},
		{"price negative", "sale_price", func(p *UpdateCampaignParams) { p.SalePrice = -1 }},
		{"stock cap zero", "stock_cap", func(p *UpdateCampaignParams) { p.StockCap = 0 }},
		{"stock cap negative", "stock_cap", func(p *UpdateCampaignParams) { p.StockCap = -5 }},
		{"ends equal starts", "ends_at", func(p *UpdateCampaignParams) { p.EndsAt = p.StartsAt }},
		{"ends before starts", "ends_at", func(p *UpdateCampaignParams) { p.EndsAt = p.StartsAt.Add(-time.Minute) }},
		{"ends in the past", "ends_at", func(p *UpdateCampaignParams) {
			p.StartsAt = svcT0.Add(-2 * time.Hour)
			p.EndsAt = svcT0.Add(-time.Hour)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newUpdateEnv(t)
			c := e.seedScheduled(t)
			p := e.params(c)
			tc.edit(&p)

			_, err := e.svc.UpdateCampaign(t.Context(), p)
			if !errors.Is(err, ErrInvalidCampaign) {
				t.Fatalf("err = %v, want ErrInvalidCampaign", err)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("err = %q, want it to name %q", err.Error(), tc.field)
			}
			e.assertUnchanged(t, c)
			e.assertNoAudit(t)
		})
	}
}

func TestUpdateCampaignValidationBoundaryPriceJustBelowListing(t *testing.T) {
	e := newUpdateEnv(t)
	c := e.seedScheduled(t)
	p := e.params(c)
	p.SalePrice = 99999

	got, err := e.svc.UpdateCampaign(t.Context(), p)
	if err != nil {
		t.Fatalf("UpdateCampaign: %v", err)
	}
	if got.SalePrice != 99999 {
		t.Fatalf("sale_price = %d, want 99999", got.SalePrice)
	}
}

func TestUpdateCampaignNotFound(t *testing.T) {
	e := newUpdateEnv(t)

	_, err := e.svc.UpdateCampaign(t.Context(), UpdateCampaignParams{
		CampaignID: "missing", SalePrice: 90000, StockCap: 5,
		StartsAt: svcT0.Add(time.Hour), EndsAt: svcT0.Add(2 * time.Hour), ActorID: "admin-1",
	})
	if !errors.Is(err, store.ErrCampaignNotFound) {
		t.Fatalf("err = %v, want store.ErrCampaignNotFound", err)
	}
	e.assertNoAudit(t)
}

func TestUpdateCampaignAuditBeforeWriteFailsClosed(t *testing.T) {
	t.Run("audit up orders audit then store", func(t *testing.T) {
		e := newUpdateEnv(t)
		c := e.seedScheduled(t)

		if _, err := e.svc.UpdateCampaign(t.Context(), e.params(c)); err != nil {
			t.Fatalf("UpdateCampaign: %v", err)
		}
		calls := e.log.get()
		if len(calls) != 2 || calls[0] != "audit" || calls[1] != "store" {
			t.Fatalf("call order = %v, want [audit store]", calls)
		}
	})
	for name, auditErr := range auditFailures {
		t.Run("audit down "+name, func(t *testing.T) {
			e := newUpdateEnv(t)
			c := e.seedScheduled(t)
			e.audit.err = auditErr

			_, err := e.svc.UpdateCampaign(t.Context(), e.params(c))
			assertAuditUnavailable(t, err)
			if calls := e.log.get(); len(calls) != 1 || calls[0] != "audit" {
				t.Fatalf("calls = %v, want only [audit]", calls)
			}
			e.assertUnchanged(t, c)
		})
	}
}

func TestUpdateCampaignListingDown(t *testing.T) {
	failures := map[string]error{
		"unavailable": status.Error(codes.Unavailable, "team-listing down"),
		"timeout":     context.DeadlineExceeded,
		"internal":    status.Error(codes.Internal, "boom"),
	}
	for name, lerr := range failures {
		t.Run(name, func(t *testing.T) {
			e := newUpdateEnv(t)
			c := e.seedScheduled(t)
			e.listings.err = lerr

			_, err := e.svc.UpdateCampaign(t.Context(), e.params(c))
			if !errors.Is(err, listing.ErrLookupFailed) {
				t.Fatalf("err = %v, want listing.ErrLookupFailed", err)
			}
			e.assertUnchanged(t, c)
			e.assertNoAudit(t)
			if calls := e.log.get(); len(calls) != 0 {
				t.Fatalf("calls = %v, want none", calls)
			}
		})
	}
}

func TestUpdateCampaignUnknownListing(t *testing.T) {
	e := newUpdateEnv(t)
	c := e.seedScheduled(t)
	e.listings.err = status.Error(codes.NotFound, "no listing")

	_, err := e.svc.UpdateCampaign(t.Context(), e.params(c))
	if !errors.Is(err, ErrInvalidCampaign) || !strings.Contains(err.Error(), "listing_id") {
		t.Fatalf("err = %v, want ErrInvalidCampaign naming listing_id", err)
	}
	e.assertUnchanged(t, c)
	e.assertNoAudit(t)
}

func TestUpdateCampaignStartsBetweenCheckAndWrite(t *testing.T) {
	e := newUpdateEnv(t)
	c := e.seedScheduled(t)
	e.repo.startsOn = func(id string) {
		e.repo.mu.Lock()
		defer e.repo.mu.Unlock()
		e.repo.cancelled[id] = true
	}

	_, err := e.svc.UpdateCampaign(t.Context(), e.params(c))
	if !errors.Is(err, store.ErrCampaignNotScheduled) {
		t.Fatalf("err = %v, want store.ErrCampaignNotScheduled", err)
	}
	e.assertUnchanged(t, c)
}
