package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/store"
)

var oldPeriodEnd = erasedAt.Add(10 * 24 * time.Hour)

func seedErasureSubscription(t *testing.T, pool *pgxpool.Pool, id, seller, plan, status string, autoRenew bool, pending string) {
	t.Helper()
	var pendingArg any
	if pending != "" {
		pendingArg = pending
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO seller_subscriptions
		(id, seller_id, plan_id, status, current_period_start, current_period_end, auto_renew, pending_plan_id, period_seq)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,3)`, id, seller, plan, status, oldPeriodEnd.Add(-30*24*time.Hour), oldPeriodEnd, autoRenew, pendingArg)
	if err != nil {
		t.Fatalf("seed subscription %s: %v", id, err)
	}
}

func seedErasureCampaign(t *testing.T, pool *pgxpool.Pool, id, seller string, status int32, spent, clicks int64) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `INSERT INTO ad_campaigns (id, seller_id, listing_id, budget, bid, status, spent, clicks, impressions, category_id)
		VALUES ($1,$2,$3,5000,800,$4,$5,$6,7,'cat-erase')`, id, seller, "l-"+id, status, spent, clicks)
	if err != nil {
		t.Fatalf("seed campaign %s: %v", id, err)
	}
}

func subscriptionRows(t *testing.T, pool *pgxpool.Pool, seller string) string {
	return rowJSON(t, pool, `SELECT row_to_json(s)::text FROM seller_subscriptions s WHERE seller_id = $1`, seller)
}

func campaignRows(t *testing.T, pool *pgxpool.Pool, ids ...string) string {
	return rowJSON(t, pool, `SELECT row_to_json(c)::text FROM ad_campaigns c WHERE id = ANY($1)`, ids)
}

type subState struct {
	id, plan, status string
	autoRenew        bool
	pending          *string
	seq              int64
	end              time.Time
}

func readSub(t *testing.T, pool *pgxpool.Pool, seller string) subState {
	t.Helper()
	var s subState
	if err := pool.QueryRow(t.Context(), `SELECT id, plan_id, status, auto_renew, pending_plan_id, period_seq, current_period_end
		FROM seller_subscriptions WHERE seller_id = $1`, seller).Scan(&s.id, &s.plan, &s.status, &s.autoRenew, &s.pending, &s.seq, &s.end); err != nil {
		t.Fatalf("read subscription of %s: %v", seller, err)
	}
	return s
}

func campaignCounters(t *testing.T, pool *pgxpool.Pool, id string) (status int32, budget, spent, impressions, clicks int64) {
	t.Helper()
	if err := pool.QueryRow(t.Context(), `SELECT status, budget, spent, impressions, clicks FROM ad_campaigns WHERE id = $1`, id).
		Scan(&status, &budget, &spent, &impressions, &clicks); err != nil {
		t.Fatalf("read campaign %s: %v", id, err)
	}
	return
}

func TestEraseUserEndsPaidSubscription(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresErasureRepository(pool)
	subs := store.NewPostgresSubscriptionRepository(pool)
	seedErasureSubscription(t, pool, "sub-u", "U", "plan_pro", store.SubscriptionStatusActive, true, "plan_free")
	seedErasureSubscription(t, pool, "sub-w", "W", "plan_premium", store.SubscriptionStatusActive, true, "")
	seedErasureSubscription(t, pool, "sub-v", "V", "plan_free", store.SubscriptionStatusExpired, false, "")
	wBefore := subscriptionRows(t, pool, "W")
	vBefore := subscriptionRows(t, pool, "V")

	if err := repo.EraseUser(t.Context(), "U", erasedAt); err != nil {
		t.Fatalf("EraseUser: %v", err)
	}

	got := readSub(t, pool, "U")
	if got.id != "sub-u" || got.plan != "plan_free" || got.status != store.SubscriptionStatusCancelled ||
		got.autoRenew || got.pending != nil || got.seq != 3 || !got.end.Equal(oldPeriodEnd) {
		t.Errorf("U subscription after erasure = %+v, want sub-u plan_free cancelled no-renew no-pending seq 3 end %v", got, oldPeriodEnd)
	}
	if _, err := subs.GetActiveSubscriptionBySeller(t.Context(), "U"); !errors.Is(err, store.ErrSubscriptionNotFound) {
		t.Errorf("GetActiveSubscriptionBySeller(U) err = %v, want ErrSubscriptionNotFound", err)
	}
	if after := subscriptionRows(t, pool, "W"); after != wBefore {
		t.Errorf("W subscription changed:\nbefore %s\nafter  %s", wBefore, after)
	}

	for _, user := range []string{"V", "X"} {
		if err := repo.EraseUser(t.Context(), user, erasedAt); err != nil {
			t.Fatalf("EraseUser(%s): %v", user, err)
		}
	}
	if after := subscriptionRows(t, pool, "V"); after != vBefore {
		t.Errorf("V expired FREE subscription changed:\nbefore %s\nafter  %s", vBefore, after)
	}
	if got := subscriptionRows(t, pool, "X"); got != "" {
		t.Errorf("a subscription row appeared for X: %s", got)
	}
}

func TestEraseUserSubscriptionNotDueForRenewal(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresErasureRepository(pool)
	subs := store.NewPostgresSubscriptionRepository(pool)
	seedErasureSubscription(t, pool, "sub-u", "U", "plan_pro", store.SubscriptionStatusActive, true, "")

	if err := repo.EraseUser(t.Context(), "U", erasedAt); err != nil {
		t.Fatalf("EraseUser: %v", err)
	}

	due, err := subs.ListDueForRenewal(t.Context(), oldPeriodEnd.Add(time.Hour), 100)
	if err != nil {
		t.Fatalf("ListDueForRenewal: %v", err)
	}
	for _, d := range due {
		if d.SellerID == "U" {
			t.Fatalf("erased seller U is due for renewal: %+v", d)
		}
	}
}

func TestEraseUserEndsAdCampaigns(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresErasureRepository(pool)
	seedErasureCampaign(t, pool, "c-active", "U", store.AdCampaignStatusActive, 800, 1)
	seedErasureCampaign(t, pool, "c-paused", "U", store.AdCampaignStatusPaused, 0, 0)
	seedErasureCampaign(t, pool, "c-budget", "U", store.AdCampaignStatusPausedBudget, 4800, 6)
	seedErasureCampaign(t, pool, "c-ended", "U", store.AdCampaignStatusEnded, 1600, 2)
	seedErasureCampaign(t, pool, "c-w", "W", store.AdCampaignStatusActive, 0, 0)
	endedBefore := campaignRows(t, pool, "c-ended")
	wBefore := campaignRows(t, pool, "c-w")
	want := map[string][2]int64{"c-active": {800, 1}, "c-paused": {0, 0}, "c-budget": {4800, 6}}

	if err := repo.EraseUser(t.Context(), "U", erasedAt); err != nil {
		t.Fatalf("EraseUser: %v", err)
	}

	for id, w := range want {
		status, budget, spent, impressions, clicks := campaignCounters(t, pool, id)
		if status != store.AdCampaignStatusEnded || budget != 5000 || spent != w[0] || impressions != 7 || clicks != w[1] {
			t.Errorf("%s = status %d budget %d spent %d impressions %d clicks %d, want ENDED 5000 %d 7 %d",
				id, status, budget, spent, impressions, clicks, w[0], w[1])
		}
	}
	if got := campaignRows(t, pool, "c-ended"); got != endedBefore {
		t.Errorf("already ended campaign changed:\nbefore %s\nafter  %s", endedBefore, got)
	}
	if got := campaignRows(t, pool, "c-w"); got != wBefore {
		t.Errorf("W campaign changed:\nbefore %s\nafter  %s", wBefore, got)
	}
}

func TestEraseUserCampaignsNotServed(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresErasureRepository(pool)
	ads := store.NewPostgresAdCampaignRepository(pool)
	seedErasureCampaign(t, pool, "c-u", "U", store.AdCampaignStatusActive, 0, 0)
	if err := ads.IssueImpressions(t.Context(), []store.ImpressionIssue{{Token: "tok-u", CampaignID: "c-u", ListingID: "l-c-u"}}); err != nil {
		t.Fatalf("IssueImpressions: %v", err)
	}

	if err := repo.EraseUser(t.Context(), "U", erasedAt); err != nil {
		t.Fatalf("EraseUser: %v", err)
	}

	served, err := ads.ListServable(t.Context(), "cat-erase", 10)
	if err != nil {
		t.Fatalf("ListServable: %v", err)
	}
	for _, c := range served {
		if c.ID == "c-u" {
			t.Fatalf("erased seller's campaign is still served: %+v", c)
		}
	}
	res, err := ads.RecordClick(t.Context(), "tok-u", "buyer-1", time.Now())
	if err != nil {
		t.Fatalf("RecordClick: %v", err)
	}
	if res.Charged {
		t.Error("click on a pre-erasure token was charged")
	}
	if _, _, spent, _, clicks := campaignCounters(t, pool, "c-u"); spent != 0 || clicks != 0 {
		t.Errorf("campaign spent %d clicks %d after erasure, want 0 0", spent, clicks)
	}
}

func TestEraseUserMonetizationRollsBack(t *testing.T) {
	s := seedErasure(t)
	_, err := s.pool.Exec(t.Context(), `CREATE FUNCTION erasure_ads_fail() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'injected failure'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER erasure_ads_fail BEFORE UPDATE ON ad_campaigns FOR EACH ROW EXECUTE FUNCTION erasure_ads_fail();`)
	if err != nil {
		t.Fatalf("install failure: %v", err)
	}
	before := everything(t, s.pool)

	err = s.repo.EraseUser(t.Context(), "U", erasedAt)
	if err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("EraseUser = %v, want the store error returned for retry", err)
	}
	if got := everything(t, s.pool); got != before {
		t.Fatalf("partial erasure after failure:\nbefore %s\nafter  %s", before, got)
	}
	if sub := readSub(t, s.pool, "U"); sub.status != store.SubscriptionStatusActive || !sub.autoRenew || sub.plan != "plan_pro" {
		t.Errorf("subscription changed although the transaction failed: %+v", sub)
	}

	if _, err := s.pool.Exec(t.Context(), `DROP TRIGGER erasure_ads_fail ON ad_campaigns`); err != nil {
		t.Fatal(err)
	}
	if err := s.repo.EraseUser(t.Context(), "U", erasedAt); err != nil {
		t.Fatalf("retry after recovery: %v", err)
	}
	if sub := readSub(t, s.pool, "U"); sub.status != store.SubscriptionStatusCancelled || sub.autoRenew {
		t.Errorf("subscription not ended after recovery: %+v", sub)
	}
	if status, _, _, _, _ := campaignCounters(t, s.pool, "c-u"); status != store.AdCampaignStatusEnded {
		t.Errorf("campaign status %d after recovery, want ENDED", status)
	}
}
