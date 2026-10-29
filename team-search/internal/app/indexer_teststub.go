package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	"github.com/buidangphuc/team-search/internal/config"
)

var errTestStub = errors.New("TEST-STUB: not implemented (task 1.8 replaces this file with indexer.go)")

func RunIndexer(context.Context, config.IndexerSettings) error { return errTestStub }

func newIndexerConsumer(*kgo.Client, consume.Handler, consume.DLQProducer, string, *kafkax.Fingerprinter, *slog.Logger) (*consume.Consumer, error) {
	return nil, errTestStub
}
