package events_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-analytics/generated/platform/identity/v1"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

var erAt = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func erEnvelope(t *testing.T, id, typ string, payload proto.Message) []byte {
	t.Helper()
	pb, err := proto.Marshal(payload)
	require.NoError(t, err)
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: id, Type: typ, Payload: pb, OccurredAt: timestamppb.New(erAt)})
	require.NoError(t, err)
	return b
}

func erUserErased(t *testing.T, userID string) consume.Record {
	t.Helper()
	return consume.Record{Topic: "identity.events", Key: []byte(userID), Value: erEnvelope(t, "evt-erased-"+userID, events.UserErasedEventType,
		&identityv1.UserErased{UserId: userID, Roles: []string{"buyer"}, ErasedAt: timestamppb.New(erAt)})}
}

func erWriter(t *testing.T) *duckdb.Writer {
	t.Helper()
	w, err := duckdb.Open(context.Background(), "", duckdb.WithTombstoneSecret("test-secret"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func erSeed(t *testing.T, w *duckdb.Writer, user string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, w.Write(ctx, []*warehouse.TrackingRecord{
		{EventID: evID("t"), EventType: "view", ListingID: "l1", SessionID: "s-" + user, AnonymousID: "anon-" + user, OccurredAt: erAt, PrincipalID: user, PrincipalType: "user"},
	}))
	require.NoError(t, w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: evID("o"), OrderID: "ord-1", ListingID: "l1", SellerID: "seller-x", Quantity: 1, UnitPrice: 10, Currency: "VND", OccurredAt: erAt, Status: "PAID", BuyerID: user},
	}))
	require.NoError(t, w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{
		{EventID: evID("e"), Fact: warehouse.FactFavoriteAdded, UserID: user, ListingID: "l1", OccurredAt: erAt},
	}))
}

func erCount(t *testing.T, w *duckdb.Writer, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, w.DB().QueryRow(q, args...).Scan(&n))
	return n
}

func erTotalRows(t *testing.T, w *duckdb.Writer) int {
	t.Helper()
	return erCount(t, w, `SELECT (SELECT count(*) FROM tracking_events) + (SELECT count(*) FROM order_facts) + (SELECT count(*) FROM engagement_facts)`)
}

func erNoTrace(t *testing.T, w *duckdb.Writer, needle string) {
	t.Helper()
	for _, tbl := range []string{"tracking_events", "order_facts", "engagement_facts", "listing_sellers", "erased_users"} {
		n := erCount(t, w, fmt.Sprintf(`SELECT count(*) FROM %s t WHERE to_json(t)::VARCHAR LIKE '%%' || ? || '%%'`, tbl), needle)
		assert.Zero(t, n, "%s still contains %q", tbl, needle)
	}
}

func erHandler(t *testing.T, e warehouse.UserEraser) (consume.Handler, *bytes.Buffer, *kafkax.Fingerprinter) {
	t.Helper()
	buf := &bytes.Buffer{}
	fp, err := kafkax.NewFingerprinter("fp-secret")
	require.NoError(t, err)
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return events.NewErasureHandler(e, fp, logger), buf, fp
}

type flakyEraser struct {
	inner    warehouse.UserEraser
	failures int
	calls    int
}

func (f *flakyEraser) EraseUser(ctx context.Context, id string) (warehouse.EraseCounts, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, errors.New("duckdb: database is read-only")
	}
	return f.inner.EraseUser(ctx, id)
}

// Scenario: AUE-1 A UserErased event purges the user
func TestErasureHandlerPurgesUser(t *testing.T) {
	w := erWriter(t)
	erSeed(t, w, "user-erase-1")
	erSeed(t, w, "user-keep-1")
	h, _, _ := erHandler(t, w)

	require.NoError(t, h(context.Background(), erUserErased(t, "user-erase-1")))

	erNoTrace(t, w, "user-erase-1")
	assert.Equal(t, 1, erCount(t, w, `SELECT count(*) FROM tracking_events WHERE principal_id = 'user-keep-1'`))
	assert.Equal(t, 1, erCount(t, w, `SELECT count(*) FROM erased_users`))
	assert.Equal(t, 5, erTotalRows(t, w), "two users x 3 rows, minus the deleted favourite")
}

// Scenario: AUE-2 Another identity event type is skipped
func TestErasureHandlerSkipsOtherIdentityEventTypes(t *testing.T) {
	w := erWriter(t)
	erSeed(t, w, "user-keep-2")
	h, _, _ := erHandler(t, w)
	rec := consume.Record{Key: []byte("user-keep-2"), Value: erEnvelope(t, "evt-status", "platform.identity.v1.UserStatusChanged",
		&identityv1.UserStatusChanged{UserId: "user-keep-2", ChangedAt: timestamppb.New(erAt)})}
	before := erTotalRows(t, w)

	err := h(context.Background(), rec)

	require.NoError(t, err, "a skipped type is acknowledged, not retried and not dead-lettered")
	assert.Equal(t, before, erTotalRows(t, w))
	assert.Equal(t, 1, erCount(t, w, `SELECT count(*) FROM tracking_events WHERE principal_id = 'user-keep-2'`))
	assert.Zero(t, erCount(t, w, `SELECT count(*) FROM erased_users`))
}

