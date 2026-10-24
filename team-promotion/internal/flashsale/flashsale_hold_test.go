package flashsale

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/open-feature/go-sdk/openfeature"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

var svcT0 = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type holdEnv struct {
	svc       *Service
	campaigns *storefake.InMemoryFlashSaleRepository
	holds     *storefake.InMemoryFlashHoldRepository
	clk       *fakeClock
}

func newHoldEnv(t *testing.T, killSwitchOn bool) holdEnv {
	t.Helper()
	return newHoldEnvWith(t, flagProvider(killSwitchOn), nil)
}

func newHoldEnvWith(t *testing.T, p openfeature.FeatureProvider, wrap func(*storefake.InMemoryFlashHoldRepository) store.FlashHoldRepository) holdEnv {
	t.Helper()
	campaigns := storefake.NewInMemoryFlashSaleRepository()
	holds := storefake.NewInMemoryFlashHoldRepository(campaigns)
	var used store.FlashHoldRepository = holds
	if wrap != nil {
		used = wrap(holds)
	}
	clk := &fakeClock{t: svcT0}
	svc := New(campaigns, used, newFlags(t, p), nil, auditx.NewGate(&recordingAuditWriter{}, nil), (&eventLog{}).build, nil)
	svc.nowFn = clk.now
	return holdEnv{svc: svc, campaigns: campaigns, holds: holds, clk: clk}
}

func (e holdEnv) campaign(t *testing.T, stockCap, stockSold int64) store.FlashSaleCampaign {
	t.Helper()
	return e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 90000, StockCap: stockCap, StockSold: stockSold,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(24 * time.Hour),
	})
}

func (e holdEnv) campaignWith(t *testing.T, c store.FlashSaleCampaign) store.FlashSaleCampaign {
	t.Helper()
	c, err := e.campaigns.Create(t.Context(), c, noEvent)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	return c
}

func (e holdEnv) reserve(t *testing.T, id string, c store.FlashSaleCampaign, qty int64) (bool, string) {
	t.Helper()
	ok, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), id, c.ID, c.ListingID, c.VariantID, qty, c.SalePrice)
	if err != nil {
		t.Fatalf("ReserveFlashSaleStock(%s): %v", id, err)
	}
	return ok, reason
}

func (e holdEnv) mustReserve(t *testing.T, id string, c store.FlashSaleCampaign, qty int64) {
	t.Helper()
	if ok, reason := e.reserve(t, id, c, qty); !ok {
		t.Fatalf("reserve %s qty %d: refused with reason %q", id, qty, reason)
	}
}

func (e holdEnv) commit(t *testing.T, id string) bool {
	t.Helper()
	ok, err := e.svc.CommitFlashSaleStock(t.Context(), id)
	if err != nil {
		t.Fatalf("CommitFlashSaleStock(%s): %v", id, err)
	}
	return ok
}

func (e holdEnv) release(t *testing.T, id string) bool {
	t.Helper()
	ok, err := e.svc.ReleaseFlashSaleStock(t.Context(), id)
	if err != nil {
		t.Fatalf("ReleaseFlashSaleStock(%s): %v", id, err)
	}
	return ok
}

func (e holdEnv) remaining(t *testing.T, c store.FlashSaleCampaign) int64 {
	t.Helper()
	r, _, err := e.svc.GetFlashSaleStock(t.Context(), c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	return r
}

func (e holdEnv) sold(t *testing.T, c store.FlashSaleCampaign) int64 {
	t.Helper()
	got, err := e.campaigns.GetByID(t.Context(), c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return got.StockSold
}

func (e holdEnv) hasHold(t *testing.T, id string) bool {
	t.Helper()
	_, err := e.holds.Get(t.Context(), id)
	if err == nil {
		return true
	}
	if errors.Is(err, store.ErrFlashHoldNotFound) {
		return false
	}
	t.Fatalf("holds.Get: %v", err)
	return false
}

func TestReserveFlashSaleStockHoldsUnits(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	ok, reason := e.reserve(t, "r1", c, 2)
	if !ok || reason != "" {
		t.Fatalf("reserved=%v reason=%q, want true and empty", ok, reason)
	}
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3", got)
	}
}

func TestReserveFlashSaleStockIdempotent(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.mustReserve(t, "r1", c, 2)
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3 (dropped once)", got)
	}
	if h, err := e.holds.Get(t.Context(), "r1"); err != nil || h.Quantity != 2 {
		t.Fatalf("hold = %+v err=%v, want one hold of 2", h, err)
	}
}

