package events

import (
	"context"
	"errors"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
)

// HandlerFunc applies one record value (a marshalled EventEnvelope). The
// HandleRaw method of ListingConsumer, ChatConsumer and OrderConsumer satisfies
// it.
type HandlerFunc func(ctx context.Context, value []byte) error

// NewRunner binds a consumer-group client (kafkax.NewConsumerClient) to handle
// through the SDK consume loop. An error wrapping ErrPermanent is dead-lettered
// without retries; any other error is retried with backoff, then dead-lettered.
// The loop commits a record only after it was handled or dead-lettered, and
// Run returns an error, without committing, when a record can be neither.
// The result is a lifecycle component; it fails when the SDK rejects the
// options (an empty dlqTopic).
func NewRunner(client *kgo.Client, handle HandlerFunc, dlq consume.DLQProducer, dlqTopic string, logger *slog.Logger) (*consume.Consumer, error) {
	h := func(ctx context.Context, r consume.Record) error {
		err := handle(ctx, r.Value)
		if errors.Is(err, ErrPermanent) {
			return consume.Permanent(err)
		}
		return err
	}
	return consume.NewConsumer(client, h, dlq, consume.Options{DLQTopic: dlqTopic, Logger: logger})
}
