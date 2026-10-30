package events

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-analytics/generated/platform/listing/v1"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// ListingChangedEventType is the EventEnvelope.Type discriminator of the
// team-domain listing event that carries the listing's owner (seller_id).
const ListingChangedEventType = "platform.listing.v1.ListingChanged"

// ListingSellerFromEnvelope decodes one listing.events record into a
// listing -> seller mapping plus the listing's category and price. A well-formed envelope of another type, or a
// snapshot without a listing id or seller id, is a clean skip (ok=false). The
// change type is deliberately ignored: a DELETED event keeps the mapping so
// historical tracking stays attributable to the seller.
func ListingSellerFromEnvelope(value []byte) (rec *warehouse.ListingSellerRecord, ok bool, err error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return nil, false, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.GetType() != ListingChangedEventType {
		return nil, false, nil
	}
	var lc listingv1.ListingChanged
	if err := proto.Unmarshal(env.GetPayload(), &lc); err != nil {
		return nil, false, fmt.Errorf("unmarshal ListingChanged: %w", err)
	}
	l := lc.GetListing()
	if l.GetId() == "" || l.GetSellerId() == "" {
		return nil, false, nil
	}
	updatedAt := time.Now().UTC()
	if env.GetOccurredAt() != nil {
		updatedAt = env.GetOccurredAt().AsTime().UTC()
	}
	return &warehouse.ListingSellerRecord{
		ListingID: l.GetId(), SellerID: l.GetSellerId(), CategoryID: l.GetCategoryId(), Price: l.GetPrice(), UpdatedAt: updatedAt,
	}, true, nil
}
