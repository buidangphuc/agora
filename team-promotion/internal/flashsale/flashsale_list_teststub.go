package flashsale

import (
	"context"

	"github.com/buidangphuc/team-promotion/internal/store"
)

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

func (s *Service) ListCampaigns(_ context.Context, _ []string, _ string, _ int32) (ListCampaignsResult, error) {
	return ListCampaignsResult{}, nil
}

func (s *Service) ListActiveCampaignsPage(_ context.Context, _ string, _ int32) (ListActiveResult, error) {
	return ListActiveResult{}, nil
}
