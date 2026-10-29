package store_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-order/internal/store"
)

const (
	migrationsDir = "../../migrations"
	lastMigration = 22
	orderTopic    = "order.events"
)

func migrationVersion(name string) int {
	n, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
	if err != nil {
		return -1
	}
	return n
}

func migrationsUpTo(t *testing.T, maxVersion int) fs.FS {
	t.Helper()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	m := fstest.MapFS{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") || migrationVersion(e.Name()) > maxVersion {
			continue
		}
		b, err := os.ReadFile(migrationsDir + "/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		m[e.Name()] = &fstest.MapFile{Data: b}
	}
	return m
}

func schemaVersion(t *testing.T, pool *pgxpool.Pool) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := pool.QueryRow(t.Context(), `SELECT version, dirty FROM schema_migrations`).Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func newRelayer(t *testing.T, pool *pgxpool.Pool, pub outbox.Publisher) *outbox.Relayer {
	t.Helper()
	rl, err := outbox.NewRelayer(newOutbox(t, pool), pub, outbox.RelayerOptions{
		Topic:       orderTopic,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  time.Millisecond,
	})
	if err != nil {
		t.Fatalf("relayer: %v", err)
	}
	return rl
}

func drain(t *testing.T, rl *outbox.Relayer) {
	t.Helper()
	for i := 0; i < 20; i++ {
		rep, err := rl.RelayOnce(t.Context())
		if err != nil {
			t.Fatalf("relay pass: %v", err)
		}
		if rep.Claimed == 0 {
			return
		}
	}
	t.Fatal("relayer still claiming rows after 20 passes")
}

func eventIDs(recs []testkit.Record) []string {
	var out []string
	for _, r := range recs {
		for _, h := range r.Headers {
			if h.Key == kafkax.HeaderEventID {
				out = append(out, string(h.Value))
			}
		}
	}
	return out
}

func newPaidFixture(t *testing.T, pool *pgxpool.Pool, build store.PaidOutboxBuilder) (store.OrderRepository, store.Order) {
	t.Helper()
	repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithPaidOutbox(build))
	o, err := repo.CreateOrder(t.Context(), store.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10, Currency: "VND",
		Items: []store.OrderItem{{ListingID: "l1", Quantity: 1, UnitPrice: 10}}})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return repo, o
}

func outboxRows(t *testing.T, pool *pgxpool.Pool, orderID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1`, orderID).Scan(&n); err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	return n
}

// Scenario: A paid transition writes its outbox row in the same transaction
func TestPaidTransitionWritesOutboxRow(t *testing.T) {
	pool := pgPool(t)
	repo, o := newPaidFixture(t, pool, BuildPaidOutboxRow)
	pending := []store.OrderStatus{store.OrderStatusPending}

	got, err := repo.UpdateOrderStatusFrom(t.Context(), o.ID, store.OrderStatusPaid, pending, "")
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	if got.Status != store.OrderStatusPaid {
		t.Fatalf("status = %v, want PAID", got.Status)
	}
	read, err := repo.GetOrder(t.Context(), o.ID)
	if err != nil || read.Status != store.OrderStatusPaid {
		t.Fatalf("stored order = %+v err=%v, want PAID", read, err)
	}

	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1 AND event_type = $2 AND status = 'pending' AND seq IS NOT NULL`,
		o.ID, OrderPaidEventType).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pending OrderPaidEvent rows for the order = %d err=%v, want exactly 1", n, err)
	}
	if total := outboxRows(t, pool, o.ID); total != 1 {
		t.Fatalf("outbox rows for the order = %d, want 1", total)
	}

	rec := testkit.NewRecorder()
	drain(t, newRelayer(t, pool, rec))
	recs := rec.Records()
	if len(recs) != 1 || recs[0].Topic != orderTopic || string(recs[0].Key) != o.ID {
		t.Fatalf("published = %+v, want one record on %s keyed by the order id", recs, orderTopic)
	}

	if _, err := repo.UpdateOrderStatusFrom(t.Context(), o.ID, store.OrderStatusPaid, pending, ""); !errors.Is(err, store.ErrStatusConflict) {
		t.Fatalf("second pay: want ErrStatusConflict, got %v", err)
	}
	if total := outboxRows(t, pool, o.ID); total != 1 {
		t.Fatalf("outbox rows after a refused re-pay = %d, want 1", total)
	}
}

// Scenario: A failing enqueue rolls the transition back
func TestPaidTransitionRollsBackWhenOutboxFails(t *testing.T) {
	pool := pgPool(t)
	repo, o := newPaidFixture(t, pool, func(o store.Order) (outbox.Row, error) {
		return outbox.Row{AggregateType: "Order", AggregateID: o.ID, EventType: OrderPaidEventType, Payload: []byte("env")}, nil
	})

	_, err := repo.UpdateOrderStatusFrom(t.Context(), o.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, "")
	if err == nil {
		t.Fatal("pay must fail when the outbox insert is rejected")
	}
	got, gerr := repo.GetOrder(t.Context(), o.ID)
	if gerr != nil {
		t.Fatalf("get order: %v", gerr)
	}
	if got.Status != store.OrderStatusPending || got.PaidAt != nil {
		t.Fatalf("order = %+v, want PENDING and unpaid after the rollback", got)
	}
	if n := outboxRows(t, pool, o.ID); n != 0 {
		t.Fatalf("outbox rows = %d, want 0 after the rollback", n)
	}
}

