package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	engagementv1 "github.com/buidangphuc/team-search/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-search/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-search/generated/platform/order/v1"
	"github.com/buidangphuc/team-search/internal/events"
)

const (
	salesType  = "platform.order.v1.ListingSalesChanged"
	ratingType = "platform.engagement.v1.ListingRatingChanged"
)

var changedAt = time.Unix(1700000000, 123456789)

func TestSalesHandler_BR1_AppliesAbsoluteValueAtChangedAtNanos(t *testing.T) {
	idx := &mockIndex{}
	h := events.NewListingSalesHandler(idx)
	msg := &orderv1.ListingSalesChanged{ListingId: "l1", SoldCount: 5, ChangedAt: timestamppb.New(changedAt)}
	if err := h(context.Background(), rec(envelopeAt(t, salesType, msg, time.Time{}))); err != nil {
		t.Fatal(err)
	}
	if len(idx.salesCalls) != 1 {
		t.Fatalf("want 1 UpdateSales, got %d", len(idx.salesCalls))
	}
	got := idx.salesCalls[0]
	if got.id != "l1" || got.sold != 5 || got.version != changedAt.UnixNano() {
		t.Errorf("UpdateSales = %+v, want l1/5/%d", got, changedAt.UnixNano())
	}
	if len(idx.ratingCalls) != 0 || idx.lastUpsertDoc.ID != "" || idx.lastPartialID != "" {
		t.Errorf("sales event touched something else: %+v", idx)
	}
}

func TestSalesHandler_BR3_ZeroAndLowerValuesAreForwardedForTheIndexGuard(t *testing.T) {
	idx := &mockIndex{}
	h := events.NewListingSalesHandler(idx)
	for _, sold := range []int64{0, 3} {
		msg := &orderv1.ListingSalesChanged{ListingId: "l1", SoldCount: sold, ChangedAt: timestamppb.New(changedAt)}
		if err := h(context.Background(), rec(envelopeAt(t, salesType, msg, time.Time{}))); err != nil {
			t.Fatalf("sold_count %d: %v", sold, err)
		}
	}
	if len(idx.salesCalls) != 2 || idx.salesCalls[0].sold != 0 || idx.salesCalls[1].sold != 3 {
		t.Errorf("calls = %+v", idx.salesCalls)
	}
}

