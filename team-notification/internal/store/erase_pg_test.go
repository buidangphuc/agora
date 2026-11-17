package store_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/store"
)

const (
	eraseVictim    = "user-erase-victim"
	eraseBystander = "user-erase-bystander"
)

type eraseFixture struct {
	pool  *pgxpool.Pool
	notes *store.Notifications
	alert *store.Alerts
	prefs *store.Prefs
	erase *store.Eraser
}

func seedErase(t *testing.T) eraseFixture {
	t.Helper()
	pool := notifDB(t)
	ctx := context.Background()
	f := eraseFixture{pool: pool, notes: store.NewNotifications(pool), alert: store.NewAlerts(pool), prefs: store.NewPrefs(pool), erase: store.NewEraser(pool)}
	for i, u := range []string{eraseVictim, eraseVictim, eraseVictim, eraseVictim, eraseBystander, eraseBystander} {
		id := "n-" + string(rune('a'+i))
		if _, err := f.notes.CreateNotification(ctx, note(id, u, "T"+id)); err != nil {
			t.Fatalf("seed notification: %v", err)
		}
	}
	for _, s := range []struct {
		user, listing string
		typ           notificationv1.AlertType
	}{
		{eraseVictim, "L1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP},
		{eraseVictim, "L2", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK},
		{eraseBystander, "L1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP},
		{eraseBystander, "L2", notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK},
	} {
		if _, err := f.alert.Create(ctx, &notificationv1.AlertSubscription{UserId: s.user, ListingId: s.listing, Type: s.typ}); err != nil {
			t.Fatalf("seed alert: %v", err)
		}
	}
	for _, u := range []string{eraseVictim, eraseBystander} {
		if _, err := f.prefs.Upsert(ctx, u, &notificationv1.NotificationPrefs{TypeEnabled: map[string]bool{"NOTIFICATION_TYPE_ORDER": false}, DigestFreq: notificationv1.DigestFrequency_DIGEST_FREQUENCY_DAILY}); err != nil {
			t.Fatalf("seed prefs: %v", err)
		}
	}
	return f
}

func eraseDump(t *testing.T, pool *pgxpool.Pool, query string, args ...any) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func eraseSnapshot(t *testing.T, pool *pgxpool.Pool) map[string][]string {
	t.Helper()
	return map[string][]string{
		"notifications": eraseDump(t, pool, `SELECT n::text FROM notifications n ORDER BY id`),
		"alerts":        eraseDump(t, pool, `SELECT a::text FROM alert_subscriptions a ORDER BY id`),
		"prefs":         eraseDump(t, pool, `SELECT p::text FROM notification_prefs p ORDER BY user_id`),
	}
}

func eraseCount(t *testing.T, pool *pgxpool.Pool, table, user string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE user_id = $1`, user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func eraseOK(t *testing.T, e *store.Eraser, user string) int64 {
	t.Helper()
	n, err := e.Erase(context.Background(), user)
	if err != nil {
		t.Fatalf("Erase(%s): %v", user, err)
	}
	return n
}

// Scenario: The user's notification data is deleted
func TestEraseDeletesAllRowsOfTheUser(t *testing.T) {
	f := seedErase(t)
	if n := eraseOK(t, f.erase, eraseVictim); n != 7 {
		t.Fatalf("Erase returned %d, want 7 rows (4 notifications + 2 alerts + 1 prefs)", n)
	}
	for _, tbl := range []string{"notifications", "alert_subscriptions", "notification_prefs"} {
		if n := eraseCount(t, f.pool, tbl, eraseVictim); n != 0 {
			t.Errorf("%s keeps %d rows of the erased user", tbl, n)
		}
	}
}

// Scenario: Other users' notifications are untouched
func TestEraseLeavesOtherUsersUntouched(t *testing.T) {
	f := seedErase(t)
	ctx := context.Background()
	q := map[string]string{
		"notifications":       `SELECT n::text FROM notifications n WHERE user_id = $1 ORDER BY id`,
		"alert_subscriptions": `SELECT a::text FROM alert_subscriptions a WHERE user_id = $1 ORDER BY id`,
		"notification_prefs":  `SELECT p::text FROM notification_prefs p WHERE user_id = $1`,
	}
	before := map[string][]string{}
	for k, s := range q {
		before[k] = eraseDump(t, f.pool, s, eraseBystander)
	}
	eraseOK(t, f.erase, eraseVictim)
	for k, s := range q {
		if after := eraseDump(t, f.pool, s, eraseBystander); !reflect.DeepEqual(before[k], after) {
			t.Errorf("%s of the bystander changed:\n%v\n%v", k, before[k], after)
		}
	}
	got, total, err := f.notes.ListNotifications(ctx, eraseBystander, 10, 0)
	if err != nil || len(got) != 2 || total != 2 {
		t.Fatalf("bystander list: n=%d total=%d err=%v, want 2/2", len(got), total, err)
	}
}

// Scenario: A user with no notification data is a no-op
func TestEraseUnknownUserIsNoOp(t *testing.T) {
	f := seedErase(t)
	before := eraseSnapshot(t, f.pool)
	n, err := f.erase.Erase(context.Background(), "user-with-no-data")
	if err != nil {
		t.Fatalf("Erase of an unknown user returned %v, want nil", err)
	}
	if n != 0 {
		t.Fatalf("Erase returned %d for an unknown user, want 0", n)
	}
	if after := eraseSnapshot(t, f.pool); !reflect.DeepEqual(before, after) {
		t.Fatal("erasing an unknown user changed rows")
	}
}

// Scenario: A replayed event changes nothing
func TestEraseReplayChangesNothing(t *testing.T) {
	f := seedErase(t)
	eraseOK(t, f.erase, eraseVictim)
	first := eraseSnapshot(t, f.pool)
	if n := eraseOK(t, f.erase, eraseVictim); n != 0 {
		t.Fatalf("second delivery returned %d, want 0", n)
	}
	if second := eraseSnapshot(t, f.pool); !reflect.DeepEqual(first, second) {
		t.Fatalf("replay changed state:\n%v\n%v", first, second)
	}
}

// Scenario: A malformed event goes to the DLQ (an empty user id erases nothing)
func TestEraseEmptyUserIDRefusedAndChangesNothing(t *testing.T) {
	f := seedErase(t)
	before := eraseSnapshot(t, f.pool)
	if _, err := f.erase.Erase(context.Background(), ""); err == nil {
		t.Fatal("Erase with an empty user id must fail")
	}
	if after := eraseSnapshot(t, f.pool); !reflect.DeepEqual(before, after) {
		t.Fatal("empty user id changed rows")
	}
}

// Scenario: A store failure rolls back and is retried
func TestEraseRollsBackOnFailure(t *testing.T) {
	f := seedErase(t)
	ctx := context.Background()
	exec := func(q string) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE FUNCTION fail_erase() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`)
	exec(`CREATE TRIGGER fail_erase_prefs BEFORE DELETE ON notification_prefs FOR EACH ROW EXECUTE FUNCTION fail_erase()`)
	exec(`CREATE TRIGGER fail_erase_alerts BEFORE DELETE ON alert_subscriptions FOR EACH ROW EXECUTE FUNCTION fail_erase()`)
	before := eraseSnapshot(t, f.pool)
	_, err := f.erase.Erase(ctx, eraseVictim)
	if err == nil {
		t.Fatal("Erase must return the store failure so the record is retried")
	}
	if !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("error does not carry the cause: %v", err)
	}
	if after := eraseSnapshot(t, f.pool); !reflect.DeepEqual(before, after) {
		t.Fatalf("partial deletion survived the failure:\nbefore %v\nafter  %v", before, after)
	}
	exec(`DROP TRIGGER fail_erase_prefs ON notification_prefs`)
	exec(`DROP TRIGGER fail_erase_alerts ON alert_subscriptions`)
	eraseOK(t, f.erase, eraseVictim)
	for _, tbl := range []string{"notifications", "alert_subscriptions", "notification_prefs"} {
		if n := eraseCount(t, f.pool, tbl, eraseVictim); n != 0 {
			t.Errorf("retry after recovery left %d rows in %s", n, tbl)
		}
	}
}
