package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	identityv1 "github.com/buidangphuc/team-analytics/generated/platform/identity/v1"
	"github.com/buidangphuc/team-analytics/internal/app"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

const (
	erasedType        = "platform.identity.v1.UserErased"
	statusChangedType = "platform.identity.v1.UserStatusChanged"
	erasureWait       = 60 * time.Second
)

var erasureEnv = map[string]string{
	"IDENTITY_EVENTS_TOPIC":    "identity.events",
	"ERASURE_CONSUMER_GROUP":   "team-analytics.user-erasure",
	"ERASURE_DLQ_TOPIC":        "identity.events.analytics.dlq",
	"ERASURE_TOMBSTONE_SECRET": "app-test-secret",
}

func erasureWarehouse(t *testing.T) *duckdb.Writer {
	t.Helper()
	w, err := duckdb.Open(context.Background(), "", duckdb.WithTombstoneSecret("app-test-secret"))
	require.NoError(t, err)
	app.SetHooks(t, app.Hooks{
		OpenWarehouse: func(context.Context, config.Settings) (warehouse.WarehouseWriter, error) { return w, nil },
	})
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func seedErasureUser(t *testing.T, w *duckdb.Writer, user string) {
	t.Helper()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	ctx := context.Background()
	require.NoError(t, w.Write(ctx, []*warehouse.TrackingRecord{
		{EventID: "t-" + user, EventType: "view", ListingID: "l1", SessionID: "s-" + user, AnonymousID: "anon-" + user, OccurredAt: at, PrincipalID: user, PrincipalType: "user"},
	}))
	require.NoError(t, w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: "o-" + user, OrderID: "ord", ListingID: "l1", SellerID: "s", Quantity: 1, UnitPrice: 5, Currency: "VND", OccurredAt: at, Status: "PAID", BuyerID: user},
	}))
	require.NoError(t, w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{
		{EventID: "e-" + user, Fact: warehouse.FactFavoriteAdded, UserID: user, ListingID: "l1", OccurredAt: at},
	}))
}

func mentions(t *testing.T, w *duckdb.Writer, needle string) int {
	t.Helper()
	total := 0
	for _, tbl := range []string{"tracking_events", "order_facts", "engagement_facts"} {
		var n int
		require.NoError(t, w.DB().QueryRow(`SELECT count(*) FROM `+tbl+` t WHERE to_json(t)::VARCHAR LIKE '%' || ? || '%'`, needle).Scan(&n))
		total += n
	}
	return total
}

func tombstones(t *testing.T, w *duckdb.Writer) int {
	t.Helper()
	var n int
	if err := w.DB().QueryRow(`SELECT count(*) FROM erased_users`).Scan(&n); err != nil {
		return -1
	}
	return n
}

// Scenario: AUE-1 A UserErased event purges the user; offsets advance only after the purge
func TestErasureConsumerPurgesUserAndCommitsAfterPurge(t *testing.T) {
	brokers := testkit.Redpanda(t)
	w := erasureWarehouse(t)
	seedErasureUser(t, w, "user-erase-app-1")
	seedErasureUser(t, w, "user-keep-app-1")
	s := baseSettings(t, brokers, erasureEnv)

	produce(t, brokers, "identity.events", &kgo.Record{Key: []byte("user-erase-app-1"), Value: envelope(t, "evt-1", erasedType, "identity",
		marshal(t, &identityv1.UserErased{UserId: "user-erase-app-1", ErasedAt: timestamppb.Now()}))})
	p := startProc(t, app.RunConsumer, s)

	testkit.Eventually(t, erasureWait, func() bool { return tombstones(t, w) == 1 })
	assert.Zero(t, mentions(t, w, "user-erase-app-1"))
	assert.NotZero(t, mentions(t, w, "user-keep-app-1"))
	p.stopAndWait(t)
	assert.GreaterOrEqual(t, committedOffset(t, brokers, "team-analytics.user-erasure", "identity.events"), int64(1))
}

// Scenario: AUE-2 and AUE-3 Other types are skipped, poison records are dead-lettered with an error header, and the group moves on
func TestErasureConsumerSkipsOtherTypesAndDeadLettersPoison(t *testing.T) {
	brokers := testkit.Redpanda(t)
	w := erasureWarehouse(t)
	seedErasureUser(t, w, "user-keep-app-2")
	s := baseSettings(t, brokers, erasureEnv)

	produce(t, brokers, "identity.events",
		&kgo.Record{Key: []byte("user-keep-app-2"), Value: envelope(t, "evt-status", statusChangedType, "identity",
			marshal(t, &identityv1.UserStatusChanged{UserId: "user-keep-app-2", ChangedAt: timestamppb.Now()}))},
		&kgo.Record{Key: []byte("bad-key"), Value: []byte("\xff\xff\xff\xff")},
		&kgo.Record{Key: []byte("empty-key"), Value: envelope(t, "evt-empty", erasedType, "identity", marshal(t, &identityv1.UserErased{}))},
	)
	p := startProc(t, app.RunConsumer, s)

	got := testkit.ReadAll(t, brokers, "identity.events.analytics.dlq", 2, erasureWait)
	assert.Equal(t, "bad-key", string(got[0].Key))
	assert.Equal(t, "empty-key", string(got[1].Key))
	for _, r := range got {
		var hasErr bool
		for _, h := range r.Headers {
			if h.Key == "error" || h.Key == "dlq-error" {
				hasErr = true
			}
		}
		assert.True(t, hasErr, "DLQ record %s has no error header", r.Key)
	}
	p.stopAndWait(t)
	assert.GreaterOrEqual(t, committedOffset(t, brokers, "team-analytics.user-erasure", "identity.events"), int64(3))
	assert.NotZero(t, mentions(t, w, "user-keep-app-2"))
	assert.Equal(t, 0, tombstones(t, w))
}
