package events_test

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-gateway/generated/platform/events/v1"
	"github.com/buidangphuc/team-gateway/internal/events"
)

func headerValue(hs []kgoHeader, key string) (string, bool) {
	for _, h := range hs {
		if h.Key == key {
			return string(h.Value), true
		}
	}
	return "", false
}

type kgoHeader struct {
	Key   string
	Value []byte
}

// scenario: A beacon becomes a keyed record with trace headers
func TestKafkaPublisherProducesKeyedRecordWithHeaders(t *testing.T) {
	brokers := testkit.Redpanda(t)
	producer, err := kafkax.NewProducer(config.Kafka{Brokers: brokers})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	t.Cleanup(producer.Close)

	const topic = "analytics.events"
	pub := events.NewKafkaPublisher(producer, topic)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	ctx = reqctx.WithRequestID(ctx, "req-1")

	ev := &analyticsv1.TrackingEvent{SessionId: "s-1", AnonymousId: "a-1", ListingId: "l-1"}
	principal := &commonv1.Principal{}
	if err := pub.PublishTrackingEvent(ctx, ev, principal, "req-1", "evt-1"); err != nil {
		t.Fatalf("PublishTrackingEvent: %v", err)
	}

	recs := testkit.ReadAll(t, brokers, topic, 1, 30*time.Second)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	rec := recs[0]
	if string(rec.Key) != "s-1" {
		t.Fatalf("key = %q, want s-1", rec.Key)
	}

	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.GetType() != "platform.analytics.v1.TrackingEvent" {
		t.Fatalf("envelope type = %q", env.GetType())
	}
	if env.GetEventId() != "evt-1" || env.GetRequestId() != "req-1" {
		t.Fatalf("envelope ids = %q/%q, want evt-1/req-1", env.GetEventId(), env.GetRequestId())
	}
	var got analyticsv1.TrackingEvent
	if err := proto.Unmarshal(env.GetPayload(), &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.GetSessionId() != "s-1" || got.GetListingId() != "l-1" {
		t.Fatalf("payload = %v", &got)
	}

	hs := make([]kgoHeader, len(rec.Headers))
	for i, h := range rec.Headers {
		hs[i] = kgoHeader{Key: h.Key, Value: h.Value}
	}
	if v, ok := headerValue(hs, "x-request-id"); !ok || v != "req-1" {
		t.Fatalf("x-request-id = %q (present=%v), want req-1", v, ok)
	}
	tp, ok := headerValue(hs, "traceparent")
	if !ok || tp != "00-0102030405060708090a0b0c0d0e0f10-0102030405060708-01" {
		t.Fatalf("traceparent = %q (present=%v)", tp, ok)
	}
}

// scenario: A beacon becomes a keyed record with trace headers
func TestKafkaPublisherKeyFallsBackToAnonymousThenListing(t *testing.T) {
	brokers := testkit.Redpanda(t)
	producer, err := kafkax.NewProducer(config.Kafka{Brokers: brokers})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	t.Cleanup(producer.Close)

	const topic = "analytics.events.keys"
	pub := events.NewKafkaPublisher(producer, topic)
	ctx := context.Background()

	if err := pub.PublishTrackingEvent(ctx, &analyticsv1.TrackingEvent{AnonymousId: "a-9", ListingId: "l-9"}, nil, "", ""); err != nil {
		t.Fatalf("publish anon: %v", err)
	}
	if err := pub.PublishTrackingEvent(ctx, &analyticsv1.TrackingEvent{ListingId: "l-9"}, nil, "", ""); err != nil {
		t.Fatalf("publish listing: %v", err)
	}
	recs := testkit.ReadAll(t, brokers, topic, 2, 30*time.Second)
	keys := map[string]bool{}
	for _, r := range recs {
		keys[string(r.Key)] = true
	}
	if !keys["a-9"] || !keys["l-9"] || len(keys) != 2 {
		t.Fatalf("keys = %v, want a-9 and l-9", keys)
	}
}
