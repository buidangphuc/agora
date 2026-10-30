package events_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	engagementv1 "github.com/buidangphuc/team-analytics/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-analytics/generated/platform/listing/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
	"github.com/buidangphuc/team-analytics/internal/warehouse/fake"
)

const (
	hTrackingType   = "platform.analytics.v1.TrackingEvent"
	hOrderPaidType  = "platform.order.v1.OrderPaidEvent"
	hListingType    = "platform.listing.v1.ListingChanged"
	hFavoriteAdded  = "platform.engagement.v1.FavoriteAdded"
	hCorruptPayload = "\xff\xff"
)

var hAt = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

type hCounterWriter struct {
	*fake.Writer
	failures int64
	err      error
}

func (w *hCounterWriter) RecordDecodeFailures(_ context.Context, _ time.Time, n int64) error {
	if w.err != nil {
		return w.err
	}
	w.failures += n
	return nil
}

type hListingWriter struct {
	rows []*warehouse.ListingSellerRecord
	err  error
}

func (w *hListingWriter) UpsertListingSellers(_ context.Context, b []*warehouse.ListingSellerRecord) error {
	if w.err != nil {
		return w.err
	}
	w.rows = append(w.rows, b...)
	return nil
}

type hEngagementWriter struct {
	rows []*warehouse.EngagementFactRecord
	err  error
}

func (w *hEngagementWriter) WriteEngagementFacts(_ context.Context, b []*warehouse.EngagementFactRecord) error {
	if w.err != nil {
		return w.err
	}
	w.rows = append(w.rows, b...)
	return nil
}

func hMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	require.NoError(t, err)
	return b
}

func hEnvelope(t *testing.T, id, typ string, payload []byte) []byte {
	t.Helper()
	return hMarshal(t, &eventsv1.EventEnvelope{EventId: id, Type: typ, Payload: payload, OccurredAt: timestamppb.New(hAt)})
}

func hTracking(t *testing.T, id, key string) consume.Record {
	t.Helper()
	p := hMarshal(t, &analyticsv1.TrackingEvent{EventType: analyticsv1.EventType_EVENT_TYPE_VIEW, ListingId: "lst-" + id, SessionId: "s-" + id})
	return consume.Record{Topic: "analytics.events", Key: []byte(key), Value: hEnvelope(t, id, hTrackingType, p)}
}

func hCorruptTracking(t *testing.T, key string) consume.Record {
	t.Helper()
	return consume.Record{Topic: "analytics.events", Key: []byte(key), Value: hEnvelope(t, "bad", hTrackingType, []byte(hCorruptPayload))}
}

func hOrder(t *testing.T, id string, items int) consume.Record {
	t.Helper()
	ope := &orderv1.OrderPaidEvent{OrderId: "order-" + id, BuyerId: "buyer-1", Currency: "VND", PaidAt: timestamppb.New(hAt)}
	for i := 0; i < items; i++ {
		ope.Items = append(ope.Items, &orderv1.OrderLineItemFact{ListingId: "lst-" + id, SellerId: "sel-1", Quantity: 1, UnitPrice: 1000, Currency: "VND"})
	}
	return consume.Record{Topic: "order.events", Key: []byte("order-" + id), Value: hEnvelope(t, "evt-"+id, hOrderPaidType, hMarshal(t, ope))}
}

func hListing(t *testing.T, id string) consume.Record {
	t.Helper()
	lc := &listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: id, SellerId: "seller-" + id, CategoryId: "cat-books", Price: 125000},
		ChangeType: listingv1.ChangeType_CHANGE_TYPE_CREATED,
	}
	return consume.Record{Topic: "listing.events", Key: []byte(id), Value: hEnvelope(t, "evt-"+id, hListingType, hMarshal(t, lc))}
}

func hFavorite(t *testing.T, key string) consume.Record {
	t.Helper()
	p := hMarshal(t, &engagementv1.FavoriteAdded{UserId: "user-1", ListingId: "lst-1"})
	return consume.Record{Topic: "engagement.events", Key: []byte(key), Value: hEnvelope(t, "evt-fav", hFavoriteAdded, p)}
}

func hLogging(t *testing.T) (*bytes.Buffer, *kafkax.Fingerprinter) {
	t.Helper()
	fp, err := kafkax.NewFingerprinter("s1")
	require.NoError(t, err)
	return &bytes.Buffer{}, fp
}

