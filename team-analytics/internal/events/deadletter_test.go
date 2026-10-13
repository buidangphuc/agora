package events_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"

	"github.com/buidangphuc/team-analytics/internal/events"
)

func headerMap(rec *kgo.Record) map[string]string {
	m := map[string]string{}
	for _, h := range rec.Headers {
		m[h.Key] = string(h.Value)
	}
	return m
}

// Scenario: a dead-lettered record carries the kafkax dlq-* headers (the v0 "error" and "source_topic" names are gone).
func TestDeadLetterRecordCarriesKafkaxHeaders(t *testing.T) {
	src := consume.Record{
		Topic: "engagement.events", Partition: 3, Offset: 42, Key: []byte("k"), Value: []byte("v"),
		Headers: []consume.Header{
			{Key: kafkax.HeaderEventType, Value: []byte("platform.engagement.v1.FavoriteAdded")},
			{Key: kafkax.HeaderDLQError, Value: []byte("stale failure of an earlier replay")},
		},
	}
	rec := events.DeadLetterRecord("engagement.events.analytics.dlq", "team-analytics.engagement", src, errors.New("unmarshal envelope: bad"))

	assert.Equal(t, "engagement.events.analytics.dlq", rec.Topic)
	assert.Equal(t, []byte("k"), rec.Key)
	assert.Equal(t, []byte("v"), rec.Value)
	h := headerMap(rec)
	assert.Equal(t, "unmarshal envelope: bad", h[kafkax.HeaderDLQError], "the stale dlq-error is replaced")
	assert.Equal(t, "engagement.events", h[kafkax.HeaderDLQSourceTopic])
	assert.Equal(t, "3", h[kafkax.HeaderDLQSourcePartition])
	assert.Equal(t, "42", h[kafkax.HeaderDLQSourceOffset])
	assert.Equal(t, "team-analytics.engagement", h[kafkax.HeaderDLQSourceGroup])
	assert.Equal(t, "platform.engagement.v1.FavoriteAdded", h[kafkax.HeaderEventType], "source headers are kept")
	assert.NotContains(t, h, "error")
	assert.NotContains(t, h, "source_topic")
	n := 0
	for _, x := range rec.Headers {
		if x.Key == kafkax.HeaderDLQError {
			n++
		}
	}
	require.Equal(t, 1, n, "exactly one dlq-error header")
}
