package sponsored

import "context"

type SlotsParams struct {
	ContextStr string
	Placement  string
	CategoryID string
	Limit      int32
}

type Slot struct {
	ListingID  string
	CampaignID string
	Token      string
}

type SlotsResult struct {
	ListingIDs []string
	Slots      []Slot
}

func (s *Service) WithMinBid(minBid int64) *Service {
	s.minBid = minBid
	return s
}

func (s *Service) ListSponsoredSlots(context.Context, SlotsParams) (SlotsResult, error) {
	return SlotsResult{}, nil
}