func TestReserveFlashSaleStockReplayIgnoresChangedFields(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	ok, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), "r1", c.ID, c.ListingID, c.VariantID, 4, c.SalePrice-777)
	if err != nil || !ok || reason != "" {
		t.Fatalf("replay: ok=%v reason=%q err=%v, want stored outcome reserved=true", ok, reason, err)
	}
	if h, _ := e.holds.Get(t.Context(), "r1"); h.Quantity != 2 {
		t.Fatalf("hold quantity = %d, want 2", h.Quantity)
	}
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3", got)
	}
}

func TestReserveFlashSaleStockRefusesInactiveWindow(t *testing.T) {
	e := newHoldEnv(t, true)
	ended := e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-2 * time.Hour), EndsAt: svcT0.Add(-time.Hour),
	})
	future := e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l2", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(time.Hour), EndsAt: svcT0.Add(2 * time.Hour),
	})
	for name, c := range map[string]store.FlashSaleCampaign{"ended": ended, "not started": future} {
		id := "r-" + name
		ok, reason := e.reserve(t, id, c, 1)
		if ok || reason != "not_active" {
			t.Errorf("%s: reserved=%v reason=%q, want false/not_active", name, ok, reason)
		}
		if e.hasHold(t, id) {
			t.Errorf("%s: a hold was created", name)
		}
	}
}

func TestReserveFlashSaleStockRefusesWhenKillSwitchOff(t *testing.T) {
	e := newHoldEnv(t, false)
	c := e.campaign(t, 5, 0)
	ok, reason := e.reserve(t, "r1", c, 1)
	if ok || reason != "not_active" {
		t.Fatalf("reserved=%v reason=%q, want false/not_active", ok, reason)
	}
	if e.hasHold(t, "r1") {
		t.Fatal("a hold was created")
	}
}

func TestReserveFlashSaleStockRefusesPriceMismatch(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	ok, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), "r1", c.ID, c.ListingID, c.VariantID, 1, c.SalePrice-1)
	if err != nil || ok || reason != "price_changed" {
		t.Fatalf("ok=%v reason=%q err=%v, want false/price_changed", ok, reason, err)
	}
	if e.hasHold(t, "r1") {
		t.Fatal("a hold was created")
	}
}

func TestReserveFlashSaleStockRefusesInsufficientStock(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 4)
	ok, reason := e.reserve(t, "r2", c, 2)
	if ok || reason != "insufficient_stock" {
		t.Fatalf("reserved=%v reason=%q, want false/insufficient_stock", ok, reason)
	}
	if held, _ := e.holds.ActiveHeld(t.Context(), c.ID, svcT0); held != 4 {
		t.Fatalf("held total = %d, want 4", held)
	}
}

func TestReserveFlashSaleStockBoundaryExactRemaining(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 2)
	e.mustReserve(t, "r1", c, 1)
	if ok, reason := e.reserve(t, "r2", c, 2); !ok || reason != "" {
		t.Fatalf("reserved=%v reason=%q, want true", ok, reason)
	}
	if got := e.remaining(t, c); got != 0 {
		t.Fatalf("remaining = %d, want 0", got)
	}
}

func TestReserveFlashSaleStockReasonOrder(t *testing.T) {
	e := newHoldEnv(t, true)
	ended := e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 100, StockCap: 1,
		StartsAt: svcT0.Add(-2 * time.Hour), EndsAt: svcT0.Add(-time.Hour),
	})
	_, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), "r1", ended.ID, "l1", "", 9, 55)
	if err != nil || reason != "not_active" {
		t.Fatalf("ended campaign: reason=%q err=%v, want not_active", reason, err)
	}
	active := e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l2", SalePrice: 100, StockCap: 1,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(time.Hour),
	})
	_, reason, err = e.svc.ReserveFlashSaleStock(t.Context(), "r2", active.ID, "l2", "", 9, 55)
	if err != nil || reason != "price_changed" {
		t.Fatalf("active campaign: reason=%q err=%v, want price_changed", reason, err)
	}
}

