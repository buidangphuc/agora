package events

import (
	"context"
	"strconv"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
)

// DeadLetterRecord builds the record published to dlqTopic for src: the same key
// and value, the source headers (except an earlier dlq-*, which the new failure
// replaces) and the kafkax dlq-* headers naming the cause, the source topic,
// partition and offset, and the consumer group. The SDK consumers build the same
// record for a whole batch; this is for a service that dead-letters single
// records inside a batch handler.
func DeadLetterRecord(dlqTopic, group string, src consume.Record, cause error) *kgo.Record {
	headers := make([]kgo.RecordHeader, 0, len(src.Headers)+5)
	for _, h := range src.Headers {
		if !strings.HasPrefix(h.Key, "dlq-") {
			headers = append(headers, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
	}
	headers = append(headers,
		kgo.RecordHeader{Key: kafkax.HeaderDLQError, Value: []byte(cause.Error())},
		kgo.RecordHeader{Key: kafkax.HeaderDLQSourceTopic, Value: []byte(src.Topic)},
		kgo.RecordHeader{Key: kafkax.HeaderDLQSourcePartition, Value: []byte(strconv.FormatInt(int64(src.Partition), 10))},
		kgo.RecordHeader{Key: kafkax.HeaderDLQSourceOffset, Value: []byte(strconv.FormatInt(src.Offset, 10))},
	)
	if group != "" {
		headers = append(headers, kgo.RecordHeader{Key: kafkax.HeaderDLQSourceGroup, Value: []byte(group)})
	}
	return &kgo.Record{Topic: dlqTopic, Key: src.Key, Value: src.Value, Headers: headers}
}

// clientDLQ publishes dead-letter records through a consumer's own client, which
// is how these consumers have always produced to their DLQ topic.
type clientDLQ struct{ client *kgo.Client }

// PublishRecord implements consume.DLQProducer.
func (d clientDLQ) PublishRecord(ctx context.Context, r *kgo.Record) error {
	return d.client.ProduceSync(ctx, r).FirstErr()
}

// groupOf returns the consumer group client was built with.
func groupOf(client *kgo.Client) string {
	g, _ := client.OptValue(kgo.ConsumerGroup).(string)
	return g
}
