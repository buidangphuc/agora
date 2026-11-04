package flashsale

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type UpdateCampaignParams struct {
	CampaignID string
	SalePrice  int64
	StockCap   int64
	StartsAt   time.Time
	EndsAt     time.Time
	ActorID    string
}

type campaignUpdater interface {
	UpdateIfScheduled(ctx context.Context, id string, c store.FlashSaleCampaign, now time.Time) (store.FlashSaleCampaign, error)
}

var errUpdateNotConfigured = errors.New("flash sale update is not supported by the repository")

func (s *Service) UpdateCampaign(ctx context.Context, p UpdateCampaignParams) (store.FlashSaleCampaign, error) {
	updater, ok := s.campaigns.(campaignUpdater)
	if !ok {
		return store.FlashSaleCampaign{}, errUpdateNotConfigured
	}
	cur, err := s.campaigns.GetByID(ctx, p.CampaignID)
	if err != nil {
		return store.FlashSaleCampaign{}, err
	}
	now := s.nowFn()
	switch {
	case p.StockCap <= 0:
		return store.FlashSaleCampaign{}, fmt.Errorf("%w: stock_cap must be positive", ErrInvalidCampaign)
	case p.SalePrice <= 0:
		return store.FlashSaleCampaign{}, fmt.Errorf("%w: sale_price must be positive", ErrInvalidCampaign)
	case !p.EndsAt.After(p.StartsAt) || !p.EndsAt.After(now):
		return store.FlashSaleCampaign{}, fmt.Errorf("%w: ends_at must be after starts_at and in the future", ErrInvalidCampaign)
	}
	resp, err := s.listings.GetListing(ctx, &listingv1.GetListingRequest{Id: cur.ListingID})
	if err != nil {
		if status.Convert(err).Proto().GetCode() == int32(code.Code_NOT_FOUND) {
			return store.FlashSaleCampaign{}, fmt.Errorf("%w: listing_id does not exist", ErrInvalidCampaign)
		}
		s.logger.WarnContext(ctx, "listing price lookup failed", "listing_id", cur.ListingID, "err", err)
		return store.FlashSaleCampaign{}, listing.ErrLookupFailed
	}
	if p.SalePrice >= resp.GetListing().GetPrice() {
		return store.FlashSaleCampaign{}, fmt.Errorf("%w: sale_price must be below the listing price", ErrInvalidCampaign)
	}
	next := store.FlashSaleCampaign{
		SalePrice: p.SalePrice,
		StockCap:  p.StockCap,
		StartsAt:  p.StartsAt,
		EndsAt:    p.EndsAt,
	}
	var updated store.FlashSaleCampaign
	ev := auditx.Event{
		ActorID:    p.ActorID,
		Action:     "campaign.update",
		TargetType: "flash_sale",
		TargetID:   p.CampaignID,
	}
	err = s.audit.Do(ctx, ev, func(ctx context.Context) error {
		var err error
		updated, err = updater.UpdateIfScheduled(ctx, p.CampaignID, next, s.nowFn())
		return err
	})
	if err != nil {
		return store.FlashSaleCampaign{}, err
	}
	return updated, nil
}
