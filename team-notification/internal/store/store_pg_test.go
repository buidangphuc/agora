package store_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/store"
)

var (
	_ store.Querier = (*pgxpool.Pool)(nil)
)

func migrationFiles(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	return sub
}

func notifDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrationFiles(t)); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url, MaxConns: 5})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func note(id, user, title string) *notificationv1.Notification {
	return &notificationv1.Notification{Id: id, UserId: user, Title: title, Body: "b-" + title, Type: notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER, LinkUrl: "/x"}
}

func TestMigrationsAreEmbeddedUnderInternalStore(t *testing.T) {
	entries, err := fs.ReadDir(migrationFiles(t), ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	want := []string{
		"001_create_notifications.up.sql",
		"002_alert_subscriptions.down.sql", "002_alert_subscriptions.up.sql",
		"003_notification_prefs.down.sql", "003_notification_prefs.up.sql",
		"004_consumer_state.down.sql", "004_consumer_state.up.sql",
		"005_low_stock_state.down.sql", "005_low_stock_state.up.sql",
	}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("embedded migrations = %v, want %v", names, want)
	}
}

func TestLegacyRepositoryAndMigrationsDirsAreGone(t *testing.T) {
	for _, p := range []string{"../repository", "../../migrations"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s must be deleted (stat err=%v)", p, err)
		}
	}
}

func TestMigrateUpIsRepeatableOnTheEmbeddedFiles(t *testing.T) {
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	for i := 0; i < 2; i++ {
		if err := migrate.Up(context.Background(), url, migrationFiles(t)); err != nil {
			t.Fatalf("migrate up run %d: %v", i+1, err)
		}
	}
}

