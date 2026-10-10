package consumer

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-search/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-search/generated/platform/listing/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

// stockOnlyIndex counts UpdateStock calls; any other method panics via the nil
// embedded interface, which would fail the test.
type stockOnlyIndex struct {
	index.Index
	calls int
}

func (s *stockOnlyIndex) UpdateStock(context.Context, string, int32, int64) error {
	s.calls++
	return nil
}

// D2 + AD1: a malformed ListingStockChanged is retried, then parked on the DLQ
// with its original key and bytes, and never applied.
func TestProcessRecord_MalformedStockEventReachesDLQ(t *testing.T) {
	payload, _ := proto.Marshal(&listingv1.ListingStockChanged{ListingId: "l1", Stock: -1})
	value, _ := proto.Marshal(&eventsv1.EventEnvelope{
		Type: listingStockChangedType, OccurredAt: timestamppb.New(time.Unix(1700000000, 0)), Payload: payload,
	})
	idx := &stockOnlyIndex{}
	dlq := &fakeDLQ{}
	if err := testConsumer(dlq).processRecord(context.Background(), []byte("l1"), value, ListingEventHandler(idx), discardLogger()); err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	if len(dlq.records) != 1 || string(dlq.records[0][0]) != "l1" || string(dlq.records[0][1]) != string(value) {
		t.Fatalf("want the record parked unchanged, got %d records", len(dlq.records))
	}
	if idx.calls != 0 {
		t.Errorf("malformed stock event was applied %d times", idx.calls)
	}
}
