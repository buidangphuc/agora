package service

// Service tests for the flash-stock hold RPCs (openspec change
// promotion-flash-stock-hold). Interface assumed (implementer creates it):
//
//	func (s *FlashSaleService) WithHolds(h repository.FlashHoldRepository) *FlashSaleService
//	func (s *FlashSaleService) ReserveFlashSaleStock(ctx, reservationID, campaignID, listingID,
//	    variantID string, quantity, expectedSalePrice int64) (reserved bool, reason string, err error)
//	func (s *FlashSaleService) CommitFlashSaleStock(ctx, reservationID string) (bool, error)
//	func (s *FlashSaleService) ReleaseFlashSaleStock(ctx, reservationID string) (bool, error)
//
// The hold repository contract is described in internal/repository/flashsale_hold_test.go.
// The service clock is the existing nowFn field, driven by a fake clock here.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/repository"
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
	svc       *FlashSaleService
	campaigns *repository.InMemoryFlashSaleRepository
	holds     *repository.InMemoryFlashHoldRepository
	clk       *fakeClock
}

func newHoldEnv(t *testing.T, killSwitchOn bool) holdEnv {
	t.Helper()
	campaigns := repository.NewInMemoryFlashSaleRepository()
	holds := repository.NewInMemoryFlashHoldRepository(campaigns)
	clk := &fakeClock{t: svcT0}
	svc := NewFlashSaleService(campaigns, nil, stubFlags{enabled: killSwitchOn}, nil, nil).WithHolds(holds)
	svc.nowFn = clk.now
	return holdEnv{svc: svc, campaigns: campaigns, holds: holds, clk: clk}
}

// campaign creates an active campaign (window around svcT0) for listing "l1".
func (e holdEnv) campaign(t *testing.T, stockCap, stockSold int64) repository.FlashSaleCampaign {
	t.Helper()
	return e.campaignWith(t, repository.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 90000, StockCap: stockCap, StockSold: stockSold,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(24 * time.Hour),
	})
}

func (e holdEnv) campaignWith(t *testing.T, c repository.FlashSaleCampaign) repository.FlashSaleCampaign {
	t.Helper()
	c, err := e.campaigns.Create(context.Background(), c)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	return c
}

func (e holdEnv) reserve(t *testing.T, id string, c repository.FlashSaleCampaign, qty int64) (bool, string) {
	t.Helper()
	ok, reason, err := e.svc.ReserveFlashSaleStock(context.Background(), id, c.ID, c.ListingID, c.VariantID, qty, c.SalePrice)
	if err != nil {
		t.Fatalf("ReserveFlashSaleStock(%s): %v", id, err)
	}
	return ok, reason
}

func (e holdEnv) mustReserve(t *testing.T, id string, c repository.FlashSaleCampaign, qty int64) {
	t.Helper()
	if ok, reason := e.reserve(t, id, c, qty); !ok {
		t.Fatalf("reserve %s qty %d: refused with reason %q", id, qty, reason)
	}
}

func (e holdEnv) commit(t *testing.T, id string) bool {
	t.Helper()
	ok, err := e.svc.CommitFlashSaleStock(context.Background(), id)
	if err != nil {
		t.Fatalf("CommitFlashSaleStock(%s): %v", id, err)
	}
	return ok
}

func (e holdEnv) release(t *testing.T, id string) bool {
	t.Helper()
	ok, err := e.svc.ReleaseFlashSaleStock(context.Background(), id)
	if err != nil {
		t.Fatalf("ReleaseFlashSaleStock(%s): %v", id, err)
	}
	return ok
}

