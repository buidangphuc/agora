package flashsale

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

type sellerCampaigns interface {
	ListMyCampaigns(ctx context.Context, sellerID string, statuses []string, now time.Time, offset, limit int) ([]store.FlashSaleCampaign, error)
	CancelIfScheduledOwned(ctx context.Context, id, sellerID string, now time.Time) (time.Time, error)
}

var errSellerNotSupported = errors.New("flash sale repository does not support seller campaigns")

func (s *Service) ListMyCampaigns(ctx context.Context, sellerID string, statuses []string, cursor string, pageSize int32) (ListCampaignsResult, error) {
	repo, ok := s.campaigns.(sellerCampaigns)
	if !ok {
		return ListCampaignsResult{}, errSellerNotSupported
	}
	limit := int(pageSize)
	if limit <= 0 {
		limit = defaultCampaignPageSize
	}
	if limit > maxCampaignPageSize {
		limit = maxCampaignPageSize
	}
	offset := parseOffset(cursor)
	now := s.nowFn()
	items, err := repo.ListMyCampaigns(ctx, sellerID, statuses, now, offset, limit+1)
	if err != nil {
		return ListCampaignsResult{}, err
	}
	res := ListCampaignsResult{FlashSaleDisabled: !s.flashSaleEnabled(ctx)}
	if len(items) > limit {
		items = items[:limit]
		res.NextCursor = strconv.Itoa(offset + limit)
	}
	res.Campaigns = make([]CampaignView, 0, len(items))
	for _, c := range items {
		res.Campaigns = append(res.Campaigns, CampaignView{FlashSaleCampaign: c, Status: campaignStatus(c, now)})
	}
	return res, nil
}

func (s *Service) CancelMyCampaign(ctx context.Context, campaignID, sellerID string) (CancelCampaignResult, error) {
	repo, ok := s.campaigns.(sellerCampaigns)
	if !ok {
		return CancelCampaignResult{}, errSellerNotSupported
	}
	at, err := repo.CancelIfScheduledOwned(ctx, campaignID, sellerID, s.nowFn())
	if err != nil {
		return CancelCampaignResult{}, err
	}
	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return CancelCampaignResult{}, err
	}
	return CancelCampaignResult{Campaign: c, Status: store.CampaignStatusCancelled, CancelledAt: at}, nil
}
