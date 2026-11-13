package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/config"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/upstream"
)

const (
	backfillBatchSize        = 100
	backfillServicePrincipal = "service-team-promotion"
	backfillListingScope     = "listing.read"
)

type BackfillSummary struct {
	Updated int
	Skipped int
}

type campaignBackfillRepo interface {
	ListUnassignedCampaigns(ctx context.Context, afterID string, limit int) ([]store.FlashSaleCampaign, error)
	AssignCampaignSellers(ctx context.Context, sellers map[string]string) (int, error)
}

type backfillListings interface {
	GetListing(ctx context.Context, in *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error)
}

func BackfillCampaignSellers(ctx context.Context, repo campaignBackfillRepo, listings backfillListings) (BackfillSummary, error) {
	var sum BackfillSummary
	after := ""
	for {
		batch, err := repo.ListUnassignedCampaigns(ctx, after, backfillBatchSize)
		if err != nil {
			return sum, err
		}
		if len(batch) == 0 {
			return sum, nil
		}
		assign := make(map[string]string, len(batch))
		skipped := 0
		for _, c := range batch {
			resp, err := listings.GetListing(ctx, &listingv1.GetListingRequest{Id: c.ListingID})
			if err != nil {
				if status.Code(err) == codes.NotFound {
					skipped++
					continue
				}
				return sum, fmt.Errorf("look up listing for campaign %s: %w", c.ID, err)
			}
			seller := resp.GetListing().GetSellerId()
			if seller == "" {
				skipped++
				continue
			}
			assign[c.ID] = seller
		}
		n, err := repo.AssignCampaignSellers(ctx, assign)
		if err != nil {
			return sum, err
		}
		sum.Updated += n
		sum.Skipped += skipped
		after = batch[len(batch)-1].ID
		if len(batch) < backfillBatchSize {
			return sum, nil
		}
	}
}

func runBackfillCampaignSellers(ctx context.Context) error {
	ms, err := config.LoadMigrate()
	if err != nil {
		return err
	}
	addr := os.Getenv("UPSTREAM_DOMAIN_ADDR")
	if addr == "" {
		return fmt.Errorf("UPSTREAM_DOMAIN_ADDR is required")
	}
	pool, err := postgres.Open(ctx, ms.Postgres)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()
	listings, err := upstream.NewListingClient(addr)
	if err != nil {
		return err
	}
	defer func() { _ = listings.Close() }()

	ctx = sdkupstream.WithServicePrincipal(ctx, backfillServicePrincipal, backfillListingScope)
	sum, err := BackfillCampaignSellers(ctx, store.NewPostgresFlashSaleRepository(pool, nil), listings)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "campaign seller backfill finished", "updated", sum.Updated, "skipped", sum.Skipped)
	return nil
}
