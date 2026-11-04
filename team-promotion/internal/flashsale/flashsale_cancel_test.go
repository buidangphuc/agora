package flashsale

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

type cancelState struct {
	mu sync.Mutex
	at map[string]time.Time
}

func (s *cancelState) cancelledAt(id string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.at[id]
	return at, ok
}

type cancelRepo struct {
	*storefake.InMemoryFlashSaleRepository
	state *cancelState
	log   *callLog
}

func (r *cancelRepo) Cancel(ctx context.Context, id string, now time.Time) (time.Time, error) {
	r.log.add("store")
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	c, err := r.InMemoryFlashSaleRepository.GetByID(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	if at, ok := r.state.at[id]; ok {
		return at, nil
	}
	if !c.EndsAt.IsZero() && !c.EndsAt.After(now) {
		return time.Time{}, store.ErrCampaignEnded
	}
	r.state.at[id] = now
	return now, nil
}

type cancelHolds struct {
	*storefake.InMemoryFlashHoldRepository
	state *cancelState
}

func (h *cancelHolds) Reserve(ctx context.Context, hold store.FlashHold, now time.Time) (store.FlashHold, bool, error) {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	if _, ok := h.state.at[hold.CampaignID]; ok {
		return store.FlashHold{}, false, store.ErrCampaignCancelled
	}
	return h.InMemoryFlashHoldRepository.Reserve(ctx, hold, now)
}

type cancelEnv struct {
	svc    *Service
	repo   *cancelRepo
	holds  *cancelHolds
	audit  *recordingAuditWriter
	log    *callLog
	clk    *fakeClock
	events *eventLog
}

func newCancelEnv(t *testing.T) cancelEnv {
	t.Helper()
	log := &callLog{}
	state := &cancelState{at: map[string]time.Time{}}
	base := storefake.NewInMemoryFlashSaleRepository()
	repo := &cancelRepo{InMemoryFlashSaleRepository: base, state: state, log: log}
	holds := &cancelHolds{InMemoryFlashHoldRepository: storefake.NewInMemoryFlashHoldRepository(base), state: state}
	aw := &recordingAuditWriter{log: log}
	events := &eventLog{}
	clk := &fakeClock{t: svcT0}
	svc := New(repo, holds, newFlags(t, flagProvider(true)), nil, auditx.NewGate(aw, nil), events.build, nil)
	svc.nowFn = clk.now
	return cancelEnv{svc: svc, repo: repo, holds: holds, audit: aw, log: log, clk: clk, events: events}
}

func (e cancelEnv) seed(t *testing.T, stockCap, stockSold int64, startsAt, endsAt time.Time) store.FlashSaleCampaign {
	t.Helper()
	c, err := e.repo.Create(t.Context(), store.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 90000, StockCap: stockCap, StockSold: stockSold, StartsAt: startsAt, EndsAt: endsAt,
	}, noEvent)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return c
}

func (e cancelEnv) seedActive(t *testing.T, stockCap, stockSold int64) store.FlashSaleCampaign {
	t.Helper()
	return e.seed(t, stockCap, stockSold, svcT0.Add(-time.Hour), svcT0.Add(24*time.Hour))
}

func TestCancelCampaignAuditBeforeWrite(t *testing.T) {
	e := newCancelEnv(t)
	c := e.seedActive(t, 10, 0)

	res, err := e.svc.CancelCampaign(t.Context(), c.ID, "admin-1")
	if err != nil {
		t.Fatalf("CancelCampaign: %v", err)
	}
	if res.Status != store.CampaignStatusCancelled {
		t.Fatalf("status = %q, want %q", res.Status, store.CampaignStatusCancelled)
	}
	if res.Campaign.ID != c.ID {
		t.Fatalf("campaign id = %q, want %q", res.Campaign.ID, c.ID)
	}
	if !res.CancelledAt.Equal(svcT0) {
		t.Fatalf("cancelled_at = %v, want %v", res.CancelledAt, svcT0)
	}
	calls := e.log.get()
	if len(calls) != 2 || calls[0] != "audit" || calls[1] != "store" {
		t.Fatalf("call order = %v, want [audit store]", calls)
	}
	evs := e.audit.got()
	if len(evs) != 1 {
		t.Fatalf("audit events = %d, want 1", len(evs))
	}
	ev := evs[0]
	if ev.Action != "campaign.cancel" || ev.TargetType != "flash_sale" || ev.TargetID != c.ID || ev.ActorID != "admin-1" {
		t.Fatalf("audit event = %+v, want campaign.cancel flash_sale/%s by admin-1", ev, c.ID)
	}
	if at, ok := e.repo.state.cancelledAt(c.ID); !ok || !at.Equal(svcT0) {
		t.Fatalf("stored cancelled_at = %v ok=%v, want %v", at, ok, svcT0)
	}
}

