package sponsored

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func servedToken(t *testing.T, svc *Service, wantCampaign string) string {
	t.Helper()
	res, err := svc.ListSponsoredSlots(t.Context(), SlotsParams{Placement: "home", Limit: 4})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	for _, s := range res.Slots {
		if s.CampaignID == wantCampaign {
			return s.Token
		}
	}
	t.Fatalf("campaign %s not served: %+v", wantCampaign, res.Slots)
	return ""
}

func readCampaign(t *testing.T, repo store.AdCampaignRepository, id string) store.AdCampaign {
	t.Helper()
	c, err := repo.GetByID(t.Context(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return c
}

func TestRecordClickChargesBidOnce(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "c1", bid: 800, budget: 5000})
	token := servedToken(t, svc, "c1")

	res, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token})
	if err != nil {
		t.Fatalf("RecordAdClick: %v", err)
	}
	if !res.Charged || res.ListingID != "listing-c1" {
		t.Fatalf("result = %+v, want charged for listing-c1", res)
	}
	c := readCampaign(t, mem, "c1")
	if c.Spent != 800 || c.Clicks != 1 {
		t.Fatalf("spent=%d clicks=%d, want 800 and 1", c.Spent, c.Clicks)
	}
}

func TestRecordClickReplayChargesNothing(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "c1", bid: 800, budget: 5000})
	token := servedToken(t, svc, "c1")
	if _, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token}); err != nil {
		t.Fatalf("first click: %v", err)
	}

	res, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if res.Charged || res.ListingID != "listing-c1" {
		t.Fatalf("replay = %+v, want uncharged with listing-c1", res)
	}
	c := readCampaign(t, mem, "c1")
	if c.Spent != 800 || c.Clicks != 1 {
		t.Fatalf("spent=%d clicks=%d after replay, want 800 and 1", c.Spent, c.Clicks)
	}
}

func TestRecordClickExpiredChargesNothing(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "c1", bid: 800, budget: 5000})
	token := servedToken(t, svc, "c1")
	svc.WithClock(func() time.Time { return time.Now().Add(31 * time.Minute) })

	res, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token})
	if err != nil {
		t.Fatalf("RecordAdClick: %v", err)
	}
	if res.Charged || res.ListingID != "listing-c1" {
		t.Fatalf("result = %+v, want uncharged with listing-c1", res)
	}
	if c := readCampaign(t, mem, "c1"); c.Spent != 0 || c.Clicks != 0 {
		t.Fatalf("spent=%d clicks=%d, want 0 and 0", c.Spent, c.Clicks)
	}
}

func TestRecordClickUnknownTokenNotFound(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "c1", bid: 800, budget: 5000})

	_, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: "never-issued"})
	if err == nil {
		t.Fatal("want an error for an unknown token")
	}
	if !errors.Is(err, store.ErrImpressionNotFound) && status.Code(err) != codes.NotFound {
		t.Fatalf("err = %v, want ErrImpressionNotFound or NotFound", err)
	}
	if c := readCampaign(t, mem, "c1"); c.Spent != 0 || c.Clicks != 0 {
		t.Fatalf("spent=%d clicks=%d, want 0 and 0", c.Spent, c.Clicks)
	}
}

func TestRecordClickOwnSellerNotCharged(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "c1", bid: 800, budget: 5000})
	token := servedToken(t, svc, "c1")

	res, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token, PrincipalID: "seller-c1"})
	if err != nil {
		t.Fatalf("RecordAdClick: %v", err)
	}
	if res.Charged || res.ListingID != "listing-c1" {
		t.Fatalf("own click = %+v, want uncharged with listing-c1", res)
	}
	if c := readCampaign(t, mem, "c1"); c.Spent != 0 || c.Clicks != 0 {
		t.Fatalf("spent=%d clicks=%d, want 0 and 0", c.Spent, c.Clicks)
	}
	res, err = svc.RecordAdClick(t.Context(), RecordClickParams{Token: token, PrincipalID: "buyer-9"})
	if err != nil || !res.Charged {
		t.Fatalf("token must stay unconsumed after the own click: res=%+v err=%v", res, err)
	}
}

func TestRecordClickOtherPrincipalCharged(t *testing.T) {
	for name, principal := range map[string]string{"other seller": "seller-other", "buyer": "buyer-1", "anonymous": ""} {
		t.Run(name, func(t *testing.T) {
			svc, _, mem := newServingSvc(t)
			put(t, mem, seed{id: "c1", bid: 800, budget: 5000})
			token := servedToken(t, svc, "c1")

			res, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token, PrincipalID: principal})
			if err != nil || !res.Charged {
				t.Fatalf("res=%+v err=%v, want charged", res, err)
			}
		})
	}
}

func TestRecordClickExhaustsBudgetPauses(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "c1", bid: 800, budget: 1000})
	token := servedToken(t, svc, "c1")

	res, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token})
	if err != nil || !res.Charged {
		t.Fatalf("res=%+v err=%v, want charged", res, err)
	}
	c := readCampaign(t, mem, "c1")
	if c.Spent != 800 || c.Status != store.AdCampaignStatusPausedBudget {
		t.Fatalf("spent=%d status=%d, want 800 and PAUSED_BUDGET", c.Spent, c.Status)
	}
}

func TestRecordClickPausedCampaignNotCharged(t *testing.T) {
	for name, status := range map[string]int32{"paused budget": store.AdCampaignStatusPausedBudget, "paused": store.AdCampaignStatusPaused} {
		t.Run(name, func(t *testing.T) {
			svc, _, mem := newServingSvc(t)
			put(t, mem, seed{id: "c1", bid: 800, budget: 5000})
			token := servedToken(t, svc, "c1")
			c := readCampaign(t, mem, "c1")
			c.Status = status
			if _, err := mem.Create(t.Context(), c); err != nil {
				t.Fatalf("flip status: %v", err)
			}

			res, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token})
			if err != nil {
				t.Fatalf("RecordAdClick: %v", err)
			}
			if res.Charged || res.ListingID != "listing-c1" {
				t.Fatalf("result = %+v, want uncharged with listing-c1", res)
			}
			if got := readCampaign(t, mem, "c1"); got.Spent != 0 || got.Clicks != 0 {
				t.Fatalf("spent=%d clicks=%d, want 0 and 0", got.Spent, got.Clicks)
			}
		})
	}
}

type failingClickRepo struct {
	store.AdCampaignRepository
}

func (failingClickRepo) RecordClick(context.Context, string, string, time.Time) (store.ClickResult, error) {
	return store.ClickResult{}, errors.New("db down")
}

func TestRecordClickStoreFailureIsUnavailable(t *testing.T) {
	svc, _, mem := newServingSvc(t)
	put(t, mem, seed{id: "c1", bid: 800, budget: 5000})
	token := servedToken(t, svc, "c1")
	svc.repo = failingClickRepo{AdCampaignRepository: svc.repo}

	_, err := svc.RecordAdClick(t.Context(), RecordClickParams{Token: token})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v, want Unavailable", err)
	}
}