func hSettings(t *testing.T, env map[string]string) config.Settings {
	t.Helper()
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50059")
	for k, v := range env {
		t.Setenv(k, v)
	}
	s, err := config.Load()
	require.NoError(t, err)
	return s
}

func hRuntime() sdkconfig.Runtime {
	return sdkconfig.Runtime{Env: "test", LogLevel: "debug", LogJSON: true}
}

func TestBatchHandlerWritesTrackingAndOrderFactsThenAcknowledges(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hCounterWriter{Writer: fake.New()}
	h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	batch := []consume.Record{
		hTracking(t, "t1", "k1"), hOrder(t, "o1", 1), hTracking(t, "t2", "k2"), hOrder(t, "o2", 2), hTracking(t, "t3", "k3"),
	}
	require.NoError(t, h(context.Background(), batch))

	require.Len(t, w.Rows(), 3, "scenario: A decoded batch is written and acknowledged")
	assert.Equal(t, []string{"t1", "t2", "t3"}, []string{w.Rows()[0].EventID, w.Rows()[1].EventID, w.Rows()[2].EventID})
	require.Len(t, w.OrderFactRows(), 3)
	assert.Zero(t, w.failures)
}

func TestWriteFailureIsReturnedForRetry(t *testing.T) {
	buf, fp := hLogging(t)
	boom := errors.New("warehouse down")
	w := &hCounterWriter{Writer: fake.New()}
	w.FailWith = boom
	h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	err := h(context.Background(), []consume.Record{hTracking(t, "t1", "k1"), hTracking(t, "t2", "k2")})

	require.ErrorIs(t, err, boom, "scenario: A write failure is returned for retry, not skipped")
	assert.NotErrorIs(t, err, events.ErrDeadLetter)
	assert.Empty(t, w.Rows())
}

func TestOrderWriteFailureIsReturnedForRetry(t *testing.T) {
	buf, fp := hLogging(t)
	boom := errors.New("warehouse down")
	w := &hCounterWriter{Writer: fake.New()}
	w.FailWith = boom
	h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	err := h(context.Background(), []consume.Record{hOrder(t, "o1", 1)})

	require.ErrorIs(t, err, boom, "scenario: A write failure is returned for retry, not skipped")
	assert.NotErrorIs(t, err, events.ErrDeadLetter)
}

func TestNonTrackingEnvelopeIsSkippedByBatchHandler(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hCounterWriter{Writer: fake.New()}
	h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	other := consume.Record{Topic: "analytics.events", Key: []byte("k"), Value: hEnvelope(t, "x", "platform.other.v1.Thing", nil)}
	require.NoError(t, h(context.Background(), []consume.Record{other}), "scenario: A non-tracking envelope is skipped without a row")

	assert.Empty(t, w.Rows())
	assert.Empty(t, w.OrderFactRows())
	assert.Zero(t, w.failures, "a skipped envelope is not a decode failure")
}

func TestUndecodableRecordIsCountedAndTheRestIsWritten(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hCounterWriter{Writer: fake.New()}
	h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	batch := []consume.Record{hTracking(t, "t1", "k1"), hCorruptTracking(t, "kb"), hTracking(t, "t2", "k2")}
	require.NoError(t, h(context.Background(), batch))

	assert.Len(t, w.Rows(), 2, "scenario: An undecodable tracking payload is counted and the rest of the batch is written")
	assert.EqualValues(t, 1, w.failures)
}

func TestDecodeCounterFailureDoesNotFailTheBatch(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hCounterWriter{Writer: fake.New(), err: errors.New("counter table locked")}
	h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	batch := []consume.Record{hTracking(t, "t1", "k1"), hCorruptTracking(t, "key-bad"), hTracking(t, "t2", "k2")}
	require.NoError(t, h(context.Background(), batch), "scenario: A failing decode counter does not block the batch")

	assert.Len(t, w.Rows(), 2)
	assert.Contains(t, buf.String(), "counter table locked")
	assert.Contains(t, buf.String(), fp.Of([]byte("key-bad")))
	assert.NotContains(t, buf.String(), "key-bad")
}

func TestEmptyKeyRecordIsHandled(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hCounterWriter{Writer: fake.New()}
	h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	rec := hCorruptTracking(t, "")
	rec.Key = nil
	require.NoError(t, h(context.Background(), []consume.Record{rec}), "scenario: A record with an empty key does not fail the batch")

	assert.Contains(t, buf.String(), fp.Of(nil))
	assert.Contains(t, buf.String(), "key_fp")
}

