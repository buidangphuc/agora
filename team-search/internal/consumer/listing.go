package consumer

import (
	"context"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	"github.com/buidangphuc/team-search/internal/events"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/taxonomy"
)

func NewListingEventHandler(idx index.Index, embedder retrieval.EmbedClient, classifier taxonomy.Classifier) Handler {
	h := events.NewListingEventHandler(idx, embedder, classifier)
	return func(ctx context.Context, key, value []byte) error {
		return h(ctx, consume.Record{Key: key, Value: value})
	}
}
