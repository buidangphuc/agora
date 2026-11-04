package flashsale

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

type CancelCampaignResult struct {
	Campaign    store.FlashSaleCampaign
	Status      string
	CancelledAt time.Time
}

func (s *Service) CancelCampaign(_ context.Context, _, _ string) (CancelCampaignResult, error) {
	return CancelCampaignResult{}, errors.New("CancelCampaign not implemented")
}
