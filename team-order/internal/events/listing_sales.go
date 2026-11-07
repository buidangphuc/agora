package events

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

const ListingSalesChangedEventType = "platform.order.v1.ListingSalesChanged"

var listingSalesNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-order/order.events/ListingSalesChanged"))

func ListingSalesChangedEventID(listingID string, soldCount int64, sourceEventID string) string {
	return uuid.NewSHA1(listingSalesNamespace, []byte(listingID+"\x00"+strconv.FormatInt(soldCount, 10)+"\x00"+sourceEventID)).String()
}

func BuildListingSalesChangedOutboxRow(change store.ListingSalesChange) (outbox.Row, error) {
	eventID := ListingSalesChangedEventID(change.ListingID, change.SoldCount, change.SourceEventID)
	payload, err := proto.Marshal(&orderv1.ListingSalesChanged{
		ListingId: change.ListingID,
		SoldCount: change.SoldCount,
		ChangedAt: timestamppb.New(change.ChangedAt.UTC()),
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal ListingSalesChanged: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       ListingSalesChangedEventType,
		OccurredAt: timestamppb.New(change.ChangedAt.UTC()),
		Payload:    payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "Listing",
		AggregateID:   change.ListingID,
		EventType:     ListingSalesChangedEventType,
		Payload:       value,
	}, nil
}
