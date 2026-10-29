package events_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-analytics/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	"github.com/buidangphuc/team-analytics/internal/events"
)

type hDLQSink struct {
	got []*kgo.Record
	err error
}

func (s *hDLQSink) PublishRecord(_ context.Context, r *kgo.Record) error {
	s.got = append(s.got, r)
	return s.err
}

func hTrackingDLQRecord(t *testing.T, consent *analyticsv1.ConsentState) *kgo.Record {
	t.Helper()
	te := &analyticsv1.TrackingEvent{
		EventType:  analyticsv1.EventType_EVENT_TYPE_VIEW,
		ListingId:  "lst-9",
		SessionId:  "sess-9",
		GaClientId: "123.456",
		Consent:    consent,
	}
	env := &eventsv1.EventEnvelope{
		EventId:   "evt-9",
		Type:      hTrackingType,
		Payload:   hMarshal(t, te),
		Principal: &commonv1.Principal{Id: "user-9"},
	}
	return &kgo.Record{
		Topic:   "analytics.events.dlq",
		Key:     []byte("user-9"),
		Value:   hMarshal(t, env),
		Headers: []kgo.RecordHeader{{Key: "dlq-error", Value: []byte("boom")}, {Key: "x-request-id", Value: []byte("r1")}},
	}
}

func hDecodeTracking(t *testing.T, value []byte) (*eventsv1.EventEnvelope, *analyticsv1.TrackingEvent) {
	t.Helper()
	env := &eventsv1.EventEnvelope{}
	require.NoError(t, proto.Unmarshal(value, env))
	te := &analyticsv1.TrackingEvent{}
	require.NoError(t, proto.Unmarshal(env.GetPayload(), te))
	return env, te
}

func hRedactor(t *testing.T, sink *hDLQSink) *events.RedactingDLQ {
	t.Helper()
	buf, fp := hLogging(t)
	return events.NewRedactingDLQ(sink, fp, logx.NewTo(buf, hRuntime(), "test"))
}

func TestRedactingDLQBlanksGAClientIDWithoutConsent(t *testing.T) {
	sink := &hDLQSink{}
	in := hTrackingDLQRecord(t, &analyticsv1.ConsentState{Version: 1, Analytics: false})

	require.NoError(t, hRedactor(t, sink).PublishRecord(context.Background(), in))

	require.Len(t, sink.got, 1, "scenario: A dead-letter copy blanks the client id without consent")
	out := sink.got[0]
	env, te := hDecodeTracking(t, out.Value)
	assert.Empty(t, te.GetGaClientId())
	assert.Equal(t, "lst-9", te.GetListingId())
	assert.Equal(t, "sess-9", te.GetSessionId())
	assert.Equal(t, "user-9", env.GetPrincipal().GetId())
	assert.Equal(t, "evt-9", env.GetEventId())
	assert.Equal(t, in.Key, out.Key)
	assert.Equal(t, in.Topic, out.Topic)
	assert.Equal(t, in.Headers, out.Headers)
}

func TestRedactingDLQKeepsGAClientIDWithConsent(t *testing.T) {
	sink := &hDLQSink{}
	in := hTrackingDLQRecord(t, &analyticsv1.ConsentState{Version: 1, Analytics: true})
	want := append([]byte(nil), in.Value...)

	require.NoError(t, hRedactor(t, sink).PublishRecord(context.Background(), in))

	require.Len(t, sink.got, 1, "scenario: A dead-letter copy keeps the client id with consent")
	assert.Equal(t, want, sink.got[0].Value)
	assert.Equal(t, in.Key, sink.got[0].Key)
}

func TestRedactingDLQTreatsMissingOrVersionZeroConsentAsDenied(t *testing.T) {
	cases := map[string]*analyticsv1.ConsentState{
		"absent consent":           nil,
		"analytics with version 0": {Version: 0, Analytics: true},
		"negative version":         {Version: -1, Analytics: true},
		"unknown version":          {Version: 2, Analytics: true},
	}
	for name, consent := range cases {
		t.Run(name, func(t *testing.T) {
			sink := &hDLQSink{}
			in := hTrackingDLQRecord(t, consent)

			require.NoError(t, hRedactor(t, sink).PublishRecord(context.Background(), in))

			require.Len(t, sink.got, 1, "scenario: Absent or version-0 consent is treated as denied")
			_, te := hDecodeTracking(t, sink.got[0].Value)
			assert.Empty(t, te.GetGaClientId())
			assert.Equal(t, "lst-9", te.GetListingId())
		})
	}
}

func TestRedactingDLQPassesOtherRecordsUnchanged(t *testing.T) {
	boom := errors.New("broker unavailable")
	sink := &hDLQSink{err: boom}
	r := hRedactor(t, sink)

	undecodable := &kgo.Record{Topic: "t.dlq", Key: []byte("k1"), Value: []byte{0xff, 0xff, 0xff, 0xff}}
	order := hOrder(t, "o1", 1)
	orderRec := &kgo.Record{Topic: "t.dlq", Key: []byte("k2"), Value: order.Value}

	errA := r.PublishRecord(context.Background(), undecodable)
	errB := r.PublishRecord(context.Background(), orderRec)

	require.Len(t, sink.got, 2, "scenario: An undecodable or non-tracking record is published unchanged")
	assert.Equal(t, []byte{0xff, 0xff, 0xff, 0xff}, sink.got[0].Value)
	assert.Equal(t, order.Value, sink.got[1].Value)
	assert.Equal(t, boom, errA)
	assert.Equal(t, boom, errB)
}