func TestReserveFlashSaleStockVariantScope(t *testing.T) {
	e := newHoldEnv(t, true)
	vc := e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l1", VariantID: "v1", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(time.Hour),
	})
	anyVariant := e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l2", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(time.Hour),
	})
	ctx := t.Context()
	reserve := func(id, campaign, listing, variant string) (bool, string) {
		ok, reason, err := e.svc.ReserveFlashSaleStock(ctx, id, campaign, listing, variant, 1, 100)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		return ok, reason
	}
	if ok, reason := reserve("a", vc.ID, "l1", "v2"); ok || reason != "not_active" {
		t.Errorf("other variant: ok=%v reason=%q, want false/not_active", ok, reason)
	}
	if ok, reason := reserve("b", vc.ID, "l1", "v1"); !ok || reason != "" {
		t.Errorf("same variant: ok=%v reason=%q, want true", ok, reason)
	}
	if ok, reason := reserve("c", anyVariant.ID, "l2", "v2"); !ok || reason != "" {
		t.Errorf("campaign without variant: ok=%v reason=%q, want true", ok, reason)
	}
	if ok, reason := reserve("d", vc.ID, "other-listing", "v1"); ok || reason != "not_active" {
		t.Errorf("foreign listing: ok=%v reason=%q, want false/not_active", ok, reason)
	}
}

func TestReserveFlashSaleStockUnknownCampaign(t *testing.T) {
	e := newHoldEnv(t, true)
	ok, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), "r1", "no-such-campaign", "l1", "", 1, 100)
	if err != nil || ok || reason != "not_active" {
		t.Fatalf("ok=%v reason=%q err=%v, want false/not_active/nil", ok, reason, err)
	}
	if e.hasHold(t, "r1") {
		t.Fatal("a hold was created")
	}
}

func TestReserveFlashSaleStockReplayAfterRelease(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	if !e.release(t, "r1") {
		t.Fatal("release returned false")
	}
	if ok, reason := e.reserve(t, "r1", c, 2); ok || reason != "not_active" {
		t.Fatalf("replay: reserved=%v reason=%q, want false/not_active", ok, reason)
	}
	if got := e.remaining(t, c); got != 5 {
		t.Fatalf("remaining = %d, want 5 (held total 0)", got)
	}
}

func TestReserveFlashSaleStockReplayAfterExpiry(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.clk.advance(15 * time.Minute)
	if ok, reason := e.reserve(t, "r1", c, 2); ok || reason != "not_active" {
		t.Fatalf("replay: reserved=%v reason=%q, want false/not_active", ok, reason)
	}
	if got := e.remaining(t, c); got != 5 {
		t.Fatalf("remaining = %d, want 5 (no new hold)", got)
	}
}

func TestFlashHoldExpiresAfterFifteenMinutes(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.clk.advance(14*time.Minute + 59*time.Second)
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining at 14:59 = %d, want 3", got)
	}
	e.clk.advance(time.Second)
	if got := e.remaining(t, c); got != 5 {
		t.Fatalf("remaining at 15:00 = %d, want 5", got)
	}
	if ok, reason := e.reserve(t, "r2", c, 5); !ok || reason != "" {
		t.Fatalf("reserve of 5 at 15:00: ok=%v reason=%q, want true", ok, reason)
	}
}

func TestReserveFlashSaleStockConcurrentCap(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	reserved, insufficient := 0, 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), fmt.Sprintf("r%d", i), c.ID, c.ListingID, "", 1, c.SalePrice)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				t.Errorf("reserve: %v", err)
			case ok:
				reserved++
			case reason == "insufficient_stock":
				insufficient++
			default:
				t.Errorf("unexpected reason %q", reason)
			}
		}(i)
	}
	wg.Wait()
	if reserved != 5 || insufficient != 15 {
		t.Fatalf("reserved=%d insufficient=%d, want 5/15", reserved, insufficient)
	}
	if got := e.remaining(t, c); got != 0 {
		t.Fatalf("remaining = %d, want 0", got)
	}
}

func TestReserveFlashSaleStockConcurrentSameReservation(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	reserved := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := e.svc.ReserveFlashSaleStock(t.Context(), "same", c.ID, c.ListingID, "", 2, c.SalePrice)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("reserve: %v", err)
			} else if ok {
				reserved++
			}
		}()
	}
	wg.Wait()
	if reserved != 10 {
		t.Fatalf("reserved=%d, want all 10", reserved)
	}
	if h, err := e.holds.Get(t.Context(), "same"); err != nil || h.Quantity != 2 {
		t.Fatalf("hold = %+v err=%v, want one hold of 2", h, err)
	}
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3", got)
	}
}

