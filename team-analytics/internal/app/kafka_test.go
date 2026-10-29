package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	engagementv1 "github.com/buidangphuc/team-analytics/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-analytics/generated/platform/listing/v1"
	"github.com/buidangphuc/team-analytics/internal/app"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

const dlqWait = 30 * time.Second

func useWriter(t *testing.T, w *recWriter) {
	t.Helper()
	app.SetHooks(t, app.Hooks{
		OpenWarehouse: func(context.Context, config.Settings) (warehouse.WarehouseWriter, error) { return w, nil },
	})
}

func TestPoisonBatchIsDeadLetteredAfterFiveAttempts(t *testing.T) {
	brokers := testkit.Redpanda(t)
	w := &recWriter{}
	w.fail.Store(true)
	useWriter(t, w)
	s := baseSettings(t, brokers, nil)

	produce(t, brokers, s.Kafka.AnalyticsTopic,
		&kgo.Record{Key: []byte("k1"), Value: trackingValue(t, "e1", "u1", "", false)},
		&kgo.Record{Key: []byte("k2"), Value: trackingValue(t, "e2", "u2", "", false)},
	)
	p := startProc(t, app.RunConsumer, s)

	got := testkit.ReadAll(t, brokers, s.Kafka.DLQTopic, 2, dlqWait)
	assert.Equal(t, "k1", string(got[0].Key))
	assert.Equal(t, "k2", string(got[1].Key))
	assert.Equal(t, int32(5), w.trackCalls.Load())

	p.stopAndWait(t)
	assert.GreaterOrEqual(t, committedOffset(t, brokers, s.Kafka.ConsumerGroup, s.Kafka.AnalyticsTopic), int64(2))
}

func TestPoisonBatchDLQCarriesNoGAClientIDWithoutConsent(t *testing.T) {
	brokers := testkit.Redpanda(t)
	w := &recWriter{}
	w.fail.Store(true)
	useWriter(t, w)
	s := baseSettings(t, brokers, nil)

	produce(t, brokers, s.Kafka.AnalyticsTopic,
		&kgo.Record{Key: []byte("k1"), Value: trackingValue(t, "e1", "u1", "123.456", false)},
	)
	startProc(t, app.RunConsumer, s)

	got := testkit.ReadAll(t, brokers, s.Kafka.DLQTopic, 1, dlqWait)
	assert.Equal(t, "k1", string(got[0].Key))
	assert.NotContains(t, string(got[0].Value), "123.456")
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(got[0].Value, &env))
	var te analyticsv1.TrackingEvent
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &te))
	assert.Empty(t, te.GetGaClientId())
	assert.Equal(t, "lst-e1", te.GetListingId())
}

func TestPoisonListingBatchIsDeadLettered(t *testing.T) {
	brokers := testkit.Redpanda(t)
	w := &recWriter{}
	w.fail.Store(true)
	useWriter(t, w)
	s := baseSettings(t, brokers, nil)

	listing := func(id string) *kgo.Record {
		lc := &listingv1.ListingChanged{
			Listing:    &listingv1.Listing{Id: id, SellerId: "seller-" + id, CategoryId: "cat-books", Price: 125000},
			ChangeType: listingv1.ChangeType_CHANGE_TYPE_CREATED,
		}
		return &kgo.Record{Key: []byte(id), Value: envelope(t, "evt-"+id, "platform.listing.v1.ListingChanged", "seller-"+id, marshal(t, lc))}
	}
	produce(t, brokers, s.Kafka.ListingTopic, listing("l1"), listing("l2"))
	p := startProc(t, app.RunConsumer, s)

	got := testkit.ReadAll(t, brokers, s.Kafka.ListingDLQTopic, 2, dlqWait)
	assert.Equal(t, "l1", string(got[0].Key))
	assert.Equal(t, "l2", string(got[1].Key))
	assert.Equal(t, int32(5), w.upsertCalls.Load())

	p.stopAndWait(t)
	assert.GreaterOrEqual(t, committedOffset(t, brokers, s.Kafka.ListingConsumerGroup, s.Kafka.ListingTopic), int64(2))
}

func TestMalformedEngagementRecordIsDeadLetteredAndGroupMovesOn(t *testing.T) {
	brokers := testkit.Redpanda(t)
	w := &recWriter{}
	useWriter(t, w)
	s := baseSettings(t, brokers, nil)

	fav := marshal(t, &engagementv1.FavoriteAdded{UserId: "user-1", ListingId: "lst-1"})
	produce(t, brokers, s.Engagement.EventsTopic,
		&kgo.Record{Key: []byte("bad-key"), Value: []byte("\xff\xff\xff\xff")},
		&kgo.Record{Key: []byte("good-key"), Value: envelope(t, "evt-fav", favoriteAddedType, "user-1", fav)},
	)
	startProc(t, app.RunConsumer, s)

	got := testkit.ReadAll(t, brokers, s.Engagement.DLQTopic, 1, dlqWait)
	assert.Equal(t, "bad-key", string(got[0].Key))
	src, ok := headerValue(got[0], "dlq-source-topic")
	assert.True(t, ok)
	assert.Equal(t, s.Engagement.EventsTopic, src)
	_, ok = headerValue(got[0], "dlq-error")
	assert.True(t, ok)
	testkit.Eventually(t, dlqWait, func() bool { return w.engagementCount() == 1 })
}

const favoriteAddedType = "platform.engagement.v1.FavoriteAdded"

func TestBurstOf1000EventsIsWrittenAtOrAbove100PerSecond(t *testing.T) {
	brokers := testkit.Redpanda(t)
	w := &recWriter{}
	useWriter(t, w)
	s := baseSettings(t, brokers, nil)

	recs := make([]*kgo.Record, 0, 1000)
	for i := range 1000 {
		id := fmt.Sprintf("e%04d", i)
		recs = append(recs, &kgo.Record{Key: []byte("u" + id), Value: trackingValue(t, id, "u"+id, "", false)})
	}
	produce(t, brokers, s.Kafka.AnalyticsTopic, recs...)
	startProc(t, app.RunConsumer, s)

	testkit.Eventually(t, 60*time.Second, func() bool { return w.rowCount() == 1000 })
	w.mu.Lock()
	elapsed := w.last.Sub(w.first)
	w.mu.Unlock()
	rate := 1000 / max(elapsed.Seconds(), 0.001)
	assert.GreaterOrEqual(t, rate, 100.0, "1000 events written in %v", elapsed)
}

func TestWorkersStopAfterServerAndBeforeWarehouseClose(t *testing.T) {
	brokers := testkit.Redpanda(t)
	w := &recWriter{gate: make(chan struct{}), started: make(chan struct{})}
	useWriter(t, w)
	s := baseSettings(t, brokers, nil)

	produce(t, brokers, s.Kafka.AnalyticsTopic,
		&kgo.Record{Key: []byte("k1"), Value: trackingValue(t, "e1", "u1", "", false)},
	)
	p := startProc(t, app.RunConsumer, s)
	waitUp(t, p)
	select {
	case <-w.started:
	case <-time.After(dlqWait):
		t.Fatal("batch consumer never fetched the record")
	}

	p.cancel()
	testkit.Eventually(t, 10*time.Second, func() bool {
		return dialErr(s.Addr()) != nil
	})
	assert.Empty(t, w.eventLog(), "warehouse closed or written before the held batch was released")
	select {
	case <-p.finished:
		t.Fatal("root returned while the batch consumer still held a fetched batch")
	default:
	}

	close(w.gate)
	require.NoError(t, p.result(t, 30*time.Second))
	assert.Equal(t, []string{"write", "close"}, w.eventLog())
}
