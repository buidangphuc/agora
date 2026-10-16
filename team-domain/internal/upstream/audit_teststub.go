package upstream

// TEST-STUB (task 8): compile-only placeholder; the implementer deletes this
// file and adds WriteListingModerate to audit.go.

import (
	"context"
	"errors"
)

func (c *AuditClient) WriteListingModerate(ctx context.Context, actorID, listingID, sellerID, decision, reasonCode, note, fromStatus, toStatus string) error {
	return errors.New("not implemented")
}
