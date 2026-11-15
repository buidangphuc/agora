package sponsored

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func isPrecondition(err error) bool {
	return err != nil && (errors.Is(err, store.ErrAdCampaignState) || status.Code(err) == codes.FailedPrecondition)
}

func wantPrecondition(t *testing.T, err error) {
	t.Helper()
	if !isPrecondition(err) {
		t.Fatalf("err = %v, want FAILED_PRECONDITION", err)
	}
}

func TestListMyAdCampaignsClampsPageSize(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	for i := 0; i < 120; i++ {
		_, err := mem.Create(t.Context(), store.AdCampaign{
			ID: fmt.Sprintf("c%03d", i), SellerID: "s1", ListingID: fmt.Sprintf("l%d", i), Bid: 100, Budget: 1000,
			CreatedAt: epoch.Add(-time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	res, err := svc.ListMyAdCampaigns(t.Context(), ListMyAdCampaignsParams{SellerID: "s1", PageSize: 100000})
	if err != nil {
		t.Fatalf("ListMyAdCampaigns: %v", err)
	}
	if len(res.Campaigns) != 100 {
		t.Fatalf("got %d campaigns, want 100", len(res.Campaigns))
	}
	if res.NextCursor == "" {
		t.Fatal("next cursor empty, want one more page")
	}
}

func mgmtSvc(t *testing.T, s seed) (*Service, string) {
	t.Helper()
	svc, _, mem := newServingSvc(t)
	put(t, mem, s)
	svc.repo = mem
	return svc, "seller-" + s.id
}

func TestPauseAdCampaignIdempotent(t *testing.T) {
	svc, seller := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000, spent: 1600, status: store.AdCampaignStatusPaused})
	c, err := svc.PauseAdCampaign(t.Context(), seller, "c1")
	if err != nil {
		t.Fatalf("PauseAdCampaign: %v", err)
	}
	if c.Status != store.AdCampaignStatusPaused || c.Spent != 1600 || c.Budget != 5000 || c.Clicks != 0 {
		t.Fatalf("campaign = %+v, want unchanged PAUSED", c)
	}
	stored := readCampaign(t, svc.repo, "c1")
	if stored.Status != store.AdCampaignStatusPaused || stored.Spent != 1600 || stored.Budget != 5000 {
		t.Fatalf("stored = %+v, want unchanged", stored)
	}
}

func TestPauseAdCampaignFromBudgetPaused(t *testing.T) {
	svc, seller := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000, spent: 4800, status: store.AdCampaignStatusPausedBudget})
	c, err := svc.PauseAdCampaign(t.Context(), seller, "c1")
	if err != nil {
		t.Fatalf("PauseAdCampaign: %v", err)
	}
	if c.Status != store.AdCampaignStatusPaused {
		t.Fatalf("status = %d, want PAUSED", c.Status)
	}
	if got := readCampaign(t, svc.repo, "c1").Status; got != store.AdCampaignStatusPaused {
		t.Fatalf("stored status = %d, want PAUSED", got)
	}
}

func TestPauseAdCampaignEndedRejected(t *testing.T) {
	svc, seller := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000, status: store.AdCampaignStatusEnded})
	_, err := svc.PauseAdCampaign(t.Context(), seller, "c1")
	wantPrecondition(t, err)
	if got := readCampaign(t, svc.repo, "c1").Status; got != store.AdCampaignStatusEnded {
		t.Fatalf("stored status = %d, want ENDED", got)
	}
}

func TestPauseAdCampaignOtherSellerNotFound(t *testing.T) {
	svc, _ := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000})
	_, err := svc.PauseAdCampaign(t.Context(), "someone-else", "c1")
	if !errors.Is(err, store.ErrAdCampaignNotFound) && status.Code(err) != codes.NotFound {
		t.Fatalf("err = %v, want not found", err)
	}
	if got := readCampaign(t, svc.repo, "c1").Status; got != store.AdCampaignStatusActive {
		t.Fatalf("stored status = %d, want ACTIVE", got)
	}
}

func TestResumeAdCampaignRemainingEqualsBid(t *testing.T) {
	svc, seller := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000, spent: 4200, status: store.AdCampaignStatusPaused})
	c, err := svc.ResumeAdCampaign(t.Context(), seller, "c1")
	if err != nil {
		t.Fatalf("ResumeAdCampaign: %v", err)
	}
	if c.Status != store.AdCampaignStatusActive {
		t.Fatalf("status = %d, want ACTIVE", c.Status)
	}
}

func TestResumeAdCampaignInsufficientBudgetRejected(t *testing.T) {
	svc, seller := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000, spent: 4300, status: store.AdCampaignStatusPaused})
	_, err := svc.ResumeAdCampaign(t.Context(), seller, "c1")
	wantPrecondition(t, err)
	if got := readCampaign(t, svc.repo, "c1").Status; got != store.AdCampaignStatusPaused {
		t.Fatalf("stored status = %d, want PAUSED", got)
	}
}

func TestResumeAdCampaignPausedBudgetRejected(t *testing.T) {
	svc, seller := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000, spent: 4300, status: store.AdCampaignStatusPausedBudget})
	_, err := svc.ResumeAdCampaign(t.Context(), seller, "c1")
	wantPrecondition(t, err)
	if got := readCampaign(t, svc.repo, "c1").Status; got != store.AdCampaignStatusPausedBudget {
		t.Fatalf("stored status = %d, want PAUSED_BUDGET", got)
	}
}

func TestResumeAdCampaignEndedRejected(t *testing.T) {
	svc, seller := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000, status: store.AdCampaignStatusEnded})
	_, err := svc.ResumeAdCampaign(t.Context(), seller, "c1")
	wantPrecondition(t, err)
	if got := readCampaign(t, svc.repo, "c1").Status; got != store.AdCampaignStatusEnded {
		t.Fatalf("stored status = %d, want ENDED", got)
	}
}

func TestResumeAdCampaignIdempotent(t *testing.T) {
	svc, seller := mgmtSvc(t, seed{id: "c1", bid: 800, budget: 5000, spent: 1600})
	c, err := svc.ResumeAdCampaign(t.Context(), seller, "c1")
	if err != nil {
		t.Fatalf("ResumeAdCampaign: %v", err)
	}
	if c.Status != store.AdCampaignStatusActive || c.Spent != 1600 {
		t.Fatalf("campaign = %+v, want unchanged ACTIVE", c)
	}
}
