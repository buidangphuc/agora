package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/events"
	"github.com/buidangphuc/team-payment/internal/store"
)

func insertPendingPayment(t *testing.T, pool *pgxpool.Pool, id, orderID string) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status) VALUES ($1, $2, 'buyer-1', 300000, 1)`, id, orderID)
}

func paymentStatus(t *testing.T, pool *pgxpool.Pool, id string) int32 {
	t.Helper()
	var st int32
	if err := pool.QueryRow(context.Background(), `SELECT status FROM payment_transactions WHERE id = $1`, id).Scan(&st); err != nil {
		t.Fatalf("read payment status: %v", err)
	}
	return st
}

func outboxCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+outboxTable).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}

type outboxRowState struct {
	status   string
	attempts int
	seq      int64
}

func rowState(t *testing.T, pool *pgxpool.Pool, eventID string) outboxRowState {
	t.Helper()
	var s outboxRowState
	if err := pool.QueryRow(context.Background(), `SELECT status, attempts FROM `+outboxTable+` WHERE event_id = $1`, eventID).Scan(&s.status, &s.attempts); err != nil {
		t.Fatalf("read outbox row %s: %v", eventID, err)
	}
	return s
}

func enqueue(t *testing.T, pool *pgxpool.Pool, s *outbox.Store, orderID, eventID, payload string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	row := outbox.Row{EventID: eventID, AggregateType: "Payment", AggregateID: orderID, EventType: events.PaymentSettledEventType, Payload: []byte(payload)}
	if err := s.EnqueueTx(ctx, tx, row); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("enqueue %s: %v", eventID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func settleWithEnvelope(eventID string) store.SettleFn {
	return func(settled store.PaymentTransaction) (outbox.Row, error) {
		value, err := events.BuildPaymentSettledEnvelope(eventID, settled.ID, settled.OrderID, settled.BuyerID, paymentv1.PaymentStatus_PAYMENT_STATUS_PAID, time.Now(), "req-1")
		if err != nil {
			return outbox.Row{}, err
		}
		return outbox.Row{EventID: eventID, AggregateType: "Payment", AggregateID: settled.OrderID, EventType: events.PaymentSettledEventType, Payload: value}, nil
	}
}

func TestSettleEventIsRelayedOnce(t *testing.T) {
	pool := migratedPool(t)
	ob := newOutbox(t, pool)
	insertPendingPayment(t, pool, "pay-1", "order-1")

	updated, err := store.NewPgTxWriter(pool, ob).SettleTx(context.Background(), "pay-1", store.PaymentStatusPaid, "prov-1", settleWithEnvelope("evt-settle-1"))
	if err != nil {
		t.Fatalf("SettleTx: %v", err)
	}
	if updated.Status != store.PaymentStatusPaid {
		t.Fatalf("settled status = %v, want PAID", updated.Status)
	}

	rec := testkit.NewRecorder()
	relayer := newRelayer(t, ob, rec)
	relayUntilQuiet(t, relayer, 4)
	relayUntilQuiet(t, relayer, 2)

	got := rec.Records()
	if len(got) != 1 {
		t.Fatalf("produced %d records, want exactly 1", len(got))
	}
	if got[0].Topic != "payment.events" {
		t.Fatalf("topic = %q, want payment.events", got[0].Topic)
	}
	if string(got[0].Key) != "order-1" {
		t.Fatalf("key = %q, want the order id", got[0].Key)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(got[0].Value, &env); err != nil {
		t.Fatalf("value is not an EventEnvelope: %v", err)
	}
	if env.GetType() != "platform.payment.v1.PaymentSettled" {
		t.Fatalf("envelope type = %q", env.GetType())
	}
	var rowEventID, status string
	if err := pool.QueryRow(context.Background(), `SELECT event_id, status FROM `+outboxTable).Scan(&rowEventID, &status); err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	if env.GetEventId() != rowEventID || status != "published" {
		t.Fatalf("envelope event_id %q, outbox event_id %q status %q: want equal ids and published", env.GetEventId(), rowEventID, status)
	}
}

func TestSettleTxRollsBackWhenOutboxInsertFails_SettleWritesPaymentAndOutboxInOneTransaction(t *testing.T) {
	pool := migratedPool(t)
	ob := newOutbox(t, pool)
	insertPendingPayment(t, pool, "pay-rb", "order-rb")
	w := store.NewPgTxWriter(pool, ob)

	_, err := w.SettleTx(context.Background(), "pay-rb", store.PaymentStatusPaid, "prov", func(store.PaymentTransaction) (outbox.Row, error) {
		return outbox.Row{AggregateType: "Payment", AggregateID: "order-rb"}, nil
	})
	if err == nil {
		t.Fatal("SettleTx with an unusable outbox row must fail")
	}
	if got := paymentStatus(t, pool, "pay-rb"); got != 1 {
		t.Fatalf("payment status after failed enqueue = %d, want 1 (PENDING)", got)
	}
	if n := outboxCount(t, pool); n != 0 {
		t.Fatalf("outbox rows after rollback = %d, want 0", n)
	}

	if _, err := w.SettleTx(context.Background(), "pay-rb", store.PaymentStatusPaid, "prov", settleWithEnvelope("evt-rb")); err != nil {
		t.Fatalf("SettleTx with a working insert: %v", err)
	}
	if got := paymentStatus(t, pool, "pay-rb"); got != 2 {
		t.Fatalf("payment status = %d, want 2 (PAID)", got)
	}
	if n := outboxCount(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
}

func insertLegacyRow(t *testing.T, pool *pgxpool.Pool, id, orderID, status string, attempts int, createdAt time.Time, publishedAt *time.Time) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO `+outboxTable+`
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at, published_at)
		VALUES ($1, 'Payment', $2, 'platform.payment.v1.PaymentSettled', $3, '', $4, $5, $6, $6, $7)`,
		id, orderID, []byte(id), status, attempts, createdAt, publishedAt)
}