func TestCancelCampaignAuditDownLeavesCampaignUnchanged(t *testing.T) {
	for name, failure := range auditFailures {
		t.Run(name, func(t *testing.T) {
			e := newCancelEnv(t)
			e.audit.err = failure
			c := e.seedActive(t, 10, 0)

			_, err := e.svc.CancelCampaign(t.Context(), c.ID, "admin-1")
			assertAuditUnavailable(t, err)
			for _, call := range e.log.get() {
				if call == "store" {
					t.Fatalf("store was written despite the audit failure: %v", e.log.get())
				}
			}
			if _, ok := e.repo.state.cancelledAt(c.ID); ok {
				t.Fatal("campaign is cancelled despite the audit failure")
			}
			ok, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), "r1", c.ID, c.ListingID, "", 1, c.SalePrice)
			if err != nil || !ok {
				t.Fatalf("reserve after refused cancel = %v %q %v, want reserved", ok, reason, err)
			}
		})
	}
}

func TestCancelCampaignIdempotent(t *testing.T) {
	e := newCancelEnv(t)
	c := e.seedActive(t, 10, 0)

	first, err := e.svc.CancelCampaign(t.Context(), c.ID, "admin-1")
	if err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	e.clk.advance(10 * time.Minute)
	second, err := e.svc.CancelCampaign(t.Context(), c.ID, "admin-1")
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if first.Status != store.CampaignStatusCancelled || second.Status != store.CampaignStatusCancelled {
		t.Fatalf("statuses = %q, %q, want both cancelled", first.Status, second.Status)
	}
	if first.CancelledAt.IsZero() || !first.CancelledAt.Equal(second.CancelledAt) {
		t.Fatalf("cancelled_at first=%v second=%v, want identical and non-zero", first.CancelledAt, second.CancelledAt)
	}
}

func TestCancelCampaignEndedRefused(t *testing.T) {
	e := newCancelEnv(t)
	c := e.seed(t, 10, 0, svcT0.Add(-2*time.Hour), svcT0.Add(-time.Hour))

	_, err := e.svc.CancelCampaign(t.Context(), c.ID, "admin-1")
	if !errors.Is(err, store.ErrCampaignEnded) {
		t.Fatalf("err = %v, want store.ErrCampaignEnded", err)
	}
	if _, ok := e.repo.state.cancelledAt(c.ID); ok {
		t.Fatal("ended campaign was marked cancelled")
	}
}

func TestCancelCampaignEndedAtBoundaryRefused(t *testing.T) {
	e := newCancelEnv(t)
	c := e.seed(t, 10, 0, svcT0.Add(-2*time.Hour), svcT0)

	if _, err := e.svc.CancelCampaign(t.Context(), c.ID, "admin-1"); !errors.Is(err, store.ErrCampaignEnded) {
		t.Fatalf("err = %v, want store.ErrCampaignEnded when ends_at == now", err)
	}
}

func TestCancelCampaignUnknownNotFound(t *testing.T) {
	e := newCancelEnv(t)

	_, err := e.svc.CancelCampaign(t.Context(), "does-not-exist", "admin-1")
	if !errors.Is(err, store.ErrCampaignNotFound) {
		t.Fatalf("err = %v, want store.ErrCampaignNotFound", err)
	}
}

func TestCancelCampaignScheduledIsCancellable(t *testing.T) {
	e := newCancelEnv(t)
	c := e.seed(t, 10, 0, svcT0.Add(time.Hour), svcT0.Add(2*time.Hour))

	res, err := e.svc.CancelCampaign(t.Context(), c.ID, "admin-1")
	if err != nil || res.Status != store.CampaignStatusCancelled {
		t.Fatalf("cancel scheduled = %+v, %v; want cancelled", res, err)
	}
}