func TestNoConsumerLogContainsRawKey(t *testing.T) {
	const key = "user-123"

	t.Run("batch handler", func(t *testing.T) {
		buf, fp := hLogging(t)
		w := &hCounterWriter{Writer: fake.New()}
		h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))
		require.NoError(t, h(context.Background(), []consume.Record{hCorruptTracking(t, key)}))
		assert.Contains(t, buf.String(), `"key_fp":"`+fp.Of([]byte(key))+`"`, "scenario: A decode failure logs the fingerprint and not the key")
		assert.NotContains(t, buf.String(), key)
	})

	t.Run("listing handler", func(t *testing.T) {
		buf, fp := hLogging(t)
		h := events.NewListingHandler(&hListingWriter{}, fp, logx.NewTo(buf, hRuntime(), "test"))
		rec := consume.Record{Topic: "listing.events", Key: []byte(key), Value: []byte{0xff, 0xff, 0xff}}
		require.NoError(t, h(context.Background(), []consume.Record{rec}))
		assert.Contains(t, buf.String(), `"key_fp":"`+fp.Of([]byte(key))+`"`)
		assert.NotContains(t, buf.String(), key)
	})

	t.Run("engagement handler", func(t *testing.T) {
		buf, fp := hLogging(t)
		h := events.NewEngagementHandler(&hEngagementWriter{}, fp, logx.NewTo(buf, hRuntime(), "test"))
		rec := consume.Record{Topic: "engagement.events", Key: []byte(key), Value: []byte{0xff, 0xff, 0xff}}
		err := h(context.Background(), rec)
		require.ErrorIs(t, err, events.ErrDeadLetter)
		assert.NotContains(t, err.Error(), key)
		assert.NotContains(t, buf.String(), key)
	})
}

func TestMainConsumerOptionsFromSettings(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		s := hSettings(t, map[string]string{"BATCH_MAX_SIZE": "500", "BATCH_FLUSH_INTERVAL_SECONDS": "2"})
		spec := events.MainOptions(s)
		assert.Equal(t, 500, spec.Options.MaxPollRecords, "scenario: The batch bounds come from the settings")
		assert.Equal(t, 2*time.Second, spec.FetchMaxWait)
		assert.Equal(t, "analytics.events.dlq", spec.Options.DLQTopic)
		assert.Equal(t, []string{"analytics.events", "order.events"}, spec.Topics)
		assert.Equal(t, "team-analytics", spec.Group)
	})

	t.Run("overridden", func(t *testing.T) {
		s := hSettings(t, map[string]string{
			"BATCH_MAX_SIZE": "50", "BATCH_FLUSH_INTERVAL_SECONDS": "7", "KAFKA_DLQ_TOPIC": "custom.dlq",
			"KAFKA_ANALYTICS_TOPIC": "a.t", "KAFKA_ORDER_TOPIC": "o.t", "KAFKA_CONSUMER_GROUP": "grp",
		})
		spec := events.MainOptions(s)
		assert.Equal(t, 50, spec.Options.MaxPollRecords)
		assert.Equal(t, 7*time.Second, spec.FetchMaxWait)
		assert.Equal(t, "custom.dlq", spec.Options.DLQTopic)
		assert.Equal(t, []string{"a.t", "o.t"}, spec.Topics)
		assert.Equal(t, "grp", spec.Group)
	})
}

func TestListingConsumerSpecFromSettings(t *testing.T) {
	spec := events.ListingSpec(hSettings(t, nil))

	assert.Equal(t, "team-analytics-listing-attrs", spec.Group, "scenario: The listing consumer starts at the earliest offset in its own group")
	assert.Equal(t, []string{"listing.events"}, spec.Topics)
	assert.True(t, spec.StartAtEarliest)
	assert.Equal(t, "listing.events.analytics.dlq", spec.Options.DLQTopic)
}

func TestListingBatchSkipsUndecodableRecord(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hListingWriter{}
	h := events.NewListingHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	corrupt := consume.Record{Topic: "listing.events", Key: []byte("bad"), Value: []byte{0xff, 0xff, 0xff, 0xff}}
	batch := []consume.Record{hListing(t, "lst-1"), corrupt, hListing(t, "lst-2")}
	require.NoError(t, h(context.Background(), batch), "scenario: An undecodable listing record is skipped and the batch is upserted")

	require.Len(t, w.rows, 2)
	assert.Equal(t, "lst-1", w.rows[0].ListingID)
	assert.Equal(t, "lst-2", w.rows[1].ListingID)
}

