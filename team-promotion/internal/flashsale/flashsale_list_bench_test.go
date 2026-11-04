package flashsale

import (
	"fmt"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/flags"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

func BenchmarkListCampaignsPage50(b *testing.B) {
	repo := &listRepo{InMemoryFlashSaleRepository: storefake.NewInMemoryFlashSaleRepository()}
	for i := 0; i < 500; i++ {
		starts := listNow.Add(time.Duration(i-250) * time.Hour)
		c := campaignAt(fmt.Sprintf("listing-%d", i), starts, starts.Add(time.Hour))
		got, err := repo.InMemoryFlashSaleRepository.Create(b.Context(), c, noEvent)
		if err != nil {
			b.Fatalf("seed campaign: %v", err)
		}
		repo.seeded = append(repo.seeded, got)
	}
	fl, err := flags.NewWithProvider(b.Context(), flagProvider(true), time.Second, nil)
	if err != nil {
		b.Fatalf("flags.NewWithProvider: %v", err)
	}
	svc := New(repo, nil, fl, nil, auditx.NewGate(&recordingAuditWriter{}, nil), noEventBuilder, nil)
	svc.nowFn = func() time.Time { return listNow }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := svc.ListCampaigns(b.Context(), []string{store.CampaignStatusEnded}, "", 50)
		if err != nil {
			b.Fatalf("ListCampaigns: %v", err)
		}
		if len(res.Campaigns) != 50 {
			b.Fatalf("page size = %d, want 50", len(res.Campaigns))
		}
	}
}
