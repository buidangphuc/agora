package service

import (
	"context"
	"time"
)

// TEST-STUB (task 9, compile-only): the implementer replaces this file by
// moving the real methods into lifecycle.go. Both return the number of rows moved.

func (s *OrderService) AutoCompleteShipped(ctx context.Context, now time.Time) (int, error) {
	return 0, nil
}

func (s *OrderService) AutoApproveReturns(ctx context.Context, now time.Time) (int, error) {
	return 0, nil
}
