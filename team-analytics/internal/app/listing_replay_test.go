package app_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	commonv1 "github.com/buidangphuc/team-analytics/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-analytics/generated/platform/listing/v1"
	"github.com/buidangphuc/team-analytics/internal/app"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

const replayWithin = 90 * time.Second

var (
	replayT1 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	replayT2 = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
)

type replayRow struct {
	Seller    string
	Status    sql.NullString
	UpdatedAt time.Time
}

func listingEvent(t *testing.T, id string, status listingv1.ListingStatus, at time.Time) *kgo.Record {
	t.Helper()
	lc := &listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: id, SellerId: "seller-" + id, CategoryId: "cat-books", Price: 125000, Status: status},
		ChangeType: listingv1.ChangeType_CHANGE_TYPE_UPDATED,
	}
	value := marshal(t, &eventsv1.EventEnvelope{
		EventId:    "evt-" + id + "-" + at.Format(time.RFC3339),
		Type:       "platform.listing.v1.ListingChanged",
		Payload:    marshal(t, lc),
		OccurredAt: timestamppb.New(at),
		Principal:  &commonv1.Principal{Id: "seller-" + id, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER},
	})
	return &kgo.Record{Key: []byte(id), Value: value}
}

func seedRows(t *testing.T, path string, recs ...*warehouse.ListingSellerRecord) {
	t.Helper()
	w, err := duckdb.Open(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, w.UpsertListingSellers(context.Background(), recs))
	require.NoError(t, w.Close())
}

func readRows(t *testing.T, path string) map[string]replayRow {
	t.Helper()
	w, err := duckdb.Open(context.Background(), path)
	require.NoError(t, err)
	defer w.Close()
	rows, err := w.DB().QueryContext(context.Background(), "SELECT listing_id, seller_id, status, updated_at FROM listing_sellers")
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]replayRow{}
	for rows.Next() {
		var id string
		var r replayRow
		require.NoError(t, rows.Scan(&id, &r.Seller, &r.Status, &r.UpdatedAt))
		r.UpdatedAt = r.UpdatedAt.UTC()
		out[id] = r
	}
	require.NoError(t, rows.Err())
	return out
}

func runReplay(t *testing.T, s config.Settings) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), replayWithin)
	defer cancel()
	err := app.RunListingReplay(ctx, s)
	if err == nil {
		require.NoError(t, ctx.Err())
	}
	return err
}

func commitOffset(t *testing.T, brokers, group, topic string, offset int64) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumerGroup(group), kgo.ConsumeTopics(topic), kgo.DisableAutoCommit())
	require.NoError(t, err)
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cl.PollFetches(ctx)
	cl.CommitOffsetsSync(ctx, map[string]map[int32]kgo.EpochOffset{
		topic: {0: {Offset: offset, Epoch: -1}},
	}, func(_ *kgo.Client, _ *kmsg.OffsetCommitRequest, _ *kmsg.OffsetCommitResponse, err error) {
		require.NoError(t, err)
	})
}

func TestListingReplay_FillsStatusForAllListings(t *testing.T) {
	brokers := testkit.Redpanda(t)
	s := baseSettings(t, brokers, nil)
	seedRows(t, s.Warehouse.DuckDBPath,
		&warehouse.ListingSellerRecord{ListingID: "L1", SellerID: "seller-L1", UpdatedAt: replayT1},
		&warehouse.ListingSellerRecord{ListingID: "L2", SellerID: "seller-L2", UpdatedAt: replayT1},
		&warehouse.ListingSellerRecord{ListingID: "L3", SellerID: "seller-L3", UpdatedAt: replayT1},
	)
	produce(t, brokers, s.Kafka.ListingTopic,
		listingEvent(t, "L1", listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, replayT1),
		listingEvent(t, "L2", listingv1.ListingStatus_LISTING_STATUS_BANNED, replayT1),
		listingEvent(t, "L3", listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, replayT1),
		listingEvent(t, "L3", listingv1.ListingStatus_LISTING_STATUS_HELD, replayT2),
	)

	require.NoError(t, runReplay(t, s))

	got := readRows(t, s.Warehouse.DuckDBPath)
	require.Len(t, got, 3)
	assert.Equal(t, "LISTING_STATUS_PUBLISHED", got["L1"].Status.String)
	assert.Equal(t, "LISTING_STATUS_BANNED", got["L2"].Status.String)
	assert.Equal(t, "LISTING_STATUS_HELD", got["L3"].Status.String)
	for _, r := range got {
		assert.True(t, r.Status.Valid)
	}
}

