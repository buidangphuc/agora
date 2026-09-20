package consumer

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// OrderPaidEventType is the EventEnvelope.Type discriminator for order settlement events.
const OrderPaidEventType = "platform.order.v1.OrderPaidEvent"

// OrderFactsFromEnvelope decodes an EventEnvelope carrying OrderPaidEvent into one or more OrderFactRecord rows.
func OrderFactsFromEnvelope(value []byte) (records []*warehouse.OrderFactRecord, ok bool, err error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return nil, false, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.GetType() != OrderPaidEventType {
		return nil, false, nil // not ours; skip without error
	}

	var ope orderv1.OrderPaidEvent
	if err := proto.Unmarshal(env.GetPayload(), &ope); err != nil {
		return nil, false, fmt.Errorf("unmarshal OrderPaidEvent: %w", err)
	}

	occurredAt := time.Now().UTC()
	if env.GetOccurredAt() != nil {
		occurredAt = env.GetOccurredAt().AsTime().UTC()
	} else if ope.GetPaidAt() != nil {
		occurredAt = ope.GetPaidAt().AsTime().UTC()
	}

	orderID := ope.GetOrderId()
	currency := ope.GetCurrency()
	if currency == "" {
		currency = "VND"
	}

	for idx, it := range ope.GetItems() {
		rowEventID := fmt.Sprintf("%s-%d", env.GetEventId(), idx)
		records = append(records, &warehouse.OrderFactRecord{
			EventID:    rowEventID,
			OrderID:    orderID,
			ListingID:  it.GetListingId(),
			VariantID:  it.GetVariantId(),
			SellerID:   it.GetSellerId(),
			Quantity:   it.GetQuantity(),
			UnitPrice:  it.GetUnitPrice(),
			Currency:   currency,
			OccurredAt: occurredAt,
			Status:     "PAID",
		})
	}
	return records, true, nil
}