func insertOldOutboxRow(t *testing.T, pool *pgxpool.Pool, id, aggregate, status string, attempts int, errText *string, createdAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO order_outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, error, available_at, created_at)
		VALUES ($1, 'Order', $2, 'platform.order.v1.OrderPaidEvent', $3, '', $4, $5, $6, $7, $7)`,
		id, aggregate, []byte("payload-"+id), status, attempts, errText, createdAt); err != nil {
		t.Fatalf("insert old row %s: %v", id, err)
	}
}

// Scenario: The outbox upgrade keeps pending events in order
func TestOutboxUpgradeKeepsPendingOrder(t *testing.T) {
	dsn, pool := newDatabase(t)
	if err := migrate.Up(t.Context(), dsn, migrationsUpTo(t, 11)); err != nil {
		t.Fatalf("migrate to 11: %v", err)
	}
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		id  string
		age time.Duration
	}{
		{"e-d", 3 * time.Minute},
		{"e-a", 1 * time.Minute},
		{"e-e", 4 * time.Minute},
		{"e-c", 2 * time.Minute},
		{"e-b", 2 * time.Minute},
	} {
		insertOldOutboxRow(t, pool, r.id, "O1", "pending", 0, nil, base.Add(r.age))
	}

	if err := migrate.Up(t.Context(), dsn, os.DirFS(migrationsDir)); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if v, dirty := schemaVersion(t, pool); v != lastMigration || dirty {
		t.Fatalf("schema version = %d dirty=%v, want %d clean", v, dirty, lastMigration)
	}

	rec := testkit.NewRecorder()
	drain(t, newRelayer(t, pool, rec))
	recs := rec.Records()
	want := []string{"e-a", "e-b", "e-c", "e-d", "e-e"}
	if got := eventIDs(recs); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("published event ids = %v, want (created_at, event_id) order %v", got, want)
	}
	for i, r := range recs {
		if string(r.Value) != "payload-"+want[i] || string(r.Key) != "O1" || r.Topic != orderTopic {
			t.Errorf("record %d = key %q topic %q value %q, want key O1 topic %s payload-%s", i, r.Key, r.Topic, r.Value, orderTopic, want[i])
		}
	}
}

// Scenario: The outbox upgrade keeps pending events in order
func TestOutboxUpgradeBuildsIndexesAndDropsClaimIndex(t *testing.T) {
	pool := pgPool(t)
	have := map[string]bool{}
	rows, err := pool.Query(t.Context(), `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema() AND tablename = 'order_outbox_events'`)
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		have[n] = true
	}
	rows.Close()
	for _, want := range []string{"order_outbox_events_status_idx", "order_outbox_events_agg_idx", "order_outbox_events_pending_idx"} {
		if !have[want] {
			t.Errorf("index %s missing (have %v)", want, have)
		}
	}
	for _, gone := range []string{"order_outbox_events_claim_idx", "idx_order_outbox_claim"} {
		if have[gone] {
			t.Errorf("index %s must be dropped (have %v)", gone, have)
		}
	}
	var nullable, def string
	if err := pool.QueryRow(t.Context(), `SELECT is_nullable, COALESCE(column_default, '') FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'order_outbox_events' AND column_name = 'seq'`).Scan(&nullable, &def); err != nil {
		t.Fatalf("seq column: %v", err)
	}
	if nullable != "NO" || !strings.HasPrefix(def, "nextval(") {
		t.Fatalf("seq nullable=%s default=%q, want NOT NULL with a nextval default", nullable, def)
	}
}

// Scenario: The upgrade requeues rows parked as failed
func TestOutboxUpgradeRequeuesFailedRows(t *testing.T) {
	dsn, pool := newDatabase(t)
	if err := migrate.Up(t.Context(), dsn, migrationsUpTo(t, 11)); err != nil {
		t.Fatalf("migrate to 11: %v", err)
	}
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	reason := "max attempts"
	insertOldOutboxRow(t, pool, "f-1", "O1", "failed", 10, &reason, base)
	insertOldOutboxRow(t, pool, "f-2", "O1", "failed", 10, &reason, base.Add(time.Minute))
	insertOldOutboxRow(t, pool, "d-1", "O2", "published", 1, nil, base.Add(2*time.Minute))

	if err := migrate.Up(t.Context(), dsn, os.DirFS(migrationsDir)); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, id := range []string{"f-1", "f-2"} {
		var status string
		var attempts int
		var errText *string
		if err := pool.QueryRow(t.Context(), `SELECT status, attempts, error FROM order_outbox_events WHERE event_id = $1`, id).Scan(&status, &attempts, &errText); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if status != "pending" || attempts != 0 {
			t.Errorf("%s = status %s attempts %d, want pending with attempts reset", id, status, attempts)
		}
	}

	rec := testkit.NewRecorder()
	rl := newRelayer(t, pool, rec)
	drain(t, rl)
	if got := eventIDs(rec.Records()); strings.Join(got, ",") != "f-1,f-2" {
		t.Fatalf("published event ids = %v, want [f-1 f-2] in seq order and not the published row", got)
	}
	drain(t, rl)
	if got := eventIDs(rec.Records()); len(got) != 2 {
		t.Fatalf("a second pass published again: %v", got)
	}
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM order_outbox_events WHERE event_id = 'd-1'`).Scan(&status); err != nil || status != "published" {
		t.Fatalf("published row = %q err=%v, want unchanged", status, err)
	}
}

