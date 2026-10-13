package settlement_test

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/settlement"
)

func paidEnvelope(t *testing.T, orderID, seller string) []byte {
	t.Helper()
	raw, err := proto.Marshal(&orderv1.OrderPaidEvent{
		OrderId: orderID, TotalAmount: 100, Currency: "VND",
		Items: []*orderv1.OrderLineItemFact{{ListingId: "l", SellerId: seller, Quantity: 1, UnitPrice: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: "paid-" + orderID, Type: settlement.OrderPaidEventType, Payload: raw, OccurredAt: timestamppb.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
