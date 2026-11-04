package flashsale

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

type listRepo struct {
	*storefake.InMemoryFlashSaleRepository
	seeded []store.FlashSaleCampaign
}

func (r *listRepo) seed(t *testing.T, c store.FlashSaleCampaign) store.FlashSaleCampaign {
	t.Helper()
	got, err := r.InMemoryFlashSaleRepository.Create(t.Context(), c, noEvent)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	r.seeded = append(r.seeded, got)
	return got
}

func (r *listRepo) ListCampaigns(_ context.Context, statuses []string, now time.Time, offset, limit int) ([]store.FlashSaleCampaign, error) {
	var all []store.FlashSaleCampaign
	for _, c := range r.seeded {
		if len(statuses) > 0 && !hasString(statuses, rawStatus(c, now)) {
			continue
		}
		all = append(all, c)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].StartsAt.Equal(all[j].StartsAt) {
			return all[i].StartsAt.After(all[j].StartsAt)
		}
		return all[i].ID < all[j].ID
	})
	if offset >= len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func hasString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func rawStatus(c store.FlashSaleCampaign, now time.Time) string {
	switch {
	case c.StartsAt.After(now):
		return store.CampaignStatusScheduled
	case !c.EndsAt.After(now):
		return store.CampaignStatusEnded
	}
	return store.CampaignStatusActive
}

var listNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func newListSvc(t *testing.T, flagEnabled bool) (*Service, *listRepo) {
	t.Helper()
	repo := &listRepo{InMemoryFlashSaleRepository: storefake.NewInMemoryFlashSaleRepository()}
	svc := New(repo, nil, newFlags(t, flagProvider(flagEnabled)), nil, auditx.NewGate(&recordingAuditWriter{}, nil), noEventBuilder, nil)
	svc.nowFn = func() time.Time { return listNow }
	return svc, repo
}

func noEventBuilder(context.Context, store.FlashSaleCampaign) (outbox.Row, error) {
	return outbox.Row{}, nil
}

func campaignAt(listing string, starts, ends time.Time) store.FlashSaleCampaign {
	return store.FlashSaleCampaign{ListingID: listing, SalePrice: 1, StockCap: 5, StartsAt: starts, EndsAt: ends}
}

func TestListCampaignsNewestFirstAllStatuses(t *testing.T) {
	svc, repo := newListSvc(t, true)
	t1 := repo.seed(t, campaignAt("ended", listNow.Add(-3*time.Hour), listNow.Add(-2*time.Hour)))
	t2 := repo.seed(t, campaignAt("active", listNow.Add(-time.Hour), listNow.Add(time.Hour)))
	t3 := repo.seed(t, campaignAt("scheduled", listNow.Add(time.Hour), listNow.Add(2*time.Hour)))

	res, err := svc.ListCampaigns(t.Context(), nil, "", 0)
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if res.FlashSaleDisabled {
		t.Fatal("FlashSaleDisabled = true with the kill-switch on")
	}
	want := []struct {
		id     string
		status string
	}{{t3.ID, store.CampaignStatusScheduled}, {t2.ID, store.CampaignStatusActive}, {t1.ID, store.CampaignStatusEnded}}
	if len(res.Campaigns) != len(want) {
		t.Fatalf("got %d campaigns, want %d", len(res.Campaigns), len(want))
	}
	for i, w := range want {
		got := res.Campaigns[i]
		if got.ID != w.id || got.Status != w.status {
			t.Fatalf("campaigns[%d] = %s/%q, want %s/%q", i, got.ID, got.Status, w.id, w.status)
		}
	}
}

func TestListCampaignsStatusFilter(t *testing.T) {
	svc, repo := newListSvc(t, true)
	repo.seed(t, campaignAt("ended", listNow.Add(-3*time.Hour), listNow.Add(-2*time.Hour)))
	repo.seed(t, campaignAt("active", listNow.Add(-time.Hour), listNow.Add(time.Hour)))
	sched := repo.seed(t, campaignAt("scheduled", listNow.Add(time.Hour), listNow.Add(2*time.Hour)))

	res, err := svc.ListCampaigns(t.Context(), []string{store.CampaignStatusScheduled}, "", 0)
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if len(res.Campaigns) != 1 || res.Campaigns[0].ID != sched.ID || res.Campaigns[0].Status != store.CampaignStatusScheduled {
		t.Fatalf("campaigns = %+v, want only the scheduled one", res.Campaigns)
	}
}

func TestListCampaignsPaging(t *testing.T) {
	svc, repo := newListSvc(t, true)
	var ids []string
	for i := 0; i < 3; i++ {
		c := repo.seed(t, campaignAt(fmt.Sprintf("l%d", i), listNow.Add(time.Duration(i+1)*time.Hour), listNow.Add(10*time.Hour)))
		ids = append([]string{c.ID}, ids...)
	}

	first, err := svc.ListCampaigns(t.Context(), nil, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Campaigns) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %d campaigns cursor %q, want 2 and a cursor", len(first.Campaigns), first.NextCursor)
	}
	second, err := svc.ListCampaigns(t.Context(), nil, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Campaigns) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %d campaigns cursor %q, want 1 and empty cursor", len(second.Campaigns), second.NextCursor)
	}
	got := []string{first.Campaigns[0].ID, first.Campaigns[1].ID, second.Campaigns[0].ID}
	for i := range ids {
		if got[i] != ids[i] {
			t.Fatalf("paged order = %v, want %v", got, ids)
		}
	}
}

