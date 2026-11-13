package store

import (
	"context"
	"errors"
)

const AdCampaignStatusPausedBudget int32 = 4

type ImpressionIssue struct {
	Token      string
	CampaignID string
	ListingID  string
}

var errServingNotImplemented = errors.New("sponsored serving not implemented")

func (r *PostgresAdCampaignRepository) ListServable(context.Context, string, int) ([]AdCampaign, error) {
	return nil, errServingNotImplemented
}

func (r *PostgresAdCampaignRepository) IssueImpressions(context.Context, []ImpressionIssue) error {
	return errServingNotImplemented
}
