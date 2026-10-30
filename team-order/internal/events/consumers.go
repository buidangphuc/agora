package events

import (
	"math"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	"github.com/buidangphuc/team-order/internal/config"
)

func NewPaymentConsumer(client *kgo.Client, h consume.Handler, dlq consume.DLQProducer, keyFP *kafkax.Fingerprinter, s config.Settings) (*consume.Consumer, error) {
	return consume.NewConsumer(client, h, dlq, consume.Options{
		DLQTopic: s.Events.PaymentDLQ,
		KeyFP:    keyFP,
	})
}

func NewDisputeConsumer(client *kgo.Client, h consume.Handler, dlq consume.DLQProducer, keyFP *kafkax.Fingerprinter, s config.Settings) (*consume.Consumer, error) {
	return consume.NewConsumer(client, h, dlq, consume.Options{
		DLQTopic:       s.Events.EngagementDLQ,
		MaxAttempts:    math.MaxInt32,
		MaxBackoff:     5 * time.Second,
		MaxPollRecords: 1,
		KeyFP:          keyFP,
	})
}
