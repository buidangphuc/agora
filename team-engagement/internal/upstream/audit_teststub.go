package upstream

// TEST-STUB (task 3, red): compile-only. The implementer deletes this file and
// adds the real method to audit.go.

import (
	"context"
	"errors"
)

func (c *AuditClient) WriteReportResolveEvent(ctx context.Context, actorID, reportID, listingID, status, resolutionNote string) error {
	return errors.New("not implemented")
}
