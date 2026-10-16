package service

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-order/internal/repository"
)

// TEST-STUB (task 6): compile-only placeholder so the red tests fail on behaviour.
// The implementer deletes this file and implements ConfirmReceipt in order.go.
func (s *OrderService) ConfirmReceipt(ctx context.Context, id string) (repository.Order, error) {
	return repository.Order{}, errors.New("TEST-STUB: ConfirmReceipt not implemented")
}
