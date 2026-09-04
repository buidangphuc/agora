package consumer

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-search/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-search/generated/platform/listing/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

// Discriminator types carried in EventEnvelope.Type
const (
	listingChangedType         = "platform.listing.v1.ListingChanged"
	listingBaseInfoChangedType = "platform.listing.v1.ListingBaseInfoChanged"
	listingPricingChangedType  = "platform.listing.v1.ListingPricingChanged"
	listingStockChangedType    = "platform.listing.v1.ListingStockChanged"
	listingStatusChangedType   = "platform.listing.v1.ListingStatusChanged"
)

// ListingEventHandler decodes listing events and applies changes to OpenSearch read-model.
// Fine-grained events perform Partial Updates on OpenSearch to save CPU/Memory and avoid re-indexing text.
func ListingEventHandler(idx index.Index) Handler {
	return func(ctx context.Context, _ []byte, value []byte) error {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(value, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}

		// version (AD2): the read-model version is the event's occurred_at in
		// nanoseconds, so out-of-order/redelivered events are ordered by the guard.
		version := envVersion(&env)

		switch env.GetType() {
		case listingChangedType:
			var changed listingv1.ListingChanged
			if err := proto.Unmarshal(env.GetPayload(), &changed); err != nil {
				return fmt.Errorf("unmarshal ListingChanged: %w", err)
			}
			l := changed.GetListing()
			if l == nil || l.GetId() == "" {
				return fmt.Errorf("event has no listing id")
			}
			if changed.GetChangeType() == listingv1.ChangeType_CHANGE_TYPE_DELETED {
				return idx.Delete(ctx, l.GetId())
			}
			return idx.Upsert(ctx, toDoc(l, version))

		case listingBaseInfoChangedType:
			var base listingv1.ListingBaseInfoChanged
			if err := proto.Unmarshal(env.GetPayload(), &base); err != nil {
				return fmt.Errorf("unmarshal ListingBaseInfoChanged: %w", err)
			}
			if base.GetListingId() == "" {
				return fmt.Errorf("event has no listing id")
			}
			if base.GetChangeType() == listingv1.ChangeType_CHANGE_TYPE_DELETED {
				return idx.Delete(ctx, base.GetListingId())
			}
			// Partial update base descriptive fields
			return idx.PartialUpdate(ctx, base.GetListingId(), map[string]interface{}{
				"id":          base.GetListingId(),
				"title":       base.GetTitle(),
				"description": base.GetDescription(),
				"category_id": base.GetCategoryId(),
				"seller_id":   base.GetSellerId(),
				"status":      statusString(base.GetStatus()),
				"version":     version,
			})

		case listingPricingChangedType:
			var pricing listingv1.ListingPricingChanged
			if err := proto.Unmarshal(env.GetPayload(), &pricing); err != nil {
				return fmt.Errorf("unmarshal ListingPricingChanged: %w", err)
			}
			if pricing.GetListingId() == "" {
				return fmt.Errorf("event has no listing id")
			}
			effectivePrice := pricing.GetOriginalPrice()
			if pricing.GetIsOnSale() && pricing.GetPromotionalPrice() > 0 {
				effectivePrice = pricing.GetPromotionalPrice()
			}
			// Partial update price field in OpenSearch without re-indexing text
			return idx.PartialUpdate(ctx, pricing.GetListingId(), map[string]interface{}{
				"price":    effectivePrice,
				"currency": pricing.GetCurrency(),
				"version":  version,
			})

		case listingStatusChangedType:
			var st listingv1.ListingStatusChanged
			if err := proto.Unmarshal(env.GetPayload(), &st); err != nil {
				return fmt.Errorf("unmarshal ListingStatusChanged: %w", err)
			}
			if st.GetListingId() == "" {
				return fmt.Errorf("event has no listing id")
			}
			if st.GetStatus() == listingv1.ListingStatus_LISTING_STATUS_REJECTED {
				return idx.Delete(ctx, st.GetListingId())
			}
			return idx.PartialUpdate(ctx, st.GetListingId(), map[string]interface{}{
				"status":  statusString(st.GetStatus()),
				"version": version,
			})

		default:
			return nil // not ours; ignore
		}
	}
}

// toDoc maps a proto Listing to the indexed document, stamping the read-model
// version (AD2) so the index can reject out-of-order writes.
func toDoc(l *listingv1.Listing, version int64) index.ListingDoc {
	return index.ListingDoc{
		ID:          l.GetId(),
		Title:       l.GetTitle(),
		Description: l.GetDescription(),
		Status:      statusString(l.GetStatus()),
		Currency:    l.GetCurrency(),
		Price:       l.GetPrice(),
		CategoryID:  l.GetCategoryId(),
		SellerID:    l.GetSellerId(),
		Version:     version,
	}
}

// envVersion derives the monotonic read-model version from the envelope's
// occurred_at (nanoseconds). Returns 0 when the timestamp is absent, which
// disables the version guard for that write.
func envVersion(env *eventsv1.EventEnvelope) int64 {
	ts := env.GetOccurredAt()
	if ts == nil {
		return 0
	}
	return ts.AsTime().UnixNano()
}

func statusString(s listingv1.ListingStatus) string {
	switch s {
	case listingv1.ListingStatus_LISTING_STATUS_DRAFT:
		return "draft"
	case listingv1.ListingStatus_LISTING_STATUS_PUBLISHED:
		return "published"
	case listingv1.ListingStatus_LISTING_STATUS_REJECTED:
		return "rejected"
	default:
		return "unspecified"
	}
}