func TestNotificationsOfAUserAreScoped(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	repo := store.NewNotifications(pool)
	for i := 0; i < 3; i++ {
		if _, err := repo.CreateNotification(ctx, note(fmt.Sprintf("a-%d", i), "alice", fmt.Sprintf("A%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.CreateNotification(ctx, note("b-0", "bob", "B0")); err != nil {
		t.Fatal(err)
	}

	got, total, err := repo.ListNotifications(ctx, "alice", 10, 0)
	if err != nil || len(got) != 3 || total != 3 {
		t.Fatalf("alice list: n=%d total=%d err=%v, want 3/3", len(got), total, err)
	}
	for _, n := range got {
		if n.UserId != "alice" {
			t.Fatalf("alice sees %s's notification %s", n.UserId, n.Id)
		}
	}
	if got[0].Id != "a-2" || got[2].Id != "a-0" {
		t.Fatalf("list must be newest first, got %s,%s,%s", got[0].Id, got[1].Id, got[2].Id)
	}
	page, total, err := repo.ListNotifications(ctx, "alice", 2, 2)
	if err != nil || len(page) != 1 || page[0].Id != "a-0" || total != 3 {
		t.Fatalf("second page: %v total=%d err=%v", page, total, err)
	}
	none, total, err := repo.ListNotifications(ctx, "carol", 10, 0)
	if err != nil || len(none) != 0 || total != 0 {
		t.Fatalf("carol: n=%d total=%d err=%v", len(none), total, err)
	}
	if c, err := repo.GetUnreadCount(ctx, "alice"); err != nil || c != 3 {
		t.Fatalf("alice unread=%d err=%v", c, err)
	}
	if c, err := repo.GetUnreadCount(ctx, "bob"); err != nil || c != 1 {
		t.Fatalf("bob unread=%d err=%v", c, err)
	}
}

func TestMarkAsReadIsScopedToTheOwner(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	repo := store.NewNotifications(pool)
	if _, err := repo.CreateNotification(ctx, note("n-1", "alice", "T")); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkAsRead(ctx, "n-1", "bob"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other user's mark = %v, want ErrNotFound", err)
	}
	if err := repo.MarkAsRead(ctx, "missing", "alice"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing id mark = %v, want ErrNotFound", err)
	}
	if c, _ := repo.GetUnreadCount(ctx, "alice"); c != 1 {
		t.Fatalf("foreign mark changed unread to %d", c)
	}
	if err := repo.MarkAsRead(ctx, "n-1", "alice"); err != nil {
		t.Fatalf("owner mark: %v", err)
	}
	if c, _ := repo.GetUnreadCount(ctx, "alice"); c != 0 {
		t.Fatalf("unread after mark = %d", c)
	}
}

func TestCreateNotificationAssignsIDAndTimestamp(t *testing.T) {
	pool := notifDB(t)
	got, err := store.NewNotifications(pool).CreateNotification(context.Background(), note("", "alice", "T"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Id == "" || got.CreatedAt == nil || got.IsRead {
		t.Fatalf("created = %+v", got)
	}
}

func TestUniqueViolationMapsToAlreadyExists(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	repo := store.NewNotifications(pool)
	if _, err := repo.CreateNotification(ctx, note("dup", "alice", "T")); err != nil {
		t.Fatal(err)
	}
	_, err := repo.CreateNotification(ctx, note("dup", "alice", "T"))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate id error = %v, want the raw pg 23505 error exposed unchanged", err)
	}
}

func TestStoreRunsInsideATransaction(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var q store.Querier = tx
	if _, err := store.NewNotifications(q).CreateNotification(ctx, note("tx-1", "alice", "T")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.NewProcessedEvents(q).MarkProcessed(ctx, "c", "e"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if c, _ := store.NewNotifications(pool).GetUnreadCount(ctx, "alice"); c != 0 {
		t.Fatalf("rolled back notification persisted: unread=%d", c)
	}
	if ok, _ := store.NewProcessedEvents(pool).IsProcessed(ctx, "c", "e"); ok {
		t.Fatal("rolled back ledger row persisted")
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.NewNotifications(tx2).CreateNotification(ctx, note("tx-2", "alice", "T")); err != nil {
		t.Fatal(err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if c, _ := store.NewNotifications(pool).GetUnreadCount(ctx, "alice"); c != 1 {
		t.Fatalf("committed notification missing: unread=%d", c)
	}
}

func TestSubscribingTwiceReturnsTheSameSubscription(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	repo := store.NewAlerts(pool)
	sub := func() *notificationv1.AlertSubscription {
		return &notificationv1.AlertSubscription{UserId: "alice", ListingId: "L1", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP}
	}
	first, err := repo.Create(ctx, sub())
	if err != nil || first.Id == "" {
		t.Fatalf("first create: %+v err=%v", first, err)
	}
	second, err := repo.Create(ctx, sub())
	if err != nil || second.Id != first.Id {
		t.Fatalf("second create id=%q err=%v, want %q", second.GetId(), err, first.Id)
	}
	list, err := repo.ListByUser(ctx, "alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByUser n=%d err=%v, want 1", len(list), err)
	}
	other, err := repo.Create(ctx, &notificationv1.AlertSubscription{UserId: "alice", ListingId: "L1", Type: notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK})
	if err != nil || other.Id == first.Id {
		t.Fatalf("different type must be a different subscription: %+v err=%v", other, err)
	}
}

func TestAlertSubscriptionsAreScoped(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	repo := store.NewAlerts(pool)
	mine, _ := repo.Create(ctx, &notificationv1.AlertSubscription{UserId: "alice", ListingId: "L1", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
	_, _ = repo.Create(ctx, &notificationv1.AlertSubscription{UserId: "bob", ListingId: "L1", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
	_, _ = repo.Create(ctx, &notificationv1.AlertSubscription{UserId: "bob", ListingId: "L1", Type: notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK})

	if l, err := repo.ListByUser(ctx, "alice"); err != nil || len(l) != 1 || l[0].UserId != "alice" || l[0].ListingId != "L1" || l[0].Type != notificationv1.AlertType_ALERT_TYPE_PRICE_DROP {
		t.Fatalf("alice list = %v err=%v", l, err)
	}
	if l, err := repo.ListByListingAndType(ctx, "L1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP); err != nil || len(l) != 2 {
		t.Fatalf("by listing+type n=%d err=%v, want 2", len(l), err)
	}
	if l, _ := repo.ListByListingAndType(ctx, "L2", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP); len(l) != 0 {
		t.Fatalf("other listing n=%d", len(l))
	}
	if err := repo.Delete(ctx, mine.Id, "bob"); err != nil {
		t.Fatalf("foreign delete must be a no-op: %v", err)
	}
	if l, _ := repo.ListByUser(ctx, "alice"); len(l) != 1 {
		t.Fatal("foreign delete removed alice's subscription")
	}
	if err := repo.Delete(ctx, mine.Id, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, mine.Id, "alice"); err != nil {
		t.Fatalf("repeat delete must be a no-op: %v", err)
	}
	if l, _ := repo.ListByUser(ctx, "alice"); len(l) != 0 {
		t.Fatal("delete did not remove the subscription")
	}
}

func TestPrefsRoundtripAndIsolation(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	repo := store.NewPrefs(pool)
	if _, err := repo.Get(ctx, "alice"); !errors.Is(err, store.ErrPrefsNotFound) {
		t.Fatalf("unset get = %v, want ErrPrefsNotFound", err)
	}
	in := &notificationv1.NotificationPrefs{TypeEnabled: map[string]bool{"NOTIFICATION_TYPE_ORDER": false, "NOTIFICATION_TYPE_CHAT": true}, DigestFreq: notificationv1.DigestFrequency_DIGEST_FREQUENCY_DAILY}
	if _, err := repo.Upsert(ctx, "alice", in); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "alice")
	if err != nil || got.DigestFreq != in.DigestFreq || len(got.TypeEnabled) != 2 || got.TypeEnabled["NOTIFICATION_TYPE_ORDER"] || !got.TypeEnabled["NOTIFICATION_TYPE_CHAT"] {
		t.Fatalf("get = %v err=%v", got, err)
	}
	if _, err := repo.Get(ctx, "bob"); !errors.Is(err, store.ErrPrefsNotFound) {
		t.Fatalf("bob sees alice's prefs: %v", err)
	}
	if _, err := repo.Upsert(ctx, "alice", &notificationv1.NotificationPrefs{DigestFreq: notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, "alice")
	if got.DigestFreq != notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY || len(got.TypeEnabled) != 0 {
		t.Fatalf("upsert must replace, got %v", got)
	}
}

func TestListUsersByDigestFreq(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	repo := store.NewPrefs(pool)
	for u, f := range map[string]notificationv1.DigestFrequency{
		"u3": notificationv1.DigestFrequency_DIGEST_FREQUENCY_DAILY,
		"u1": notificationv1.DigestFrequency_DIGEST_FREQUENCY_DAILY,
		"u2": notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY,
	} {
		if _, err := repo.Upsert(ctx, u, &notificationv1.NotificationPrefs{DigestFreq: f}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListUsersByDigestFreq(ctx, notificationv1.DigestFrequency_DIGEST_FREQUENCY_DAILY)
	if err != nil || fmt.Sprint(got) != "[u1 u3]" {
		t.Fatalf("daily = %v err=%v, want [u1 u3] sorted", got, err)
	}
	if got, _ := repo.ListUsersByDigestFreq(ctx, notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF); len(got) != 0 {
		t.Fatalf("off = %v", got)
	}
}

func TestPostgresDeduper(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	d := store.NewProcessedEvents(pool)
	if ok, err := d.IsProcessed(ctx, "c1", "e1"); err != nil || ok {
		t.Fatalf("fresh id: processed=%v err=%v", ok, err)
	}
	if fresh, err := d.MarkProcessed(ctx, "c1", "e1"); err != nil || !fresh {
		t.Fatalf("first mark: fresh=%v err=%v", fresh, err)
	}
	if fresh, err := d.MarkProcessed(ctx, "c1", "e1"); err != nil || fresh {
		t.Fatalf("second mark must report not-new: fresh=%v err=%v", fresh, err)
	}
	if ok, _ := d.IsProcessed(ctx, "c1", "e1"); !ok {
		t.Fatal("marked id must be processed")
	}
	if ok, _ := d.IsProcessed(ctx, "c2", "e1"); ok {
		t.Fatal("another consumer's ledger must not see it")
	}
}

func TestPostgresLastSeenStores(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	price := store.NewListingPrices(pool)
	stock := store.NewListingStocks(pool)

	if _, known, err := price.Get(ctx, "L1"); err != nil || known {
		t.Fatalf("unknown price: known=%v err=%v", known, err)
	}
	if err := price.Set(ctx, "L1", 2_000_000); err != nil {
		t.Fatal(err)
	}
	if p, known, err := price.Get(ctx, "L1"); err != nil || !known || p != 2_000_000 {
		t.Fatalf("price = %d known=%v err=%v", p, known, err)
	}
	if _, known, _ := stock.Get(ctx, "L1"); known {
		t.Fatal("stock must stay unknown until set")
	}
	if err := stock.Set(ctx, "L1", 0); err != nil {
		t.Fatal(err)
	}
	if s, known, err := stock.Get(ctx, "L1"); err != nil || !known || s != 0 {
		t.Fatalf("stock = %d known=%v err=%v", s, known, err)
	}
	if err := price.Set(ctx, "L1", 500_000); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := price.Get(ctx, "L1"); p != 500_000 {
		t.Fatalf("price after update = %d", p)
	}
	if s, known, _ := stock.Get(ctx, "L1"); !known || s != 0 {
		t.Fatal("updating price must not touch stock")
	}
}

func TestPostgresLowStockStoreDelete(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	s := store.NewLowStock(pool)

	if _, _, _, known, err := s.Get(ctx, "L1"); err != nil || known {
		t.Fatalf("fresh: known=%v err=%v", known, err)
	}
	if err := s.Upsert(ctx, "L1", "S1", "Giày", false); err != nil {
		t.Fatal(err)
	}
	seller, title, low, known, err := s.Get(ctx, "L1")
	if err != nil || !known || seller != "S1" || title != "Giày" || low {
		t.Fatalf("after upsert: %q %q low=%v known=%v err=%v", seller, title, low, known, err)
	}
	if err := s.SetLow(ctx, "L1", true); err != nil {
		t.Fatal(err)
	}
	if _, _, low, _, _ := s.Get(ctx, "L1"); !low {
		t.Fatal("SetLow(true) not persisted")
	}
	if err := s.Delete(ctx, "L1"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, known, err := s.Get(ctx, "L1"); err != nil || known {
		t.Fatalf("after delete: known=%v err=%v", known, err)
	}
	if err := s.Delete(ctx, "L1"); err != nil {
		t.Fatalf("deleting an absent row must not error: %v", err)
	}
}

func p95(d []time.Duration) time.Duration {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[(len(d)*95+99)/100-1]
}

func TestInboxQueriesP95At10kRows(t *testing.T) {
	pool := notifDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO notifications (id, user_id, title, body, type, link_url, is_read, created_at)
		SELECT 'perf-' || g, 'heavy', 't' || g, 'b', 1, '', false, now() - (g || ' seconds')::interval
		FROM generate_series(1, 10000) g`); err != nil {
		t.Fatalf("seed 10k rows: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE notifications`); err != nil {
		t.Fatal(err)
	}
	repo := store.NewNotifications(pool)

	const runs = 200
	const limit = 100 * time.Millisecond
	measure := func(name string, fn func(i int) error) {
		lat := make([]time.Duration, 0, runs)
		for i := 0; i < runs; i++ {
			start := time.Now()
			if err := fn(i); err != nil {
				t.Fatalf("%s run %d: %v", name, i, err)
			}
			lat = append(lat, time.Since(start))
		}
		if got := p95(lat); got >= limit {
			t.Fatalf("%s p95 = %v, want < %v", name, got, limit)
		}
	}
	measure("ListNotifications", func(int) error {
		got, total, err := repo.ListNotifications(ctx, "heavy", 20, 0)
		if err == nil && (len(got) != 20 || total != 10000) {
			return fmt.Errorf("n=%d total=%d", len(got), total)
		}
		return err
	})
	measure("GetUnreadCount", func(int) error {
		c, err := repo.GetUnreadCount(ctx, "heavy")
		if err == nil && c < 9000 {
			return fmt.Errorf("unread=%d", c)
		}
		return err
	})
	measure("MarkAsRead", func(i int) error { return repo.MarkAsRead(ctx, fmt.Sprintf("perf-%d", i+1), "heavy") })
}