func TestListCampaignsExactPageHasNoCursor(t *testing.T) {
	svc, repo := newListSvc(t, true)
	for i := 0; i < 2; i++ {
		repo.seed(t, campaignAt(fmt.Sprintf("l%d", i), listNow.Add(time.Duration(i+1)*time.Hour), listNow.Add(10*time.Hour)))
	}
	res, err := svc.ListCampaigns(t.Context(), nil, "", 2)
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if len(res.Campaigns) != 2 || res.NextCursor != "" {
		t.Fatalf("got %d campaigns cursor %q, want 2 and empty cursor", len(res.Campaigns), res.NextCursor)
	}
}

func TestCampaignStatusBoundaries(t *testing.T) {
	svc, repo := newListSvc(t, true)
	starts := listNow
	ends := listNow.Add(time.Hour)
	c := repo.seed(t, campaignAt("edge", starts, ends))

	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"one second before starts_at", starts.Add(-time.Second), store.CampaignStatusScheduled},
		{"at starts_at", starts, store.CampaignStatusActive},
		{"one second before ends_at", ends.Add(-time.Second), store.CampaignStatusActive},
		{"at ends_at", ends, store.CampaignStatusEnded},
		{"after ends_at", ends.Add(time.Second), store.CampaignStatusEnded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc.nowFn = func() time.Time { return tc.at }
			res, err := svc.ListCampaigns(t.Context(), nil, "", 0)
			if err != nil {
				t.Fatalf("ListCampaigns: %v", err)
			}
			if len(res.Campaigns) != 1 || res.Campaigns[0].ID != c.ID {
				t.Fatalf("campaigns = %+v, want the single edge campaign", res.Campaigns)
			}
			if got := res.Campaigns[0].Status; got != tc.want {
				t.Fatalf("status at %s = %q, want %q", tc.at.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}

func TestListCampaignsPageSizeClamp(t *testing.T) {
	svc, repo := newListSvc(t, true)
	for i := 0; i < 250; i++ {
		repo.seed(t, campaignAt("l"+strconv.Itoa(i), listNow.Add(time.Duration(i+1)*time.Minute), listNow.Add(24*time.Hour)))
	}
	res, err := svc.ListCampaigns(t.Context(), nil, "", 500)
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if len(res.Campaigns) != 200 || res.NextCursor == "" {
		t.Fatalf("got %d campaigns cursor %q, want 200 and a cursor", len(res.Campaigns), res.NextCursor)
	}
}

func TestListCampaignsDefaultPageSize(t *testing.T) {
	svc, repo := newListSvc(t, true)
	for i := 0; i < 25; i++ {
		repo.seed(t, campaignAt("l"+strconv.Itoa(i), listNow.Add(time.Duration(i+1)*time.Minute), listNow.Add(24*time.Hour)))
	}
	res, err := svc.ListCampaigns(t.Context(), nil, "", 0)
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if len(res.Campaigns) != 20 || res.NextCursor == "" {
		t.Fatalf("got %d campaigns cursor %q, want 20 and a cursor", len(res.Campaigns), res.NextCursor)
	}
}

func TestListCampaignsKillSwitchOffStillReturnsCampaigns(t *testing.T) {
	svc, repo := newListSvc(t, false)
	repo.seed(t, campaignAt("active", listNow.Add(-time.Hour), listNow.Add(time.Hour)))

	res, err := svc.ListCampaigns(t.Context(), nil, "", 0)
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if !res.FlashSaleDisabled {
		t.Fatal("FlashSaleDisabled = false with the kill-switch off")
	}
	if len(res.Campaigns) != 1 {
		t.Fatalf("got %d campaigns, want 1 (list still returned)", len(res.Campaigns))
	}
}

func TestListActiveCampaignsKillSwitchOffReportsDisabled(t *testing.T) {
	svc, repo := newListSvc(t, false)
	repo.seed(t, campaignAt("active", listNow.Add(-time.Hour), listNow.Add(time.Hour)))

	res, err := svc.ListActiveCampaignsPage(t.Context(), "", 0)
	if err != nil {
		t.Fatalf("ListActiveCampaigns: %v", err)
	}
	if !res.FlashSaleDisabled || len(res.Campaigns) != 0 || res.NextCursor != "" {
		t.Fatalf("items=%d next=%q disabled=%v, want empty list with disabled=true", len(res.Campaigns), res.NextCursor, res.FlashSaleDisabled)
	}
}

func TestListActiveCampaignsKillSwitchOnNotDisabled(t *testing.T) {
	svc, repo := newListSvc(t, true)
	repo.seed(t, campaignAt("active", listNow.Add(-time.Hour), listNow.Add(time.Hour)))

	res, err := svc.ListActiveCampaignsPage(t.Context(), "", 0)
	if err != nil {
		t.Fatalf("ListActiveCampaigns: %v", err)
	}
	if res.FlashSaleDisabled || len(res.Campaigns) != 1 {
		t.Fatalf("items=%d disabled=%v, want 1 item and disabled=false", len(res.Campaigns), res.FlashSaleDisabled)
	}
}
