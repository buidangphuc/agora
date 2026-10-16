package events

import (
	"errors"

	"github.com/buidangphuc/team-order/internal/repository"
)

// TEST-STUB (task 7): compile-only. The implementer deletes this file and adds the
// real definitions to publisher.go.

const ReturnRequestedEventType = "platform.order.v1.ReturnRequested"

func ReturnRequestedEventID(returnID string) string { return "" }

func BuildReturnRequestedOutboxRow(ret repository.OrderReturn, currency string) (repository.OutboxRow, error) {
	return repository.OutboxRow{}, errors.New("not implemented")
}
