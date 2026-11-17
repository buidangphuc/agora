package payment

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-payment/internal/store"
)

func (s *Service) GetPayoutObligations(ctx context.Context, userID string) (store.PayoutObligations, error) {
	if userID == "" {
		return store.PayoutObligations{}, errors.New("user id is required")
	}
	reader, ok := s.ledgerRepo.(store.ObligationsReader)
	if !ok {
		return store.PayoutObligations{}, ErrLedgerNotConfigured
	}
	return reader.PayoutObligations(ctx, userID, store.Holdback{Window: s.holdWindow, Now: s.now()})
}
