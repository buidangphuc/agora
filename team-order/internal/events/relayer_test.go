package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

func relayerSettings(poll time.Duration) config.Settings {
	return config.Settings{
		Events: config.Events{OrderTopic: "order.events"},
		Outbox: sdkconfig.Outbox{PollInterval: poll, BatchSize: 100, ClaimLock: time.Minute},
	}
}

func enqueueRow(t *testing.T, r *rig, row outbox.Row) {
	t.Helper()
	tx, err := r.pool.Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, r.ob.EnqueueTx(context.Background(), tx, row))
	require.NoError(t, tx.Commit(context.Background()))
}

// Scenario: The relayer publishes keyed by order id
func TestRelayerPublishesKeyedByOrderID(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(sampleOrder())
	r.pay(o.ID)
	rec := testkit.NewRecorder()
	relayer, err := events.NewRelayer(r.ob, rec, relayerSettings(time.Second), quietLogger())
	require.NoError(t, err)

	report, err := relayer.RelayOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Published)

	got := rec.Records()
	require.Len(t, got, 1)
	assert.Equal(t, "order.events", got[0].Topic)
	assert.Equal(t, o.ID, string(got[0].Key))
	var stored []byte
	require.NoError(t, r.pool.QueryRow(ctx, `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1`, o.ID).Scan(&stored))
	assert.Equal(t, stored, got[0].Value, "the stored envelope is published verbatim")
	var status string
	require.NoError(t, r.pool.QueryRow(ctx, `SELECT status FROM order_outbox_events WHERE aggregate_id = $1`, o.ID).Scan(&status))
	assert.Equal(t, "published", status)

	report, err = relayer.RelayOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, report.Published, "a published row is not sent twice")
	assert.Len(t, rec.Records(), 1)
}

// Scenario: A pending row is claimed within one poll interval
func TestRelayerPublishesWithinPollInterval(t *testing.T) {
	r := newRig(t)
	rec := testkit.NewRecorder()
	relayer, err := events.NewRelayer(r.ob, rec, relayerSettings(time.Second), quietLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = relayer.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	o := r.createOrder(sampleOrder())
	r.pay(o.ID)

	testkit.Eventually(t, 2*time.Second, func() bool { return len(rec.Records()) == 1 })
	assert.Equal(t, o.ID, string(rec.Records()[0].Key))
}

// Scenario: Events of one order are published in seq order
func TestRelayerKeepsPerOrderSequence(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := r.createOrder(sampleOrder())

	placed, err := events.BuildPlacedOutboxRow(o)
	require.NoError(t, err)
	paid, err := events.BuildPaidOutboxRow(o)
	require.NoError(t, err)
	shipped, err := events.BuildShippedOutboxRow(store.Shipment{ID: "shp-1", OrderID: o.ID, Carrier: "GHN", TrackingCode: "T-1", BuyerID: o.BuyerID, SellerID: o.SellerID})
	require.NoError(t, err)
	for _, row := range []outbox.Row{placed, paid, shipped} {
		enqueueRow(t, r, row)
	}

	rec := testkit.NewRecorder()
	relayer, err := events.NewRelayer(r.ob, rec, relayerSettings(time.Second), quietLogger())
	require.NoError(t, err)
	for i := 0; i < 20 && len(rec.Records()) < 3; i++ {
		_, err := relayer.RelayOnce(ctx)
		require.NoError(t, err)
	}

	var types []string
	for _, p := range rec.Records() {
		assert.Equal(t, o.ID, string(p.Key))
		var env eventsv1.EventEnvelope
		require.NoError(t, proto.Unmarshal(p.Value, &env))
		types = append(types, env.GetType())
	}
	assert.Equal(t, []string{events.OrderPlacedEventType, events.OrderPaidEventType, events.OrderShippedEventType}, types)
}

// Scenario: Published rows older than 7 days are purged
func TestOutboxPurgeDeletesOnlyOldPublishedRows(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	for _, id := range []string{"old-published", "recent-published", "old-pending"} {
		enqueueRow(t, r, outbox.Row{EventID: id, AggregateType: "Order", AggregateID: id, EventType: "t", Payload: []byte(id)})
	}
	_, err := r.pool.Exec(ctx, `UPDATE order_outbox_events SET status = 'published', published_at = now() - interval '8 days' WHERE event_id = 'old-published'`)
	require.NoError(t, err)
	_, err = r.pool.Exec(ctx, `UPDATE order_outbox_events SET status = 'published', published_at = now() - interval '6 days' WHERE event_id = 'recent-published'`)
	require.NoError(t, err)
	_, err = r.pool.Exec(ctx, `UPDATE order_outbox_events SET created_at = now() - interval '8 days' WHERE event_id = 'old-pending'`)
	require.NoError(t, err)

	purger, err := events.NewOutboxPurger(r.ob, quietLogger())
	require.NoError(t, err)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = purger.Run(runCtx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	testkit.Eventually(t, 10*time.Second, func() bool {
		return r.count(`SELECT count(*) FROM order_outbox_events WHERE event_id = 'old-published'`) == 0
	})
	assert.Equal(t, 1, r.count(`SELECT count(*) FROM order_outbox_events WHERE event_id = 'recent-published'`), "a 6-day-old published row stays")
	assert.Equal(t, 1, r.count(`SELECT count(*) FROM order_outbox_events WHERE event_id = 'old-pending'`), "a pending row is never purged")
}
