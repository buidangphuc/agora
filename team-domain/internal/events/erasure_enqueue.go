package events

import "github.com/buidangphuc/team-domain/internal/store"

// RED-PHASE STUB: replaced by the implementer.
func EnqueueDeleted() store.EnqueueFn {
	return func(store.Listing) (store.OutboxRow, error) { return store.OutboxRow{}, nil }
}