// Scenario: AUE-3 Poison record goes to the DLQ
func TestErasureHandlerDeadLettersPoisonRecords(t *testing.T) {
	w := erWriter(t)
	erSeed(t, w, "user-keep-3")
	h, _, _ := erHandler(t, w)
	before := erTotalRows(t, w)

	for name, rec := range map[string]consume.Record{
		"not an envelope": {Key: []byte("k"), Value: []byte("\xff\xff\xff\xff not protobuf")},
		"empty user id":   erUserErased(t, ""),
		"corrupt payload": {Key: []byte("k"), Value: func() []byte {
			b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e", Type: events.UserErasedEventType, Payload: []byte{0xff, 0xff}})
			require.NoError(t, err)
			return b
		}()},
	} {
		t.Run(name, func(t *testing.T) {
			err := h(context.Background(), rec)
			require.Error(t, err)
			assert.ErrorIs(t, err, events.ErrDeadLetter, "poison records are permanent errors so the consumer publishes them to the DLQ with an error header")
		})
	}
	assert.Equal(t, before, erTotalRows(t, w))
	assert.Zero(t, erCount(t, w, `SELECT count(*) FROM erased_users`))
}

// Scenario: AUE-4 A failed purge is retried and commits nothing
func TestErasureConsumerPurgeFailureRetriesWithoutCommit(t *testing.T) {
	w := erWriter(t)
	erSeed(t, w, "user-erase-4")
	flaky := &flakyEraser{inner: w, failures: 1}
	h, _, _ := erHandler(t, flaky)
	rec := erUserErased(t, "user-erase-4")

	err := h(context.Background(), rec)

	require.Error(t, err, "a failed purge must surface so the offset is not committed")
	assert.NotErrorIs(t, err, events.ErrDeadLetter, "a purge failure is transient: retry, never dead-letter")
	assert.Equal(t, 1, erCount(t, w, `SELECT count(*) FROM tracking_events WHERE principal_id = 'user-erase-4'`), "no partial change")

	require.NoError(t, h(context.Background(), rec), "the redelivery succeeds once the database accepts writes")
	erNoTrace(t, w, "user-erase-4")
}

// Scenario: AUE-5 Redelivery is a no-op
func TestErasureHandlerRedeliveryIsNoOp(t *testing.T) {
	w := erWriter(t)
	erSeed(t, w, "user-erase-5")
	h, _, _ := erHandler(t, w)
	rec := erUserErased(t, "user-erase-5")
	require.NoError(t, h(context.Background(), rec))
	snap := func() string {
		rows, err := w.DB().Query(`SELECT to_json(t)::VARCHAR FROM tracking_events t UNION ALL SELECT to_json(t)::VARCHAR FROM order_facts t UNION ALL SELECT to_json(t)::VARCHAR FROM erased_users t ORDER BY 1`)
		require.NoError(t, err)
		defer rows.Close()
		var parts []string
		for rows.Next() {
			var s string
			require.NoError(t, rows.Scan(&s))
			parts = append(parts, s)
		}
		return strings.Join(parts, "\n")
	}
	before := snap()

	require.NoError(t, h(context.Background(), rec))

	assert.Equal(t, before, snap())
	assert.Equal(t, 1, erCount(t, w, `SELECT count(*) FROM erased_users`))
}

// Scenario: AUE-15 Logs carry no raw id
func TestErasureLogsCarryNoRawID(t *testing.T) {
	w := erWriter(t)
	erSeed(t, w, "user-erase-15")
	h, buf, fp := erHandler(t, w)
	rec := erUserErased(t, "user-erase-15")

	require.NoError(t, h(context.Background(), rec))
	poison := erUserErased(t, "")
	poison.Key = []byte("user-erase-15")
	_ = h(context.Background(), poison)

	out := buf.String()
	assert.NotContains(t, out, "user-erase-15", "no raw user id in any log line")
	assert.Contains(t, out, fp.Of([]byte("user-erase-15")), "the kafkax key fingerprint is logged")
	for _, tbl := range []string{"tracking_events", "order_facts", "engagement_facts"} {
		assert.Contains(t, out, tbl, "per-table count for %s is logged", tbl)
	}
}

var evSeq atomic.Int64

func evID(prefix string) string { return prefix + "-" + strconv.FormatInt(evSeq.Add(1), 10) }