func TestListingWriteFailureIsReturnedForRetry(t *testing.T) {
	buf, fp := hLogging(t)
	boom := errors.New("upsert failed")
	h := events.NewListingHandler(&hListingWriter{err: boom}, fp, logx.NewTo(buf, hRuntime(), "test"))

	err := h(context.Background(), []consume.Record{hListing(t, "lst-1")})

	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, events.ErrDeadLetter)
}

func TestEngagementOptionsFitRebalanceBudget(t *testing.T) {
	spec := events.EngagementOptions(hSettings(t, nil))

	assert.Equal(t, 10, spec.Options.MaxPollRecords, "scenario: Engagement polls fit the rebalance timeout")
	assert.Equal(t, "engagement.events.analytics.dlq", spec.Options.DLQTopic)
	assert.Equal(t, "team-analytics.engagement", spec.Group)
	assert.Equal(t, []string{"engagement.events"}, spec.Topics)

	waits := 100*time.Millisecond + 200*time.Millisecond + 400*time.Millisecond + 800*time.Millisecond
	perPoll := waits + 5*50*time.Millisecond
	total := time.Duration(spec.Options.MaxPollRecords) * perPoll
	assert.Equal(t, 17500*time.Millisecond, total)
	assert.Less(t, total, 30*time.Second)
}

func TestMalformedEngagementRecordIsPermanent(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hEngagementWriter{}
	h := events.NewEngagementHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	err := h(context.Background(), consume.Record{Topic: "engagement.events", Key: []byte("k"), Value: []byte{0xff, 0xff, 0xff, 0xff}})

	require.Error(t, err, "scenario: A malformed engagement record is permanent")
	assert.ErrorIs(t, err, events.ErrDeadLetter)
	assert.Empty(t, w.rows)
}

func TestUnknownEngagementTypeIsPermanent(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hEngagementWriter{}
	h := events.NewEngagementHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	rec := consume.Record{Topic: "engagement.events", Key: []byte("k"), Value: hEnvelope(t, "x", "platform.engagement.v1.Mystery", nil)}
	err := h(context.Background(), rec)

	require.Error(t, err, "scenario: An unknown engagement type is permanent")
	assert.ErrorIs(t, err, events.ErrUnknownEngagementType)
	assert.ErrorIs(t, err, events.ErrDeadLetter)
	assert.Empty(t, w.rows)
}

func TestValidEngagementRecordIsWritten(t *testing.T) {
	buf, fp := hLogging(t)
	w := &hEngagementWriter{}
	h := events.NewEngagementHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	require.NoError(t, h(context.Background(), hFavorite(t, "k")))

	require.Len(t, w.rows, 1, "scenario: A valid engagement record reaches engagement_facts")
	assert.Equal(t, warehouse.FactFavoriteAdded, w.rows[0].Fact)
	assert.Equal(t, "user-1", w.rows[0].UserID)
	assert.Equal(t, "lst-1", w.rows[0].ListingID)
}

func TestEngagementWriteFailureIsTransient(t *testing.T) {
	buf, fp := hLogging(t)
	boom := errors.New("duckdb busy")
	h := events.NewEngagementHandler(&hEngagementWriter{err: boom}, fp, logx.NewTo(buf, hRuntime(), "test"))

	err := h(context.Background(), hFavorite(t, "k"))

	require.ErrorIs(t, err, boom, "scenario: A failing engagement write is returned for retry")
	assert.NotErrorIs(t, err, events.ErrDeadLetter)
}

func TestMaxSizeBatchWriteFitsRebalanceBudget(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, filepath.Join(t.TempDir(), "analytics.duckdb"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	buf, fp := hLogging(t)
	h := events.NewBatchHandler(w, fp, logx.NewTo(buf, hRuntime(), "test"))

	batch := make([]consume.Record, 0, 500)
	for i := 0; i < 500; i++ {
		id := fmt.Sprintf("evt-%d", i)
		batch = append(batch, hTracking(t, id, "k"+id))
	}

	start := time.Now()
	require.NoError(t, h(ctx, batch))
	elapsed := time.Since(start)

	var n int
	require.NoError(t, w.DB().QueryRow("SELECT COUNT(*) FROM "+warehouse.TableName).Scan(&n))
	assert.Equal(t, 500, n)
	assert.Less(t, elapsed, 5*time.Second, "scenario: A maximum-size batch is handled well inside the rebalance timeout")
	assert.Less(t, 5*elapsed+1500*time.Millisecond, 27*time.Second)
}
