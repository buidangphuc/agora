package flashsale

import (
	"context"
	"errors"
	"time"

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

func (s *Service) UpdateCampaign(_ context.Context, _ UpdateCampaignParams) (store.FlashSaleCampaign, error) {
	return store.FlashSaleCampaign{}, errors.New("TEST-STUB: UpdateCampaign not implemented")
}
