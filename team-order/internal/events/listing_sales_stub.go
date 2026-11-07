package events

import (
	"errors"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-order/internal/store"
)

const ListingSalesChangedEventType = "platform.order.v1.ListingSalesChanged"

func ListingSalesChangedEventID(listingID string, soldCount int64, sourceEventID string) string {
	return ""
}

func BuildListingSalesChangedOutboxRow(change store.ListingSalesChange) (outbox.Row, error) {
	return outbox.Row{}, errors.New("not implemented")
}
