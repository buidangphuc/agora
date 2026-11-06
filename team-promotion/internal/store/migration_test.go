package store_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
)

func TestOutboxMigrationMatchesSDKSchema(t *testing.T) {
	want, err := outbox.SchemaSQL(outboxTable)
	if err != nil {
		t.Fatalf("SchemaSQL: %v", err)
	}
	if got := migrationSQL(t, "0005_outbox.up.sql"); got != want {
		t.Fatalf("0005_outbox.up.sql differs from outbox.SchemaSQL(%q):\n--- got\n%s\n--- want\n%s", outboxTable, got, want)
	}
}

func TestMigrateFromVersion4AppliesOutboxOnly(t *testing.T) {
	dsn := schemaDSN(t)
	pool := openPool(t, dsn)
	ctx := t.Context()

	for _, name := range []string{
		"0001_promotion.up.sql", "0002_subscriptions.up.sql", "0003_sponsored.up.sql", "0004_flash_sale_holds.up.sql",
	} {
		if _, err := pool.Exec(ctx, migrationSQL(t, name)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL);
		INSERT INTO schema_migrations (version, dirty) VALUES (4, false)`); err != nil {
		t.Fatalf("mark version 4: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vouchers (id, code, scope, quota, used) VALUES ('v-old', 'OLD10', 2, 5, 1);
		INSERT INTO flash_sale_campaigns (id, listing_id, sale_price, stock_cap, stock_sold) VALUES ('c-old', 'l-old', 99, 10, 4)`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	if err := migrate.Up(ctx, dsn, migrationsFS(t)); err != nil {
		t.Fatalf("migrate up from version 4: %v", err)
	}

	var version int64
	var dirty bool
	if err := pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if version != 8 || dirty {
		t.Fatalf("schema_migrations = version %d dirty %v, want 8 false", version, dirty)
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('`+outboxTable+`') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("%s exists=%v err=%v, want true", outboxTable, exists, err)
	}
	var code string
	var used int64
	if err := pool.QueryRow(ctx, `SELECT code, used FROM vouchers WHERE id = 'v-old'`).Scan(&code, &used); err != nil || code != "OLD10" || used != 1 {
		t.Fatalf("voucher after migrate = %q used %d err=%v, want unchanged", code, used, err)
	}
	var sold int64
	if err := pool.QueryRow(ctx, `SELECT stock_sold FROM flash_sale_campaigns WHERE id = 'c-old'`).Scan(&sold); err != nil || sold != 4 {
		t.Fatalf("campaign stock_sold = %d err=%v, want 4", sold, err)
	}
	if n := outboxCount(t, pool); n != 0 {
		t.Fatalf("outbox rows after migrate = %d, want 0", n)
	}

	if _, err := pool.Exec(ctx, migrationSQL(t, "0005_outbox.down.sql")); err != nil {
		t.Fatalf("apply 0005 down: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass('`+outboxTable+`') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("%s exists=%v err=%v after down, want false", outboxTable, exists, err)
	}
}

func TestMigration0007CampaignCancelAndIdempotency_CancelledAtColumn(t *testing.T) {
	dsn := schemaDSN(t)
	pool := openPool(t, dsn)
	ctx := t.Context()
	ups, err := fs.Glob(store.Migrations, "migrations/0007_*.up.sql")
	if err != nil || len(ups) != 1 {
		t.Fatalf("0007 up migration = %v err=%v, want exactly one", ups, err)
	}
	downs, err := fs.Glob(store.Migrations, "migrations/0007_*.down.sql")
	if err != nil || len(downs) != 1 {
		t.Fatalf("0007 down migration = %v err=%v, want exactly one", downs, err)
	}
	if err := migrate.Up(ctx, dsn, migrationsFS(t)); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	hasColumn := func() bool {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'flash_sale_campaigns' AND column_name = 'cancelled_at')`).Scan(&ok); err != nil {
			t.Fatalf("inspect columns: %v", err)
		}
		return ok
	}
	if !hasColumn() {
		t.Fatal("flash_sale_campaigns.cancelled_at missing after migrate up")
	}
	var nulls int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM flash_sale_campaigns WHERE cancelled_at IS NOT NULL`).Scan(&nulls); err != nil || nulls != 0 {
		t.Fatalf("cancelled rows on fresh table = %d err=%v", nulls, err)
	}
	body, err := fs.ReadFile(store.Migrations, downs[0])
	if err != nil {
		t.Fatalf("read down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(body)); err != nil {
		t.Fatalf("apply 0007 down: %v", err)
	}
	if hasColumn() {
		t.Fatal("cancelled_at still present after 0007 down")
	}
}

func columnExists(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, table, column string,
) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2)`, table, column).Scan(&ok); err != nil {
		t.Fatalf("inspect %s.%s: %v", table, column, err)
	}
	return ok
}

func tableExistsIn(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, table string,
) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = $1)`, table).Scan(&ok); err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	return ok
}

