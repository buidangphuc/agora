package storefake

import (
	"context"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func (r *InMemoryAdCampaignRepository) ListServable(context.Context, string, int) ([]store.AdCampaign, error) {
	return nil, nil
}

func (r *InMemoryAdCampaignRepository) IssueImpressions(context.Context, []store.ImpressionIssue) error {
	return nil
}