func TestOutboxUpgradeKeepsPendingOrder(t *testing.T) {
	pool := schemaPool(t)
	applyMigrations(t, pool, 1, 7)

	base := time.Now().Add(-time.Hour)
	insertLegacyRow(t, pool, "evt-c", "order-up", "pending", 0, base.Add(2*time.Second), nil)
	insertLegacyRow(t, pool, "evt-a", "order-up", "pending", 0, base, nil)
	insertLegacyRow(t, pool, "evt-b", "order-up", "pending", 0, base.Add(time.Second), nil)

	applyMigrations(t, pool, 8, 17)

	rows, err := pool.Query(context.Background(), `SELECT event_id, status, seq FROM `+outboxTable+` ORDER BY created_at`)
	if err != nil {
		t.Fatalf("query upgraded rows: %v", err)
	}
	defer rows.Close()
	var ids []string
	var prev int64
	for rows.Next() {
		var id, status string
		var seq int64
		if err := rows.Scan(&id, &status, &seq); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if status != "pending" {
			t.Fatalf("%s status = %q, want pending", id, status)
		}
		if seq <= prev {
			t.Fatalf("%s seq = %d, want greater than %d (created_at order)", id, seq, prev)
		}
		prev = seq
		ids = append(ids, id)
	}
	if want := []string{"evt-a", "evt-b", "evt-c"}; len(ids) != 3 || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Fatalf("created_at order = %v, want %v", ids, want)
	}

	pub := newScriptedPublisher(nil)
	relayUntilQuiet(t, newRelayer(t, newOutbox(t, pool), pub), 6)
	got := pub.published()
	if len(got) != 3 || got[0] != "evt-a" || got[1] != "evt-b" || got[2] != "evt-c" {
		t.Fatalf("relay order = %v, want [evt-a evt-b evt-c]", got)
	}
}

func TestOutboxUpgradeRequeuesFailedRows(t *testing.T) {
	pool := schemaPool(t)
	applyMigrations(t, pool, 1, 7)

	base := time.Now().Add(-time.Hour)
	pub := base.Add(time.Minute)
	insertLegacyRow(t, pool, "evt-failed", "order-f", "failed", 5, base, nil)
	insertLegacyRow(t, pool, "evt-done", "order-d", "published", 1, base, &pub)

	applyMigrations(t, pool, 8, 17)

	if got := rowState(t, pool, "evt-failed"); got.status != "pending" || got.attempts != 0 {
		t.Fatalf("failed row after upgrade = %+v, want pending with 0 attempts", got)
	}
	if got := rowState(t, pool, "evt-done"); got.status != "published" || got.attempts != 1 {
		t.Fatalf("published row after upgrade = %+v, want unchanged (published, 1 attempt)", got)
	}
	var publishedAt *time.Time
	if err := pool.QueryRow(context.Background(), `SELECT published_at FROM `+outboxTable+` WHERE event_id = 'evt-done'`).Scan(&publishedAt); err != nil || publishedAt == nil {
		t.Fatalf("published_at of the published row = %v (err %v), want kept", publishedAt, err)
	}
}