func TestFlashHoldConcurrentCommitAndReserve(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	for i := 0; i < 3; i++ {
		e.mustReserve(t, fmt.Sprintf("h%d", i), c, 1)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	reserved := 0
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if ok, err := e.svc.CommitFlashSaleStock(t.Context(), fmt.Sprintf("h%d", i)); err != nil || !ok {
				t.Errorf("commit h%d: ok=%v err=%v", i, ok, err)
			}
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, _, err := e.svc.ReserveFlashSaleStock(t.Context(), fmt.Sprintf("n%d", i), c.ID, c.ListingID, "", 1, c.SalePrice)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("reserve: %v", err)
			} else if ok {
				reserved++
			}
		}(i)
	}
	wg.Wait()
	if reserved != 2 {
		t.Fatalf("reserved=%d of 10, want exactly 2", reserved)
	}
	if got := e.sold(t, c); got != 3 {
		t.Fatalf("stock_sold = %d, want 3", got)
	}
	if got := e.remaining(t, c); got != 0 {
		t.Fatalf("remaining = %d, want 0 (sold + held = cap)", got)
	}
}

func TestCommitFlashSaleStockAddsToSold(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	if !e.commit(t, "r1") {
		t.Fatal("committed=false, want true")
	}
	if got := e.sold(t, c); got != 2 {
		t.Fatalf("stock_sold = %d, want 2", got)
	}
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3", got)
	}
}

func TestCommitFlashSaleStockIdempotent(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	first := e.commit(t, "r1")
	second := e.commit(t, "r1")
	if !first || !second {
		t.Fatal("both commits must report committed=true")
	}
	if got := e.sold(t, c); got != 2 {
		t.Fatalf("stock_sold = %d, want 2 (added once)", got)
	}
}

func TestCommitFlashSaleStockUnknown(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	if e.commit(t, "ghost") {
		t.Fatal("committed=true for an unknown reservation")
	}
	if got := e.sold(t, c); got != 0 {
		t.Fatalf("stock_sold = %d, want 0", got)
	}
}

func TestCommitFlashSaleStockAfterRelease(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.release(t, "r1")
	if e.commit(t, "r1") {
		t.Fatal("committed=true for a released hold")
	}
	if got := e.sold(t, c); got != 0 {
		t.Fatalf("stock_sold = %d, want 0", got)
	}
}

func TestCommitFlashSaleStockAfterExpiry(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.clk.advance(15 * time.Minute)
	if e.commit(t, "r1") {
		t.Fatal("committed=true for an expired hold")
	}
	if got := e.sold(t, c); got != 0 {
		t.Fatalf("stock_sold = %d, want 0", got)
	}
}

func TestCommitFlashSaleStockAfterWindowEnded(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(10 * time.Minute),
	})
	e.mustReserve(t, "r1", c, 2)
	e.clk.advance(12 * time.Minute) // window over, hold 12 minutes old
	if !e.commit(t, "r1") {
		t.Fatal("committed=false, want true after the window ended")
	}
	if got := e.sold(t, c); got != 2 {
		t.Fatalf("stock_sold = %d, want 2", got)
	}
}

func TestReleaseFlashSaleStockReturnsUnits(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	if !e.release(t, "r1") {
		t.Fatal("released=false, want true")
	}
	if got := e.remaining(t, c); got != 5 {
		t.Fatalf("remaining = %d, want 5", got)
	}
}

func TestReleaseFlashSaleStockAfterCommit(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.commit(t, "r1")
	if !e.release(t, "r1") {
		t.Fatal("released=false, want true")
	}
	if got := e.sold(t, c); got != 0 {
		t.Fatalf("stock_sold = %d, want 0", got)
	}
	if got := e.remaining(t, c); got != 5 {
		t.Fatalf("remaining = %d, want 5", got)
	}
}

func TestReleaseFlashSaleStockIdempotent(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.commit(t, "r1")
	for i := 0; i < 2; i++ {
		if !e.release(t, "r1") {
			t.Fatalf("release #%d: released=false, want true", i+1)
		}
		if got := e.sold(t, c); got != 0 {
			t.Fatalf("stock_sold after release #%d = %d, want 0", i+1, got)
		}
	}
}