func TestCancelCampaignStopsNewHoldsKeepsExistingAndSold(t *testing.T) {
	e := newCancelEnv(t)
	c := e.seedActive(t, 10, 2)
	ctx := t.Context()

	ok, reason, err := e.svc.ReserveFlashSaleStock(ctx, "held-1", c.ID, c.ListingID, "", 1, c.SalePrice)
	if err != nil || !ok {
		t.Fatalf("pre-cancel reserve = %v %q %v, want reserved", ok, reason, err)
	}
	if _, err := e.svc.CancelCampaign(ctx, c.ID, "admin-1"); err != nil {
		t.Fatalf("CancelCampaign: %v", err)
	}

	ok, reason, err = e.svc.ReserveFlashSaleStock(ctx, "late-1", c.ID, c.ListingID, "", 1, c.SalePrice)
	if err != nil {
		t.Fatalf("post-cancel reserve returned error %v, want not reserved with reason", err)
	}
	if ok || reason != "not_active" {
		t.Fatalf("post-cancel reserve = %v %q, want false not_active", ok, reason)
	}
	if _, err := e.holds.Get(ctx, "late-1"); !errors.Is(err, store.ErrFlashHoldNotFound) {
		t.Fatalf("refused reserve left a hold: err = %v", err)
	}

	committed, err := e.svc.CommitFlashSaleStock(ctx, "held-1")
	if err != nil || !committed {
		t.Fatalf("commit of pre-cancel hold = %v, %v, want true", committed, err)
	}
	got, err := e.repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.StockSold != 3 {
		t.Fatalf("stock_sold = %d, want 3", got.StockSold)
	}
}

func TestCancelCampaignKeepsReleaseOfExistingHold(t *testing.T) {
	e := newCancelEnv(t)
	c := e.seedActive(t, 10, 0)
	ctx := t.Context()

	if ok, _, err := e.svc.ReserveFlashSaleStock(ctx, "held-1", c.ID, c.ListingID, "", 2, c.SalePrice); err != nil || !ok {
		t.Fatalf("reserve: %v %v", ok, err)
	}
	if _, err := e.svc.CancelCampaign(ctx, c.ID, "admin-1"); err != nil {
		t.Fatalf("CancelCampaign: %v", err)
	}
	found, err := e.svc.ReleaseFlashSaleStock(ctx, "held-1")
	if err != nil || !found {
		t.Fatalf("release of pre-cancel hold = %v, %v, want found", found, err)
	}
	got, _ := e.repo.GetByID(ctx, c.ID)
	if got.StockSold != 0 {
		t.Fatalf("stock_sold = %d, want 0", got.StockSold)
	}
}

func TestCancelCampaignReplayOfPreCancelReservationStillReserved(t *testing.T) {
	e := newCancelEnv(t)
	c := e.seedActive(t, 10, 0)
	ctx := t.Context()

	if ok, _, err := e.svc.ReserveFlashSaleStock(ctx, "held-1", c.ID, c.ListingID, "", 1, c.SalePrice); err != nil || !ok {
		t.Fatalf("reserve: %v %v", ok, err)
	}
	if _, err := e.svc.CancelCampaign(ctx, c.ID, "admin-1"); err != nil {
		t.Fatalf("CancelCampaign: %v", err)
	}
	ok, reason, err := e.svc.ReserveFlashSaleStock(ctx, "held-1", c.ID, c.ListingID, "", 1, c.SalePrice)
	if err != nil || !ok || reason != "" {
		t.Fatalf("replay of existing hold = %v %q %v, want reserved", ok, reason, err)
	}
}

func TestCancelCampaignRacesReserve(t *testing.T) {
	const stockCap, workers = 5, 40
	e := newCancelEnv(t)
	c := e.seedActive(t, stockCap, 0)

	type outcome struct {
		afterCancel bool
		ok          bool
		reason      string
		err         error
	}
	var cancelled atomic.Bool
	results := make([]outcome, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			after := cancelled.Load()
			ok, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), fmt.Sprintf("r%d", i), c.ID, c.ListingID, "", 1, c.SalePrice)
			results[i] = outcome{afterCancel: after, ok: ok, reason: reason, err: err}
		}(i)
	}
	var cancelErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, cancelErr = e.svc.CancelCampaign(t.Context(), c.ID, "admin-1")
		cancelled.Store(true)
	}()
	close(start)
	wg.Wait()

	if cancelErr != nil {
		t.Fatalf("CancelCampaign: %v", cancelErr)
	}
	held := 0
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("reserve %d returned error %v", i, r.err)
		}
		if r.ok {
			held++
			if r.afterCancel {
				t.Fatalf("reserve %d began after the cancel returned yet was reserved", i)
			}
			continue
		}
		if r.afterCancel && r.reason != "not_active" {
			t.Fatalf("reserve %d after cancel: reason %q, want not_active", i, r.reason)
		}
	}
	if held > stockCap {
		t.Fatalf("holds = %d, exceeds stock cap %d", held, stockCap)
	}
	ok, reason, err := e.svc.ReserveFlashSaleStock(t.Context(), "final", c.ID, c.ListingID, "", 1, c.SalePrice)
	if err != nil || ok || reason != "not_active" {
		t.Fatalf("reserve after everything = %v %q %v, want false not_active", ok, reason, err)
	}
}