func (e holdEnv) remaining(t *testing.T, c repository.FlashSaleCampaign) int64 {
	t.Helper()
	r, _, err := e.svc.GetFlashSaleStock(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	return r
}

func (e holdEnv) sold(t *testing.T, c repository.FlashSaleCampaign) int64 {
	t.Helper()
	got, err := e.campaigns.GetByID(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return got.StockSold
}

func (e holdEnv) hasHold(t *testing.T, id string) bool {
	t.Helper()
	_, err := e.holds.Get(context.Background(), id)
	if err == nil {
		return true
	}
	if errors.Is(err, repository.ErrFlashHoldNotFound) {
		return false
	}
	t.Fatalf("holds.Get: %v", err)
	return false
}

// Scenario: Reserve holds the units of an active campaign.
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

// Scenario: Reserve is idempotent on the reservation id.
func TestReserveFlashSaleStockIdempotent(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	e.mustReserve(t, "r1", c, 2)
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3 (dropped once)", got)
	}
	if h, err := e.holds.Get(context.Background(), "r1"); err != nil || h.Quantity != 2 {
		t.Fatalf("hold = %+v err=%v, want one hold of 2", h, err)
	}
}

// Scenario: Reserve replay with different fields returns the stored outcome.
func TestReserveFlashSaleStockReplayIgnoresChangedFields(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	ok, reason, err := e.svc.ReserveFlashSaleStock(context.Background(), "r1", c.ID, c.ListingID, c.VariantID, 4, c.SalePrice-777)
	if err != nil || !ok || reason != "" {
		t.Fatalf("replay: ok=%v reason=%q err=%v, want stored outcome reserved=true", ok, reason, err)
	}
	if h, _ := e.holds.Get(context.Background(), "r1"); h.Quantity != 2 {
		t.Fatalf("hold quantity = %d, want 2", h.Quantity)
	}
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3", got)
	}
}

// Scenarios: Reserve refuses an ended campaign / a campaign that has not started.
func TestReserveFlashSaleStockRefusesInactiveWindow(t *testing.T) {
	e := newHoldEnv(t, true)
	ended := e.campaignWith(t, repository.FlashSaleCampaign{ListingID: "l1", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-2 * time.Hour), EndsAt: svcT0.Add(-time.Hour)})
	future := e.campaignWith(t, repository.FlashSaleCampaign{ListingID: "l2", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(time.Hour), EndsAt: svcT0.Add(2 * time.Hour)})
	for name, c := range map[string]repository.FlashSaleCampaign{"ended": ended, "not started": future} {
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

// Scenario: Reserve refuses while the kill-switch is OFF.
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

// Scenario: Reserve refuses a price that differs from the campaign.
func TestReserveFlashSaleStockRefusesPriceMismatch(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	ok, reason, err := e.svc.ReserveFlashSaleStock(context.Background(), "r1", c.ID, c.ListingID, c.VariantID, 1, c.SalePrice-1)
	if err != nil || ok || reason != "price_changed" {
		t.Fatalf("ok=%v reason=%q err=%v, want false/price_changed", ok, reason, err)
	}
	if e.hasHold(t, "r1") {
		t.Fatal("a hold was created")
	}
}

// Scenario: Reserve refuses more units than remain.
func TestReserveFlashSaleStockRefusesInsufficientStock(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 4)
	ok, reason := e.reserve(t, "r2", c, 2)
	if ok || reason != "insufficient_stock" {
		t.Fatalf("reserved=%v reason=%q, want false/insufficient_stock", ok, reason)
	}
	if held, _ := e.holds.ActiveHeld(context.Background(), c.ID, svcT0); held != 4 {
		t.Fatalf("held total = %d, want 4", held)
	}
}

// Scenario: Reserve accepts exactly the remaining units.
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