func TestSalesHandler_BR7_BR7a_MalformedEventReturnsErrorAndIsNotApplied(t *testing.T) {
	cases := map[string]*orderv1.ListingSalesChanged{
		"negative sold_count": {ListingId: "l1", SoldCount: -1, ChangedAt: timestamppb.New(changedAt)},
		"empty listing_id":    {SoldCount: 2, ChangedAt: timestamppb.New(changedAt)},
		"no changed_at":       {ListingId: "l1", SoldCount: 2},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			idx := &mockIndex{}
			if err := events.NewListingSalesHandler(idx)(context.Background(), rec(envelopeAt(t, salesType, msg, changedAt))); err == nil {
				t.Fatal("expected an error so consume retries and parks the record on the DLQ")
			}
			if len(idx.salesCalls) != 0 {
				t.Errorf("malformed event applied: %+v", idx.salesCalls)
			}
		})
	}
	t.Run("undecodable envelope", func(t *testing.T) {
		idx := &mockIndex{}
		if err := events.NewListingSalesHandler(idx)(context.Background(), rec([]byte{0xff, 0xff, 0xff})); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("undecodable payload", func(t *testing.T) {
		value, _ := proto.Marshal(&eventsv1.EventEnvelope{Type: salesType, Payload: []byte{0xff, 0xff, 0xff}})
		idx := &mockIndex{}
		if err := events.NewListingSalesHandler(idx)(context.Background(), rec(value)); err == nil {
			t.Fatal("expected an error")
		}
		if len(idx.salesCalls) != 0 {
			t.Errorf("applied: %+v", idx.salesCalls)
		}
	})
}

func TestSalesHandler_BR7_IndexFailureIsReturnedForRetry(t *testing.T) {
	idx := &mockIndex{updateErr: errors.New("opensearch down")}
	msg := &orderv1.ListingSalesChanged{ListingId: "l1", SoldCount: 1, ChangedAt: timestamppb.New(changedAt)}
	if err := events.NewListingSalesHandler(idx)(context.Background(), rec(envelopeAt(t, salesType, msg, time.Time{}))); err == nil {
		t.Fatal("an index failure must be returned so the record is retried")
	}
}

func TestRatingHandler_BR2_AppliesAverageAndCountAtChangedAtNanos(t *testing.T) {
	idx := &mockIndex{}
	h := events.NewListingRatingHandler(idx)
	msg := &engagementv1.ListingRatingChanged{ListingId: "l1", AverageRating: 4.5, ReviewCount: 2, ChangedAt: timestamppb.New(changedAt)}
	if err := h(context.Background(), rec(envelopeAt(t, ratingType, msg, time.Time{}))); err != nil {
		t.Fatal(err)
	}
	if len(idx.ratingCalls) != 1 {
		t.Fatalf("want 1 UpdateRating, got %d", len(idx.ratingCalls))
	}
	got := idx.ratingCalls[0]
	if got.id != "l1" || got.avg != 4.5 || got.reviews != 2 || got.version != changedAt.UnixNano() {
		t.Errorf("UpdateRating = %+v", got)
	}
	if len(idx.salesCalls) != 0 {
		t.Errorf("rating event touched sales: %+v", idx.salesCalls)
	}
}

func TestRatingHandler_BR2_BoundaryValuesAreAccepted(t *testing.T) {
	cases := map[string]*engagementv1.ListingRatingChanged{
		"zero average and zero reviews": {ListingId: "l1", AverageRating: 0, ReviewCount: 0, ChangedAt: timestamppb.New(changedAt)},
		"average 5":                     {ListingId: "l1", AverageRating: 5, ReviewCount: 1, ChangedAt: timestamppb.New(changedAt)},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			idx := &mockIndex{}
			if err := events.NewListingRatingHandler(idx)(context.Background(), rec(envelopeAt(t, ratingType, msg, time.Time{}))); err != nil {
				t.Fatal(err)
			}
			if len(idx.ratingCalls) != 1 {
				t.Errorf("not applied: %+v", idx.ratingCalls)
			}
		})
	}
}

func TestRatingHandler_BR7_BR7a_MalformedEventReturnsErrorAndIsNotApplied(t *testing.T) {
	cases := map[string]*engagementv1.ListingRatingChanged{
		"average above 5":       {ListingId: "l1", AverageRating: 7, ReviewCount: 1, ChangedAt: timestamppb.New(changedAt)},
		"average just above 5":  {ListingId: "l1", AverageRating: 5.01, ReviewCount: 1, ChangedAt: timestamppb.New(changedAt)},
		"average below 0":       {ListingId: "l1", AverageRating: -0.1, ReviewCount: 1, ChangedAt: timestamppb.New(changedAt)},
		"negative review_count": {ListingId: "l1", AverageRating: 4, ReviewCount: -1, ChangedAt: timestamppb.New(changedAt)},
		"empty listing_id":      {AverageRating: 4, ReviewCount: 1, ChangedAt: timestamppb.New(changedAt)},
		"no changed_at":         {ListingId: "l1", AverageRating: 4, ReviewCount: 1},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			idx := &mockIndex{}
			if err := events.NewListingRatingHandler(idx)(context.Background(), rec(envelopeAt(t, ratingType, msg, changedAt))); err == nil {
				t.Fatal("expected an error so consume retries and parks the record on the DLQ")
			}
			if len(idx.ratingCalls) != 0 {
				t.Errorf("malformed event applied: %+v", idx.ratingCalls)
			}
		})
	}
	t.Run("undecodable envelope", func(t *testing.T) {
		idx := &mockIndex{}
		if err := events.NewListingRatingHandler(idx)(context.Background(), rec([]byte{0xff, 0xff, 0xff})); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestRatingHandler_BR7_IndexFailureIsReturnedForRetry(t *testing.T) {
	idx := &mockIndex{updateErr: errors.New("opensearch down")}
	msg := &engagementv1.ListingRatingChanged{ListingId: "l1", AverageRating: 4, ReviewCount: 1, ChangedAt: timestamppb.New(changedAt)}
	if err := events.NewListingRatingHandler(idx)(context.Background(), rec(envelopeAt(t, ratingType, msg, time.Time{}))); err == nil {
		t.Fatal("an index failure must be returned so the record is retried")
	}
}
