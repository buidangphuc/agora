package flashsale

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

const (
	defaultCampaignPageSize = 20
	maxCampaignPageSize     = 200
)

type campaignLister interface {
	ListCampaigns(ctx context.Context, statuses []string, now time.Time, offset, limit int) ([]store.FlashSaleCampaign, error)
}

var errListNotSupported = errors.New("flash sale repository does not support listing campaigns")

type CampaignView struct {
	store.FlashSaleCampaign
	Status string
}

type ListCampaignsResult struct {
	Campaigns         []CampaignView
	NextCursor        string
	FlashSaleDisabled bool
}

type ListActiveResult struct {
	Campaigns         []store.FlashSaleCampaign
	NextCursor        string
	FlashSaleDisabled bool
}

func campaignStatus(c store.FlashSaleCampaign, now time.Time) string {
	switch {
	case !c.CancelledAt.IsZero():
		return store.CampaignStatusCancelled
	case !c.StartsAt.IsZero() && c.StartsAt.After(now):
		return store.CampaignStatusScheduled
	case !c.EndsAt.IsZero() && !c.EndsAt.After(now):
		return store.CampaignStatusEnded
	}
	return store.CampaignStatusActive
}

func (s *Service) StatusOf(c store.FlashSaleCampaign) string {
	return campaignStatus(c, s.nowFn())
}

func parseOffset(cursor string) int {
	if cursor == "" {
		return 0
	}
	if n, err := strconv.Atoi(cursor); err == nil && n >= 0 {
		return n
	}
	return 0
}

func (s *Service) ListCampaigns(ctx context.Context, statuses []string, cursor string, pageSize int32) (ListCampaignsResult, error) {
	lister, ok := s.campaigns.(campaignLister)
	if !ok {
		return ListCampaignsResult{}, errListNotSupported
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
	items, err := lister.ListCampaigns(ctx, statuses, now, offset, limit+1)
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

func (s *Service) ListActiveCampaignsPage(ctx context.Context, cursor string, pageSize int32) (ListActiveResult, error) {
	if !s.flashSaleEnabled(ctx) {
		return ListActiveResult{FlashSaleDisabled: true}, nil
	}
	items, next, err := s.ListActiveCampaigns(ctx, cursor, pageSize)
	if err != nil {
		return ListActiveResult{}, err
	}
	return ListActiveResult{Campaigns: items, NextCursor: next}, nil
}