func TestListingReplay_UsesOwnGroup(t *testing.T) {
	brokers := testkit.Redpanda(t)
	s := baseSettings(t, brokers, nil)
	require.NotEmpty(t, s.Kafka.ListingReplayGroup)
	require.NotEqual(t, s.Kafka.ListingConsumerGroup, s.Kafka.ListingReplayGroup)

	produce(t, brokers, s.Kafka.ListingTopic,
		listingEvent(t, "L1", listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, replayT1),
		listingEvent(t, "L2", listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, replayT1),
		listingEvent(t, "L3", listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, replayT1),
	)
	commitOffset(t, brokers, s.Kafka.ListingConsumerGroup, s.Kafka.ListingTopic, 1)
	before := committedOffset(t, brokers, s.Kafka.ListingConsumerGroup, s.Kafka.ListingTopic)
	require.Equal(t, int64(1), before)

	require.NoError(t, runReplay(t, s))

	assert.Equal(t, before, committedOffset(t, brokers, s.Kafka.ListingConsumerGroup, s.Kafka.ListingTopic))
	assert.Len(t, readRows(t, s.Warehouse.DuckDBPath), 3)
}

func TestListingReplay_Idempotent(t *testing.T) {
	brokers := testkit.Redpanda(t)
	s := baseSettings(t, brokers, nil)
	produce(t, brokers, s.Kafka.ListingTopic,
		listingEvent(t, "L1", listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, replayT1),
		listingEvent(t, "L2", listingv1.ListingStatus_LISTING_STATUS_BANNED, replayT1),
	)

	require.NoError(t, runReplay(t, s))
	first := readRows(t, s.Warehouse.DuckDBPath)
	require.Len(t, first, 2)

	require.NoError(t, runReplay(t, s))
	assert.Equal(t, first, readRows(t, s.Warehouse.DuckDBPath))
}

func TestListingReplay_DoesNotRegressNewerRow(t *testing.T) {
	brokers := testkit.Redpanda(t)
	s := baseSettings(t, brokers, nil)
	seedRows(t, s.Warehouse.DuckDBPath,
		&warehouse.ListingSellerRecord{ListingID: "L1", SellerID: "seller-L1", Status: "LISTING_STATUS_BANNED", UpdatedAt: replayT2},
	)
	produce(t, brokers, s.Kafka.ListingTopic,
		listingEvent(t, "L1", listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, replayT1),
	)

	require.NoError(t, runReplay(t, s))

	got := readRows(t, s.Warehouse.DuckDBPath)
	require.Len(t, got, 1)
	assert.Equal(t, "LISTING_STATUS_BANNED", got["L1"].Status.String)
	assert.True(t, replayT2.Equal(got["L1"].UpdatedAt))
}

func TestListingReplay_BrokerDownExitsNonZero(t *testing.T) {
	s := baseSettings(t, closedBroker(t), nil)
	seedRows(t, s.Warehouse.DuckDBPath,
		&warehouse.ListingSellerRecord{ListingID: "L1", SellerID: "seller-L1", UpdatedAt: replayT1},
	)
	before := readRows(t, s.Warehouse.DuckDBPath)

	err := runReplay(t, s)

	require.Error(t, err)
	assert.Equal(t, before, readRows(t, s.Warehouse.DuckDBPath))
}

func TestListingReplay_SkipsMalformedRecord(t *testing.T) {
	brokers := testkit.Redpanda(t)
	s := baseSettings(t, brokers, nil)
	produce(t, brokers, s.Kafka.ListingTopic,
		listingEvent(t, "L1", listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, replayT1),
		&kgo.Record{Key: []byte("bad"), Value: []byte{0xff, 0xff, 0xff, 0xff, 0x01}},
		listingEvent(t, "L2", listingv1.ListingStatus_LISTING_STATUS_BANNED, replayT1),
	)

	require.NoError(t, runReplay(t, s))

	got := readRows(t, s.Warehouse.DuckDBPath)
	require.Len(t, got, 2)
	assert.Equal(t, "LISTING_STATUS_PUBLISHED", got["L1"].Status.String)
	assert.Equal(t, "LISTING_STATUS_BANNED", got["L2"].Status.String)
}
