package service

import (
	"errors"
	"time"
)

// TEST-STUB (task 7): compile-only. The implementer deletes this file and adds the
// real definitions to order.go.

// ErrReturnWindowClosed: the order's return window has closed.
var ErrReturnWindowClosed = errors.New("return window closed")

// WithClock replaces the service's wall clock (used by the return-window gate).
func WithClock(now func() time.Time) OrderServiceOption {
	return func(*OrderService) {}
}