func TestReleaseFlashSaleStockUnknownNoOp(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	released, err := e.svc.ReleaseFlashSaleStock(t.Context(), "ghost")
	if err != nil || released {
		t.Fatalf("released=%v err=%v, want false,nil", released, err)
	}
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3 (unchanged)", got)
	}
}

func TestReleaseFlashSaleStockExpiredNoCounterChange(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.clk.advance(15 * time.Minute)
	if !e.release(t, "r1") {
		t.Fatal("released=false, want true")
	}
	if got := e.sold(t, c); got != 0 {
		t.Fatalf("stock_sold = %d, want 0", got)
	}
	if got := e.remaining(t, c); got != 5 {
		t.Fatalf("remaining = %d, want 5", got)
	}
}

func TestReleaseOldCommittedHoldAfterPurge(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaignWith(t, store.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(24 * time.Hour * 365),
	})
	e.mustReserve(t, "r1", c, 2)
	e.commit(t, "r1")
	e.clk.advance(90 * 24 * time.Hour)
	if _, err := e.holds.PurgeReleasedAndExpiredHolds(t.Context(), e.clk.now().Add(-30*24*time.Hour)); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if !e.hasHold(t, "r1") {
		t.Fatal("committed hold was purged")
	}
	if !e.release(t, "r1") {
		t.Fatal("released=false, want true")
	}
	if got := e.sold(t, c); got != 0 {
		t.Fatalf("stock_sold = %d, want 0", got)
	}
}

func TestGetFlashSaleStockCountsHolds(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 10, 3)
	e.mustReserve(t, "expired", c, 1)
	e.mustReserve(t, "released", c, 4)
	e.release(t, "released")
	e.clk.advance(15 * time.Minute) // "expired" is now 15 minutes old
	e.mustReserve(t, "active", c, 2)
	remaining, stockCap, err := e.svc.GetFlashSaleStock(t.Context(), c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	if remaining != 5 || stockCap != 10 {
		t.Fatalf("remaining=%d cap=%d, want 5/10", remaining, stockCap)
	}
}

type fixedHeld struct {
	*storefake.InMemoryFlashHoldRepository
	n int64
}

func (f fixedHeld) ActiveHeld(context.Context, string, time.Time) (int64, error) { return f.n, nil }

func TestGetFlashSaleStockRemainingFlooredWithHold(t *testing.T) {
	e := newHoldEnvWith(t, flagProvider(true), func(h *storefake.InMemoryFlashHoldRepository) store.FlashHoldRepository {
		return fixedHeld{InMemoryFlashHoldRepository: h, n: 2}
	})
	c := e.campaign(t, 10, 25)
	remaining, stockCap, err := e.svc.GetFlashSaleStock(t.Context(), c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	if remaining != 0 || stockCap != 10 {
		t.Fatalf("remaining=%d cap=%d, want 0/10", remaining, stockCap)
	}
}

func TestKillSwitchOffViaProvider(t *testing.T) {
	e := newHoldEnvWith(t, flagProvider(false), nil)
	c := e.campaign(t, 5, 0)
	ok, reason := e.reserve(t, "r1", c, 1)
	if ok || reason != "not_active" {
		t.Fatalf("reserved=%v reason=%q, want false/not_active", ok, reason)
	}
	if e.hasHold(t, "r1") {
		t.Fatal("a hold was created")
	}
	if got := e.remaining(t, c); got != 5 {
		t.Fatalf("remaining = %d, want 5", got)
	}
}

func TestKillSwitchOnViaProviderReserves(t *testing.T) {
	e := newHoldEnvWith(t, flagProvider(true), nil)
	c := e.campaign(t, 5, 0)
	if ok, reason := e.reserve(t, "r1", c, 1); !ok || reason != "" {
		t.Fatalf("reserved=%v reason=%q, want true", ok, reason)
	}
}

func TestFlagErrorFailsOpenReserves(t *testing.T) {
	e := newHoldEnvWith(t, erroringProvider{}, nil)
	c := e.campaign(t, 5, 0)
	if ok, reason := e.reserve(t, "r1", c, 1); !ok || reason != "" {
		t.Fatalf("reserved=%v reason=%q, want true (a flag provider error keeps the default on)", ok, reason)
	}
	if !e.hasHold(t, "r1") {
		t.Fatal("no hold was created")
	}
}