// Scenario: The same event id is independent per consumer
func TestProcessedEventsKeyIsPerConsumer(t *testing.T) {
	pool := pgPool(t)
	dedupe, err := consume.NewDedupe("processed_events")
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	ctx := t.Context()

	if dup, err := dedupe.Seen(ctx, pool, "team-order.payment", "e-1"); err != nil || dup {
		t.Fatalf("first Seen = dup %v err %v, want a fresh insert", dup, err)
	}
	if done, err := dedupe.Processed(ctx, pool, "team-order.dispute", "e-1"); err != nil || done {
		t.Fatalf("dispute Processed(e-1) = %v err %v, want not processed", done, err)
	}
	if done, err := dedupe.Processed(ctx, pool, "team-order.payment", "e-1"); err != nil || !done {
		t.Fatalf("payment Processed(e-1) = %v err %v, want processed", done, err)
	}
	if dup, err := dedupe.Seen(ctx, pool, "team-order.dispute", "e-1"); err != nil || dup {
		t.Fatalf("dispute Seen(e-1) = dup %v err %v, want its own insert", dup, err)
	}
	if dup, err := dedupe.Seen(ctx, pool, "team-order.payment", "e-1"); err != nil || !dup {
		t.Fatalf("payment Seen(e-1) again = dup %v err %v, want a duplicate", dup, err)
	}
}

// Scenario: The key swap keeps existing rows
func TestProcessedEventsPKMigrationKeepsRows(t *testing.T) {
	dsn, pool := newDatabase(t)
	if err := migrate.Up(t.Context(), dsn, migrationsUpTo(t, 10)); err != nil {
		t.Fatalf("migrate to 10: %v", err)
	}
	for _, id := range []string{"e-1", "e-2", "e-3"} {
		if _, err := pool.Exec(t.Context(), `INSERT INTO processed_events (event_id, consumer) VALUES ($1, 'team-order.payment')`, id); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	if err := migrate.Up(t.Context(), dsn, migrationsUpTo(t, 11)); err != nil {
		t.Fatalf("migrate to 11: %v", err)
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM processed_events`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("rows after the key swap = %d err=%v, want 3", n, err)
	}
	rows, err := pool.Query(t.Context(), `SELECT a.attname FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
		WHERE i.indrelid = 'processed_events'::regclass AND i.indisprimary ORDER BY a.attname`)
	if err != nil {
		t.Fatalf("primary key columns: %v", err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		cols = append(cols, c)
	}
	if strings.Join(cols, ",") != "consumer,event_id" {
		t.Fatalf("primary key columns = %v, want [consumer event_id]", cols)
	}
}

// Scenario: All migrations apply and reverse
func TestMigrationsApplyAndReverse(t *testing.T) {
	dsn, pool := newDatabase(t)
	if err := migrate.Up(t.Context(), dsn, os.DirFS(migrationsDir)); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if v, dirty := schemaVersion(t, pool); v != lastMigration || dirty {
		t.Fatalf("schema version = %d dirty=%v, want %d clean", v, dirty, lastMigration)
	}

	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())
	for v := lastMigration; v >= 1; v-- {
		matches := downFiles(t, v)
		if len(matches) != 1 {
			t.Fatalf("migration %04d: want exactly one down file, found %v", v, matches)
		}
		b, err := os.ReadFile(migrationsDir + "/" + matches[0])
		if err != nil {
			t.Fatalf("read %s: %v", matches[0], err)
		}
		if _, err := conn.Exec(t.Context(), string(b)); err != nil {
			t.Fatalf("apply %s: %v", matches[0], err)
		}
	}

	rows, err := conn.Query(t.Context(), `SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name <> 'schema_migrations'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var left []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		left = append(left, n)
	}
	if len(left) != 0 {
		t.Fatalf("tables left after every down migration: %v", left)
	}
}

func downFiles(t *testing.T, version int) []string {
	t.Helper()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".down.sql") && migrationVersion(e.Name()) == version {
			out = append(out, e.Name())
		}
	}
	return out
}
