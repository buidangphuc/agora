package events

import (
	"github.com/google/uuid"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/store"
)

// EnqueueDeleted builds the ListingChanged(DELETED) outbox row for a listing
// deleted by seller erasure, attributed to the team-domain service principal.
func EnqueueDeleted() store.EnqueueFn {
	principal := &commonv1.Principal{Id: servicePrincipalID, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE}
	return func(l store.Listing) (store.OutboxRow, error) {
		eventID := uuid.NewString()
		payload, err := BuildListingChangedEnvelope(eventID, listingWire(l), listingv1.ChangeType_CHANGE_TYPE_DELETED, principal, "")
		if err != nil {
			return store.OutboxRow{}, err
		}
		return store.OutboxRow{
			EventID:       eventID,
			AggregateType: "Listing",
			AggregateID:   l.ID,
			EventType:     ListingChangedEventType,
			Payload:       payload,
		}, nil
	}
}
