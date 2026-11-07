package events

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	engagementv1 "github.com/buidangphuc/team-search/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-search/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-search/generated/platform/order/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

const (
	listingSalesChangedType  = "platform.order.v1.ListingSalesChanged"
	listingRatingChangedType = "platform.engagement.v1.ListingRatingChanged"
	maxRating                = 5.0
)

func NewListingSalesHandler(idx index.Index) consume.Handler {
	return func(ctx context.Context, rec consume.Record) error {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(rec.Value, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		if env.GetType() != listingSalesChangedType {
			return nil
		}
		var ev orderv1.ListingSalesChanged
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			return fmt.Errorf("unmarshal ListingSalesChanged: %w", err)
		}
		if ev.GetListingId() == "" {
			return fmt.Errorf("ListingSalesChanged has no listing id")
		}
		if ev.GetSoldCount() < 0 {
			return fmt.Errorf("ListingSalesChanged %q: negative sold_count %d", ev.GetListingId(), ev.GetSoldCount())
		}
		if ev.GetChangedAt() == nil {
			return fmt.Errorf("ListingSalesChanged %q: missing changed_at", ev.GetListingId())
		}
		return idx.UpdateSales(ctx, ev.GetListingId(), ev.GetSoldCount(), ev.GetChangedAt().AsTime().UnixNano())
	}
}

func NewListingRatingHandler(idx index.Index) consume.Handler {
	return func(ctx context.Context, rec consume.Record) error {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(rec.Value, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		if env.GetType() != listingRatingChangedType {
			return nil
		}
		var ev engagementv1.ListingRatingChanged
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			return fmt.Errorf("unmarshal ListingRatingChanged: %w", err)
		}
		if ev.GetListingId() == "" {
			return fmt.Errorf("ListingRatingChanged has no listing id")
		}
		if avg := ev.GetAverageRating(); !(avg >= 0 && avg <= maxRating) {
			return fmt.Errorf("ListingRatingChanged %q: average_rating %v outside 0..5", ev.GetListingId(), avg)
		}
		if ev.GetReviewCount() < 0 {
			return fmt.Errorf("ListingRatingChanged %q: negative review_count %d", ev.GetListingId(), ev.GetReviewCount())
		}
		if ev.GetChangedAt() == nil {
			return fmt.Errorf("ListingRatingChanged %q: missing changed_at", ev.GetListingId())
		}
		return idx.UpdateRating(ctx, ev.GetListingId(), ev.GetAverageRating(), ev.GetReviewCount(), ev.GetChangedAt().AsTime().UnixNano())
	}
}
