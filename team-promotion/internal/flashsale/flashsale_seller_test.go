package flashsale

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

type sellerRepo struct {
	*storefake.InMemoryFlashSaleRepository
	log *callLog

	mu        sync.Mutex
	order     []string
	cancelled map[string]time.Time
}

func (r *sellerRepo) Create(ctx context.Context, c store.FlashSaleCampaign, event func(store.FlashSaleCampaign) (outbox.Row, error)) (store.FlashSaleCampaign, error) {
	out, err := r.InMemoryFlashSaleRepository.Create(ctx, c, event)
	if err == nil {
		r.mu.Lock()
		r.order = append(r.order, out.ID)
		r.mu.Unlock()
	}
	return out, err
}

func (r *sellerRepo) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}

func (r *sellerRepo) GetByID(ctx context.Context, id string) (store.FlashSaleCampaign, error) {
	c, err := r.InMemoryFlashSaleRepository.GetByID(ctx, id)
	if err != nil {
		return c, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c.CancelledAt = r.cancelled[id]
	return c, nil
}

func (r *sellerRepo) Cancel(ctx context.Context, id string, now time.Time) (time.Time, error) {
	r.log.add("admin-store")
	if _, err := r.GetByID(ctx, id); err != nil {
		return time.Time{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if at, ok := r.cancelled[id]; ok {
		return at, nil
	}
	r.cancelled[id] = now
	return now, nil
}

func (r *sellerRepo) ListMyCampaigns(ctx context.Context, sellerID string, statuses []string, now time.Time, offset, limit int) ([]store.FlashSaleCampaign, error) {
	r.log.add("list-mine")
	var out []store.FlashSaleCampaign
	for _, id := range r.ids() {
		c, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if c.SellerID != sellerID {
			continue
		}
		if len(statuses) > 0 && !contains(statuses, campaignStatus(c, now)) {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].StartsAt.Equal(out[j].StartsAt) {
			return out[i].StartsAt.After(out[j].StartsAt)
		}
		return out[i].ID < out[j].ID
	})
	if offset > len(out) {
		offset = len(out)
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *sellerRepo) CancelIfScheduledOwned(ctx context.Context, id, sellerID string, now time.Time) (time.Time, error) {
	r.log.add("owned-store")
	c, err := r.GetByID(ctx, id)
	if err != nil || sellerID == "" || c.SellerID != sellerID {
		return time.Time{}, store.ErrNotOwner
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if at, ok := r.cancelled[id]; ok {
		return at, nil
	}
	if c.StartsAt.IsZero() || !c.StartsAt.After(now) {
		return time.Time{}, store.ErrCampaignNotScheduled
	}
	r.cancelled[id] = now
	return now, nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

type sellerEnv struct {
	svc    *Service
	repo   *sellerRepo
	audit  *recordingAuditWriter
	log    *callLog
	clk    *fakeClock
	listed *fakeListings
}

func newSellerEnv(t *testing.T) sellerEnv {
	t.Helper()
	log := &callLog{}
	repo := &sellerRepo{InMemoryFlashSaleRepository: storefake.NewInMemoryFlashSaleRepository(), log: log, cancelled: map[string]time.Time{}}
	aw := &recordingAuditWriter{log: log}
	ls := &fakeListings{owners: map[string]string{"l-a": "seller-A", "l-b": "seller-B"}, price: 100000}
	clk := &fakeClock{t: svcT0}
	svc := New(repo, nil, newFlags(t, flagProvider(true)), ls, auditx.NewGate(aw, nil), (&eventLog{}).build, nil)
	svc.nowFn = clk.now
	return sellerEnv{svc: svc, repo: repo, audit: aw, log: log, clk: clk, listed: ls}
}

func (e sellerEnv) seed(t *testing.T, id, seller string, startsIn, endsIn time.Duration) store.FlashSaleCampaign {
	t.Helper()
	c, err := e.repo.Create(t.Context(), store.FlashSaleCampaign{
		ID: id, ListingID: "l-" + id, SalePrice: 90000, StockCap: 10, StockSold: 3,
		StartsAt: svcT0.Add(startsIn), EndsAt: svcT0.Add(endsIn), SellerID: seller,
	}, noEvent)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return c
}

func viewIDs(vs []CampaignView) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.ID
	}
	return out
}

func TestCampaignSellerCreateStoresListingOwner_NewCampaignRecordsListingOwner(t *testing.T) {
	t.Run("seller path stores the caller as owner", func(t *testing.T) {
		e := newCreateEnv(t)
		p := sellerParams()
		c, err := e.svc.CreateCampaign(t.Context(), p)
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		got, err := e.repo.GetByID(t.Context(), c.ID)
		if err != nil || got.SellerID != "seller-1" {
			t.Fatalf("stored SellerID = %q err=%v, want seller-1", got.SellerID, err)
		}
	})
	t.Run("admin without key stores the listing owner", func(t *testing.T) {
		e := newCreateEnv(t)
		p := adminParams()
		p.ListingID, p.IdempotencyKey = "theirs", ""
		c, err := e.svc.CreateCampaign(t.Context(), p)
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		got, err := e.repo.GetByID(t.Context(), c.ID)
		if err != nil || got.SellerID != "seller-2" {
			t.Fatalf("stored SellerID = %q err=%v, want seller-2 (the listing owner, not the admin)", got.SellerID, err)
		}
	})
	t.Run("admin with key stores the listing owner", func(t *testing.T) {
		e := newCreateEnv(t)
		p := adminParams()
		p.ListingID = "theirs"
		c, err := e.svc.CreateCampaign(t.Context(), p)
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		got, err := e.repo.GetByID(t.Context(), c.ID)
		if err != nil || got.SellerID != "seller-2" {
			t.Fatalf("stored SellerID = %q err=%v, want seller-2", got.SellerID, err)
		}
	})
	for name, ls := range map[string]*fakeListings{
		"admin without key, listing lookup fails":   {err: status.Error(codes.Unavailable, "team-domain down")},
		"admin without key, listing does not exist": {owners: map[string]string{}},
	} {
		t.Run(name+" leaves owner empty and still creates", func(t *testing.T) {
			e := newCreateEnv(t)
			e.listings.err, e.listings.owners = ls.err, ls.owners
			p := adminParams()
			p.IdempotencyKey = ""
			c, err := e.svc.CreateCampaign(t.Context(), p)
			if err != nil {
				t.Fatalf("admin create must not fail on a best-effort lookup: %v", err)
			}
			got, err := e.repo.GetByID(t.Context(), c.ID)
			if err != nil || got.SellerID != "" {
				t.Fatalf("stored SellerID = %q err=%v, want empty for the backfill", got.SellerID, err)
			}
			if n := len(e.audit.got()); n != 1 {
				t.Fatalf("audit events = %d, want the one admin_create event", n)
			}
		})
	}
}

func TestListMyCampaignsOwnWithStatus_SellerSeesOnlyTheirOwnCampaigns(t *testing.T) {
	e := newSellerEnv(t)
	sched := e.seed(t, "a-sched", "seller-A", time.Hour, 2*time.Hour)
	act := e.seed(t, "a-active", "seller-A", -time.Hour, time.Hour)
	e.seed(t, "b-active", "seller-B", -time.Hour, time.Hour)

	res, err := e.svc.ListMyCampaigns(t.Context(), "seller-A", nil, "", 0)
	if err != nil {
		t.Fatalf("ListMyCampaigns: %v", err)
	}
	if fmt.Sprint(viewIDs(res.Campaigns)) != fmt.Sprint([]string{sched.ID, act.ID}) {
		t.Fatalf("ids = %v, want [a-sched a-active]", viewIDs(res.Campaigns))
	}
	if res.Campaigns[0].Status != store.CampaignStatusScheduled || res.Campaigns[1].Status != store.CampaignStatusActive {
		t.Fatalf("statuses = %q %q, want scheduled active", res.Campaigns[0].Status, res.Campaigns[1].Status)
	}
	for _, v := range res.Campaigns {
		if v.SellerID != "seller-A" || v.StockCap != 10 || v.StockSold != 3 {
			t.Errorf("%s = seller %q cap %d sold %d, want seller-A 10 3", v.ID, v.SellerID, v.StockCap, v.StockSold)
		}
	}
	if len(e.audit.got()) != 0 {
		t.Fatalf("listing wrote %d audit events, want 0", len(e.audit.got()))
	}
}

func TestListMyCampaignsStatusFilter_StatusFilterNarrowsTheList(t *testing.T) {
	e := newSellerEnv(t)
	e.seed(t, "a-sched", "seller-A", time.Hour, 2*time.Hour)
	e.seed(t, "a-active", "seller-A", -time.Hour, time.Hour)
	e.seed(t, "b-sched", "seller-B", time.Hour, 2*time.Hour)

	res, err := e.svc.ListMyCampaigns(t.Context(), "seller-A", []string{store.CampaignStatusScheduled}, "", 0)
	if err != nil {
		t.Fatalf("ListMyCampaigns: %v", err)
	}
	if fmt.Sprint(viewIDs(res.Campaigns)) != "[a-sched]" {
		t.Fatalf("ids = %v, want [a-sched]", viewIDs(res.Campaigns))
	}
}

func TestListMyCampaignsPaging_PagingBoundaryAt21Campaigns(t *testing.T) {
	e := newSellerEnv(t)
	for i := 0; i < 21; i++ {
		e.seed(t, fmt.Sprintf("a-%02d", i), "seller-A", time.Duration(100-i)*time.Hour, 200*time.Hour)
	}
	e.seed(t, "b-1", "seller-B", 300*time.Hour, 400*time.Hour)

	p1, err := e.svc.ListMyCampaigns(t.Context(), "seller-A", nil, "", 20)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(p1.Campaigns) != 20 || p1.NextCursor == "" {
		t.Fatalf("page 1 = %d campaigns cursor %q, want 20 and a cursor", len(p1.Campaigns), p1.NextCursor)
	}
	p2, err := e.svc.ListMyCampaigns(t.Context(), "seller-A", nil, p1.NextCursor, 20)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(p2.Campaigns) != 1 || p2.NextCursor != "" {
		t.Fatalf("page 2 = %d campaigns cursor %q, want 1 and no cursor", len(p2.Campaigns), p2.NextCursor)
	}
	seen := map[string]bool{}
	for _, v := range append(p1.Campaigns, p2.Campaigns...) {
		if seen[v.ID] || v.SellerID != "seller-A" {
			t.Fatalf("repeat or foreign campaign %s (seller %q)", v.ID, v.SellerID)
		}
		seen[v.ID] = true
	}
	if len(seen) != 21 {
		t.Fatalf("distinct campaigns = %d, want 21", len(seen))
	}
}

func TestListMyCampaignsDefaultAndCap_PagingBoundaryAt21Campaigns(t *testing.T) {
	e := newSellerEnv(t)
	for i := 0; i < 205; i++ {
		e.seed(t, fmt.Sprintf("a-%03d", i), "seller-A", time.Duration(1000-i)*time.Hour, 2000*time.Hour)
	}
	def, err := e.svc.ListMyCampaigns(t.Context(), "seller-A", nil, "", 0)
	if err != nil || len(def.Campaigns) != 20 {
		t.Fatalf("default page = %d err=%v, want 20", len(def.Campaigns), err)
	}
	big, err := e.svc.ListMyCampaigns(t.Context(), "seller-A", nil, "", 1000)
	if err != nil || len(big.Campaigns) != 200 || big.NextCursor == "" {
		t.Fatalf("capped page = %d cursor %q err=%v, want 200 and a cursor", len(big.Campaigns), big.NextCursor, err)
	}
}

func TestCancelMyCampaign_SellerCancelsTheirScheduledCampaign(t *testing.T) {
	e := newSellerEnv(t)
	c := e.seed(t, "a-sched", "seller-A", time.Hour, 2*time.Hour)

	res, err := e.svc.CancelMyCampaign(t.Context(), c.ID, "seller-A")
	if err != nil {
		t.Fatalf("CancelMyCampaign: %v", err)
	}
	if res.Status != store.CampaignStatusCancelled || res.Campaign.ID != c.ID || !res.CancelledAt.Equal(svcT0) {
		t.Fatalf("result = status %q id %q at %v, want cancelled %s %v", res.Status, res.Campaign.ID, res.CancelledAt, c.ID, svcT0)
	}
	list, err := e.svc.ListMyCampaigns(t.Context(), "seller-A", []string{store.CampaignStatusCancelled}, "", 0)
	if err != nil || fmt.Sprint(viewIDs(list.Campaigns)) != "[a-sched]" {
		t.Fatalf("cancelled list = %v err=%v, want [a-sched]", viewIDs(list.Campaigns), err)
	}
}

func TestCancelMyCampaignRefusals_ActiveForeignAndUnknown(t *testing.T) {
	e := newSellerEnv(t)
	active := e.seed(t, "a-active", "seller-A", -time.Hour, time.Hour)
	sched := e.seed(t, "a-sched", "seller-A", time.Hour, 2*time.Hour)

	if _, err := e.svc.CancelMyCampaign(t.Context(), active.ID, "seller-A"); !errors.Is(err, store.ErrCampaignNotScheduled) {
		t.Fatalf("active: err = %v, want ErrCampaignNotScheduled", err)
	}
	if _, err := e.svc.CancelMyCampaign(t.Context(), sched.ID, "seller-B"); !errors.Is(err, store.ErrNotOwner) {
		t.Fatalf("foreign: err = %v, want ErrNotOwner", err)
	}
	if _, err := e.svc.CancelMyCampaign(t.Context(), "ghost", "seller-B"); !errors.Is(err, store.ErrNotOwner) {
		t.Fatalf("unknown: err = %v, want ErrNotOwner", err)
	}
	for _, id := range []string{active.ID, sched.ID} {
		got, err := e.repo.GetByID(t.Context(), id)
		if err != nil || !got.CancelledAt.IsZero() {
			t.Fatalf("%s changed by a refused cancel: %v err=%v", id, got.CancelledAt, err)
		}
	}
}

func TestCancelMyCampaignIdempotent_CancellingTwiceIsIdempotent(t *testing.T) {
	e := newSellerEnv(t)
	c := e.seed(t, "a-sched", "seller-A", time.Hour, 2*time.Hour)

	first, err := e.svc.CancelMyCampaign(t.Context(), c.ID, "seller-A")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	e.clk.advance(30 * time.Minute)
	second, err := e.svc.CancelMyCampaign(t.Context(), c.ID, "seller-A")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Status != store.CampaignStatusCancelled || !second.CancelledAt.Equal(first.CancelledAt) {
		t.Fatalf("second = %q at %v, want cancelled at %v", second.Status, second.CancelledAt, first.CancelledAt)
	}
}

func TestCancelMyCampaignNoAudit_SellerCancellationWritesNoAuditEvent(t *testing.T) {
	e := newSellerEnv(t)
	c := e.seed(t, "a-sched", "seller-A", time.Hour, 2*time.Hour)
	e.audit.err = status.Error(codes.Unavailable, "team-audit down")

	res, err := e.svc.CancelMyCampaign(t.Context(), c.ID, "seller-A")
	if err != nil {
		t.Fatalf("CancelMyCampaign with audit down: %v", err)
	}
	if res.Status != store.CampaignStatusCancelled {
		t.Fatalf("status = %q, want cancelled", res.Status)
	}
	if got, gerr := e.repo.GetByID(t.Context(), c.ID); gerr != nil || got.CancelledAt.IsZero() {
		t.Fatalf("campaign not cancelled: %v err=%v", got.CancelledAt, gerr)
	}
	if evs := e.audit.got(); len(evs) != 0 {
		t.Fatalf("audit events = %+v, want none", evs)
	}
	for _, call := range e.log.get() {
		if call == "audit" || call == "admin-store" {
			t.Fatalf("seller cancel went through the admin path: calls = %v", e.log.get())
		}
	}
}

func TestCampaignSellerAdminCancelRegression_AdminCancelKeepsItsAdminRules(t *testing.T) {
	e := newSellerEnv(t)
	active := e.seed(t, "a-active", "seller-A", -time.Hour, time.Hour)

	res, err := e.svc.CancelCampaign(t.Context(), active.ID, "admin-1")
	if err != nil {
		t.Fatalf("admin cancel of another seller's ACTIVE campaign: %v", err)
	}
	if res.Status != store.CampaignStatusCancelled {
		t.Fatalf("status = %q, want cancelled", res.Status)
	}
	evs := e.audit.got()
	if len(evs) != 1 || evs[0].Action != "campaign.cancel" || evs[0].ActorID != "admin-1" || evs[0].TargetID != active.ID {
		t.Fatalf("audit = %+v, want one campaign.cancel by admin-1", evs)
	}
}
