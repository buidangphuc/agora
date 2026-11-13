package listing

import (
	"context"
	"errors"
	"fmt"
)

func (s *ListingService) WithEntitlements(p EntitlementsPort) *ListingService {
	s.ent = p
	return s
}

func (s *ListingService) createLimit(ctx context.Context, sellerID string) (int64, error) {
	if s.ent == nil {
		return -1, nil
	}
	e, err := s.ent.Get(ctx, sellerID)
	if err != nil {
		if errors.Is(err, ErrEntitlementsUnavailable) {
			return 0, err
		}
		return 0, fmt.Errorf("%w: %w", ErrEntitlementsUnavailable, err)
	}
	return e.MaxListings, nil
}