func TestClaimPendingKeepsCreationOrder_Postgres(t *testing.T) {
	pool := migratedPool(t)
	ob := newOutbox(t, pool)
	enqueue(t, pool, ob, "order-ord", "evt-first", "first")
	enqueue(t, pool, ob, "order-ord", "evt-second", "second")

	pub := newScriptedPublisher(func(v string, call int) error {
		if v == "first" && call == 1 {
			return errors.New("broker unavailable")
		}
		return nil
	})
	relayer := newRelayer(t, ob, pub)

	if _, err := relayer.RelayOnce(context.Background()); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if got := pub.published(); len(got) != 0 {
		t.Fatalf("published %v while the first event failed, want nothing (second must not overtake)", got)
	}
	makeDue(t, pool)
	relayUntilQuiet(t, relayer, 4)
	got := pub.published()
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("publish order after retry = %v, want [first second]", got)
	}
}

func TestRelayerRetriesTransientPublishErrors(t *testing.T) {
	pool := migratedPool(t)
	ob := newOutbox(t, pool)
	enqueue(t, pool, ob, "order-tr", "evt-tr", "payload")

	pub := newScriptedPublisher(func(_ string, call int) error {
		if call <= 2 {
			return errors.New("broker unavailable")
		}
		return nil
	})
	relayer := newRelayer(t, ob, pub)

	for i := 1; i <= 2; i++ {
		if _, err := relayer.RelayOnce(context.Background()); err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
		makeDue(t, pool)
	}
	if got := rowState(t, pool, "evt-tr"); got.status != "pending" || got.attempts != 2 {
		t.Fatalf("after 2 transient failures row = %+v, want pending with 2 attempts", got)
	}
	if n := len(pub.published()); n != 0 {
		t.Fatalf("published %d records during the outage, want 0", n)
	}

	if _, err := relayer.RelayOnce(context.Background()); err != nil {
		t.Fatalf("third pass: %v", err)
	}
	if got := rowState(t, pool, "evt-tr"); got.status != "published" {
		t.Fatalf("after the third attempt row = %+v, want published", got)
	}
	if n := len(pub.published()); n != 1 {
		t.Fatalf("published %d records, want exactly 1", n)
	}
}

func TestRelayerParksPermanentErrors(t *testing.T) {
	pool := migratedPool(t)
	ob := newOutbox(t, pool)
	enqueue(t, pool, ob, "order-bad", "evt-bad", "bad")
	enqueue(t, pool, ob, "order-good", "evt-good", "good")

	pub := newScriptedPublisher(func(v string, _ int) error {
		if v == "bad" {
			return outbox.Permanent(errors.New("cannot encode"))
		}
		return nil
	})
	relayer := newRelayer(t, ob, pub)
	relayUntilQuiet(t, relayer, 4)
	makeDue(t, pool)
	relayUntilQuiet(t, relayer, 4)

	if got := rowState(t, pool, "evt-bad"); got.status != "failed" {
		t.Fatalf("permanent row = %+v, want failed", got)
	}
	if n := pub.callCount("bad"); n != 1 {
		t.Fatalf("permanent row attempted %d times, want 1 (not retried)", n)
	}
	if got := rowState(t, pool, "evt-good"); got.status != "published" {
		t.Fatalf("later row of another order = %+v, want published", got)
	}
}

func TestPurgeDeletesPublishedRowsOlderThanSevenDays(t *testing.T) {
	pool := migratedPool(t)
	ob := newOutbox(t, pool)
	now := time.Now()
	day := 24 * time.Hour
	old, recent := now.Add(-8*day), now.Add(-6*day)
	insertLegacyRow(t, pool, "evt-old", "o1", "published", 1, old.Add(-time.Minute), &old)
	insertLegacyRow(t, pool, "evt-recent", "o2", "published", 1, recent.Add(-time.Minute), &recent)
	insertLegacyRow(t, pool, "evt-pending", "o3", "pending", 0, now.Add(-9*day), nil)

	n, err := store.PurgeOutbox(context.Background(), ob)
	if err != nil {
		t.Fatalf("PurgeOutbox: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged %d rows, want 1", n)
	}
	rows, err := pool.Query(context.Background(), `SELECT event_id FROM `+outboxTable+` ORDER BY event_id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var left []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		left = append(left, id)
	}
	if len(left) != 2 || left[0] != "evt-pending" || left[1] != "evt-recent" {
		t.Fatalf("rows left = %v, want [evt-pending evt-recent]", left)
	}
}