// Scenario: The first failing check decides the reason.
func TestReserveFlashSaleStockReasonOrder(t *testing.T) {
	e := newHoldEnv(t, true)
	ended := e.campaignWith(t, repository.FlashSaleCampaign{ListingID: "l1", SalePrice: 100, StockCap: 1,
		StartsAt: svcT0.Add(-2 * time.Hour), EndsAt: svcT0.Add(-time.Hour)})
	_, reason, err := e.svc.ReserveFlashSaleStock(context.Background(), "r1", ended.ID, "l1", "", 9, 55)
	if err != nil || reason != "not_active" {
		t.Fatalf("ended campaign: reason=%q err=%v, want not_active", reason, err)
	}
	active := e.campaignWith(t, repository.FlashSaleCampaign{ListingID: "l2", SalePrice: 100, StockCap: 1,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(time.Hour)})
	_, reason, err = e.svc.ReserveFlashSaleStock(context.Background(), "r2", active.ID, "l2", "", 9, 55)
	if err != nil || reason != "price_changed" {
		t.Fatalf("active campaign: reason=%q err=%v, want price_changed", reason, err)
	}
}

// Scenarios: Reserve covers a variant campaign only for that variant; refuses a listing that is not the campaign's.
func TestReserveFlashSaleStockVariantScope(t *testing.T) {
	e := newHoldEnv(t, true)
	vc := e.campaignWith(t, repository.FlashSaleCampaign{ListingID: "l1", VariantID: "v1", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(time.Hour)})
	any := e.campaignWith(t, repository.FlashSaleCampaign{ListingID: "l2", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(time.Hour)})
	ctx := context.Background()
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
	if ok, reason := reserve("c", any.ID, "l2", "v2"); !ok || reason != "" {
		t.Errorf("campaign without variant: ok=%v reason=%q, want true", ok, reason)
	}
	if ok, reason := reserve("d", vc.ID, "other-listing", "v1"); ok || reason != "not_active" {
		t.Errorf("foreign listing: ok=%v reason=%q, want false/not_active", ok, reason)
	}
}

// Scenario: Reserve refuses an unknown campaign.
func TestReserveFlashSaleStockUnknownCampaign(t *testing.T) {
	e := newHoldEnv(t, true)
	ok, reason, err := e.svc.ReserveFlashSaleStock(context.Background(), "r1", "no-such-campaign", "l1", "", 1, 100)
	if err != nil || ok || reason != "not_active" {
		t.Fatalf("ok=%v reason=%q err=%v, want false/not_active/nil", ok, reason, err)
	}
	if e.hasHold(t, "r1") {
		t.Fatal("a hold was created")
	}
}

// Scenario: Reserve replay after release is not active.
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

// Scenario: Reserve replay after expiry is not active.
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

// Scenario: A hold expires 15 minutes after reserve.
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

// Scenario: Concurrent reserves are capped.
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
			ok, reason, err := e.svc.ReserveFlashSaleStock(context.Background(), fmt.Sprintf("r%d", i), c.ID, c.ListingID, "", 1, c.SalePrice)
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

// Scenario: Concurrent reserves of one reservation id hold once.
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
			ok, _, err := e.svc.ReserveFlashSaleStock(context.Background(), "same", c.ID, c.ListingID, "", 2, c.SalePrice)
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
	if h, err := e.holds.Get(context.Background(), "same"); err != nil || h.Quantity != 2 {
		t.Fatalf("hold = %+v err=%v, want one hold of 2", h, err)
	}
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3", got)
	}
}

// Scenario: Concurrent commits and reserves keep the invariant.
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
			if ok, err := e.svc.CommitFlashSaleStock(context.Background(), fmt.Sprintf("h%d", i)); err != nil || !ok {
				t.Errorf("commit h%d: ok=%v err=%v", i, ok, err)
			}
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, _, err := e.svc.ReserveFlashSaleStock(context.Background(), fmt.Sprintf("n%d", i), c.ID, c.ListingID, "", 1, c.SalePrice)
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

// Scenario: Commit adds the quantity to stock_sold.
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

// Scenario: Commit is idempotent.
func TestCommitFlashSaleStockIdempotent(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	if !e.commit(t, "r1") || !e.commit(t, "r1") {
		t.Fatal("both commits must report committed=true")
	}
	if got := e.sold(t, c); got != 2 {
		t.Fatalf("stock_sold = %d, want 2 (added once)", got)
	}
}

