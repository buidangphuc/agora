package flashsale

import (
	"context"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type CancelCampaignResult struct {
	Campaign    store.FlashSaleCampaign
	Status      string
	CancelledAt time.Time
}

type campaignCanceller interface {
	Cancel(ctx context.Context, id string, now time.Time) (time.Time, error)
}

func (s *Service) CancelCampaign(ctx context.Context, campaignID, actorID string) (CancelCampaignResult, error) {
	canceller, ok := s.campaigns.(campaignCanceller)
	if !ok {
		return CancelCampaignResult{}, errCancelNotConfigured
	}
	var at time.Time
	ev := auditx.Event{
		ActorID:    actorID,
		Action:     "campaign.cancel",
		TargetType: "flash_sale",
		TargetID:   campaignID,
	}
	err := s.audit.Do(ctx, ev, func(ctx context.Context) error {
		var err error
		at, err = canceller.Cancel(ctx, campaignID, s.nowFn())
		return err
	})
	if err != nil {
		return CancelCampaignResult{}, err
	}
	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return CancelCampaignResult{}, err
	}
	return CancelCampaignResult{Campaign: c, Status: store.CampaignStatusCancelled, CancelledAt: at}, nil
}
