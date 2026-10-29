package events

import (
	"context"
	"errors"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/taxonomy"
)

// TEST-STUB: compile-only; the implementer replaces this file with the moved listing.go.
func NewListingEventHandler(_ index.Index, _ retrieval.EmbedClient, _ taxonomy.Classifier) func(context.Context, consume.Record) error {
	return func(context.Context, consume.Record) error { return errors.New("not implemented") }
}