// Scenario: Commit of an unknown reservation does nothing.
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

// Scenario: Commit of a released hold is refused.
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

// Scenario: Commit of an expired hold is refused.
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

// Scenario: Commit still works after the sale window ended.
func TestCommitFlashSaleStockAfterWindowEnded(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaignWith(t, repository.FlashSaleCampaign{ListingID: "l1", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(10 * time.Minute)})
	e.mustReserve(t, "r1", c, 2)
	e.clk.advance(12 * time.Minute) // window over, hold 12 minutes old
	if !e.commit(t, "r1") {
		t.Fatal("committed=false, want true after the window ended")
	}
	if got := e.sold(t, c); got != 2 {
		t.Fatalf("stock_sold = %d, want 2", got)
	}
}

// Scenario: Release returns the units of an active hold.
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

// Scenario: Release returns the units of a committed hold.
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

// Scenario: Release is idempotent.
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

// Scenario: Release of an unknown reservation is a no-op.
func TestReleaseFlashSaleStockUnknownNoOp(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 5, 0)
	e.mustReserve(t, "r1", c, 2)
	released, err := e.svc.ReleaseFlashSaleStock(context.Background(), "ghost")
	if err != nil || released {
		t.Fatalf("released=%v err=%v, want false,nil", released, err)
	}
	if got := e.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3 (unchanged)", got)
	}
}

// Scenario: Release of an expired hold changes no counter.
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

// Scenario: A committed hold older than 30 days is not purged (service view).
func TestReleaseOldCommittedHoldAfterPurge(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaignWith(t, repository.FlashSaleCampaign{ListingID: "l1", SalePrice: 100, StockCap: 5,
		StartsAt: svcT0.Add(-time.Hour), EndsAt: svcT0.Add(24 * time.Hour * 365)})
	e.mustReserve(t, "r1", c, 2)
	e.commit(t, "r1")
	e.clk.advance(90 * 24 * time.Hour)
	if _, err := e.holds.PurgeReleasedAndExpiredHolds(context.Background(), e.clk.now().Add(-30*24*time.Hour)); err != nil {
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

// Scenario: Remaining reflects holds and sold units.
func TestGetFlashSaleStockCountsHolds(t *testing.T) {
	e := newHoldEnv(t, true)
	c := e.campaign(t, 10, 3)
	e.mustReserve(t, "expired", c, 1)
	e.mustReserve(t, "released", c, 4)
	e.release(t, "released")
	e.clk.advance(15 * time.Minute) // "expired" is now 15 minutes old
	e.mustReserve(t, "active", c, 2)
	remaining, stockCap, err := e.svc.GetFlashSaleStock(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	if remaining != 5 || stockCap != 10 {
		t.Fatalf("remaining=%d cap=%d, want 5/10", remaining, stockCap)
	}
}

// fixedHeld reports a fixed active-hold total, so the oversold case can be
// built without going through the cap check.
type fixedHeld struct {
	*repository.InMemoryFlashHoldRepository
	n int64
}

func (f fixedHeld) ActiveHeld(context.Context, string, time.Time) (int64, error) { return f.n, nil }

// Scenario: Remaining is floored at zero (with a reserved hold on an oversold campaign).
func TestGetFlashSaleStockRemainingFlooredWithHold(t *testing.T) {
	e := newHoldEnv(t, true)
	e.svc.WithHolds(fixedHeld{InMemoryFlashHoldRepository: e.holds, n: 2})
	c := e.campaign(t, 10, 25)
	remaining, stockCap, err := e.svc.GetFlashSaleStock(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	if remaining != 0 || stockCap != 10 {
		t.Fatalf("remaining=%d cap=%d, want 0/10", remaining, stockCap)
	}
}