func TestMigration0008UpDown(t *testing.T) {
	pool := newPoolUpTo(t, 7)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `INSERT INTO vouchers (id, code, scope, discount_type, discount_value, quota, used)
		VALUES ('v-old', 'OLD10', 2, 2, 1000, 5, 1), ('v-old2', 'OLD20', 1, 1, 20, 0, 0)`); err != nil {
		t.Fatalf("seed vouchers: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO voucher_reservations (id, reservation_id, voucher_id, buyer_id, discount_amount, status, created_at, updated_at) VALUES
		('r1', 'res-reserved', 'v-old', 'b1', 100, 'reserved',  '2026-01-01 10:00:00+00', '2026-01-01 10:00:00+00'),
		('r2', 'res-committed', 'v-old', 'b2', 100, 'committed', '2026-01-01 11:00:00+00', '2026-01-01 11:00:00+00'),
		('r3', 'res-released', 'v-old', 'b3', 100, 'released',  '2026-01-01 12:00:00+00', '2026-01-01 12:00:00+00')`); err != nil {
		t.Fatalf("seed reservations: %v", err)
	}

	if _, err := pool.Exec(ctx, migrationSQL(t, "0008_user_vouchers.up.sql")); err != nil {
		t.Fatalf("apply 0008 up: %v", err)
	}

	for _, id := range []string{"v-old", "v-old2"} {
		var owner string
		var kind, perUser int32
		var newUserOnly bool
		if err := pool.QueryRow(ctx, `SELECT owner_user_id, kind, per_user_limit, new_user_only FROM vouchers WHERE id = $1`, id).Scan(&owner, &kind, &perUser, &newUserOnly); err != nil {
			t.Fatalf("read %s after up: %v", id, err)
		}
		if owner != "" || kind != 1 || perUser != 0 || newUserOnly {
			t.Errorf("%s after 0008 = owner %q kind %d per_user_limit %d new_user_only %v, want '' 1 0 false", id, owner, kind, perUser, newUserOnly)
		}
	}
	var code string
	var used int64
	if err := pool.QueryRow(ctx, `SELECT code, used FROM vouchers WHERE id = 'v-old'`).Scan(&code, &used); err != nil || code != "OLD10" || used != 1 {
		t.Fatalf("existing voucher changed by 0008: %q used %d err=%v", code, used, err)
	}
	var diff time.Duration
	if err := pool.QueryRow(ctx, `SELECT expires_at - created_at FROM voucher_reservations WHERE id = 'r1'`).Scan(&diff); err != nil || diff != 15*time.Minute {
		t.Fatalf("reserved row expires_at - created_at = %v err=%v, want 15m", diff, err)
	}
	for _, table := range []string{"promotion_processed_events", "new_user_voucher_state"} {
		if !tableExistsIn(t, pool, table) {
			t.Errorf("table %s missing after 0008 up", table)
		}
	}
	for _, c := range []string{"user_id", "registered", "has_code", "verdict"} {
		if !columnExists(t, pool, "new_user_voucher_state", c) {
			t.Errorf("new_user_voucher_state.%s missing", c)
		}
	}
	if columnExists(t, pool, "new_user_voucher_state", "referral_code") {
		t.Error("new_user_voucher_state stores the referral code, want only has_code")
	}
	if !columnExists(t, pool, "promotion_processed_events", "event_id") {
		t.Error("promotion_processed_events.event_id missing")
	}

	if _, err := pool.Exec(ctx, migrationSQL(t, "0008_user_vouchers.down.sql")); err != nil {
		t.Fatalf("apply 0008 down: %v", err)
	}
	for _, c := range []string{"owner_user_id", "kind", "per_user_limit", "new_user_only"} {
		if columnExists(t, pool, "vouchers", c) {
			t.Errorf("vouchers.%s still present after 0008 down", c)
		}
	}
	if columnExists(t, pool, "voucher_reservations", "expires_at") {
		t.Error("voucher_reservations.expires_at still present after 0008 down")
	}
	for _, table := range []string{"promotion_processed_events", "new_user_voucher_state"} {
		if tableExistsIn(t, pool, table) {
			t.Errorf("table %s still present after 0008 down", table)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT code, used FROM vouchers WHERE id = 'v-old'`).Scan(&code, &used); err != nil || code != "OLD10" || used != 1 {
		t.Fatalf("voucher lost by 0008 down: %q used %d err=%v", code, used, err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM voucher_reservations`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("reservations after 0008 down = %d err=%v, want 3", n, err)
	}
}

func TestNewUserUniqueIndex(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	insert := func(id, code, owner string, kind int) error {
		_, err := pool.Exec(ctx, `INSERT INTO vouchers (id, code, scope, discount_type, discount_value, quota, owner_user_id, kind)
			VALUES ($1, $2, 2, 2, 20000, 1, $3, $4)`, id, code, owner, kind)
		return err
	}
	pgCode := func(err error) string {
		var pe *pgconn.PgError
		if errors.As(err, &pe) {
			return pe.Code
		}
		return ""
	}

	if err := insert("n1", "NEWU-1", "user-a", 2); err != nil {
		t.Fatalf("first new-user voucher: %v", err)
	}
	if err := insert("n2", "NEWU-2", "user-a", 2); pgCode(err) != "23505" {
		t.Fatalf("second kind-2 voucher for the same owner: err=%v, want unique violation 23505", err)
	}
	if err := insert("n3", "NEWU-3", "user-b", 2); err != nil {
		t.Fatalf("kind-2 voucher for another owner: %v", err)
	}
	if err := insert("n4", "NEWU-4", "", 2); pgCode(err) != "23514" {
		t.Fatalf("kind-2 voucher without owner: err=%v, want check violation 23514", err)
	}
	if err := insert("s1", "STD-1", "user-a", 1); err != nil {
		t.Fatalf("kind-1 voucher with the same owner must stay allowed by the partial index: %v", err)
	}
	if err := insert("s2", "STD-2", "", 1); err != nil {
		t.Fatalf("public standard voucher: %v", err)
	}
	if err := insert("s3", "STD-3", "", 1); err != nil {
		t.Fatalf("second public standard voucher: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vouchers WHERE owner_user_id = 'user-a' AND kind = 2`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("kind-2 vouchers of user-a = %d err=%v, want 1", n, err)
	}
}

func TestReservationExpiresAtColumn(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	now := time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)
	vrepo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	rrepo := store.NewPostgresReservationRepository(pool)
	v, err := vrepo.Create(ctx, store.Voucher{Code: "EXP1", Scope: 2, DiscountType: 2, DiscountValue: 1000, Quota: 10, PerUserLimit: 1, Active: true}, voucherEvent)
	if err != nil {
		t.Fatalf("voucher create: %v", err)
	}
	expires := now.Add(15 * time.Minute)
	if _, created, err := rrepo.Create(ctx, store.Reservation{ReservationID: "e1", VoucherID: v.ID, BuyerID: "b1", DiscountAmount: 10, Status: store.ReservationReserved, ExpiresAt: expires}); err != nil || !created {
		t.Fatalf("create hold: created=%v err=%v", created, err)
	}
	got, err := rrepo.GetByReservationID(ctx, "e1")
	if err != nil || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("stored ExpiresAt = %v err=%v, want %v", got.ExpiresAt, err, expires)
	}

	count := func(at time.Time) int {
		t.Helper()
		n, err := rrepo.CountActiveByBuyer(ctx, v.ID, "b1", at)
		if err != nil {
			t.Fatalf("CountActiveByBuyer(%v): %v", at, err)
		}
		return n
	}
	if n := count(expires.Add(-time.Second)); n != 1 {
		t.Fatalf("count 1s before expiry = %d, want 1", n)
	}
	if n := count(expires); n != 0 {
		t.Fatalf("count at expiry = %d, want 0", n)
	}
	if n := count(expires.Add(time.Hour)); n != 0 {
		t.Fatalf("count after expiry = %d, want 0", n)
	}
	if err := rrepo.UpdateStatus(ctx, "e1", store.ReservationCommitted); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if n := count(expires.Add(24 * time.Hour)); n != 1 {
		t.Fatalf("committed hold past expires_at counts %d, want 1", n)
	}

	if _, _, err := rrepo.Create(ctx, store.Reservation{ReservationID: "e2", VoucherID: v.ID, BuyerID: "b1", DiscountAmount: 10, Status: store.ReservationReserved, ExpiresAt: expires}); err != nil {
		t.Fatalf("create second hold: %v", err)
	}
	if n := count(expires.Add(-time.Second)); n != 2 {
		t.Fatalf("committed plus live hold = %d, want 2", n)
	}
	if err := rrepo.UpdateStatus(ctx, "e2", store.ReservationReleased); err != nil {
		t.Fatalf("release: %v", err)
	}
	if n := count(expires.Add(-time.Second)); n != 1 {
		t.Fatalf("released hold must not count: got %d, want 1", n)
	}
	if n, err := rrepo.CountActiveByBuyer(ctx, v.ID, "someone-else", expires.Add(-time.Second)); err != nil || n != 0 {
		t.Fatalf("other buyer count = %d err=%v, want 0", n, err)
	}
}

func TestVoucherPerUserFieldsRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	vrepo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	if _, err := vrepo.Create(ctx, store.Voucher{Code: "PU3", Scope: 2, DiscountType: 2, DiscountValue: 1000, Quota: 10, PerUserLimit: 3, NewUserOnly: true, Active: true}, voucherEvent); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := vrepo.GetByCode(ctx, "PU3")
	if err != nil {
		t.Fatalf("GetByCode: %v", err)
	}
	if got.PerUserLimit != 3 || !got.NewUserOnly || got.OwnerUserID != "" || got.Kind != store.VoucherKindStandard {
		t.Fatalf("round trip = per_user_limit %d new_user_only %v owner %q kind %d, want 3 true '' STANDARD", got.PerUserLimit, got.NewUserOnly, got.OwnerUserID, got.Kind)
	}
}
