package events_test

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/store"
)

const injectFailureSQL = `
CREATE FUNCTION fail_notification_insert() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'injected insert failure'; END
$$ LANGUAGE plpgsql;
CREATE TRIGGER fail_notification_insert BEFORE INSERT ON notifications
FOR EACH ROW EXECUTE FUNCTION fail_notification_insert();`

func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	sub, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	if err := migrate.Up(context.Background(), url, sub); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url, MaxConns: 5})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func failInserts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), injectFailureSQL); err != nil {
		t.Fatalf("inject failure: %v", err)
	}
}

func healInserts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DROP TRIGGER fail_notification_insert ON notifications`); err != nil {
		t.Fatalf("heal inserts: %v", err)
	}
}

func envelope(t *testing.T, eventID, typ string, payload []byte) []byte {
	t.Helper()
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, Payload: payload})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return raw
}

func envelopeOf(t *testing.T, eventID, typ string, m proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return envelope(t, eventID, typ, payload)
}

func record(raw []byte) consume.Record {
	return consume.Record{Topic: "events", Key: []byte("k"), Value: raw}
}

func deliver(h consume.Handler, raw []byte) error {
	return h(context.Background(), record(raw))
}

func mustDeliver(t *testing.T, h consume.Handler, raw []byte) {
	t.Helper()
	if err := deliver(h, raw); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func isPermanent(err error) bool {
	for ; err != nil; err = errors.Unwrap(err) {
		if reflect.TypeOf(err).Name() == "permanentError" {
			return true
		}
	}
	return false
}

func requirePermanent(t *testing.T, err error) {
	t.Helper()
	if err == nil || !isPermanent(err) {
		t.Fatalf("want a consume.Permanent error, got %v", err)
	}
}

func requireTransient(t *testing.T, err error) {
	t.Helper()
	if err == nil || isPermanent(err) {
		t.Fatalf("want a transient (non-permanent) error, got %v", err)
	}
}

func notificationsOf(t *testing.T, pool *pgxpool.Pool, user string) []*notificationv1.Notification {
	t.Helper()
	got, _, err := store.NewNotifications(pool).ListNotifications(context.Background(), user, 100, 0)
	if err != nil {
		t.Fatalf("list notifications of %s: %v", user, err)
	}
	return got
}

func totalNotifications(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications`).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return n
}

func ledgerRows(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM processed_events WHERE event_id = $1`, eventID).Scan(&n); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return n
}

func one(t *testing.T, pool *pgxpool.Pool, user string) *notificationv1.Notification {
	t.Helper()
	got := notificationsOf(t, pool, user)
	if len(got) != 1 {
		t.Fatalf("user %q: want exactly 1 notification, got %d (total %d)", user, len(got), totalNotifications(t, pool))
	}
	return got[0]
}

func disable(t *testing.T, pool *pgxpool.Pool, user string, typ notificationv1.NotificationType) {
	t.Helper()
	_, err := store.NewPrefs(pool).Upsert(context.Background(), user, &notificationv1.NotificationPrefs{
		TypeEnabled: map[string]bool{typ.String(): false},
	})
	if err != nil {
		t.Fatalf("upsert prefs: %v", err)
	}
}

func subscribe(t *testing.T, pool *pgxpool.Pool, user, listing string, at notificationv1.AlertType) {
	t.Helper()
	_, err := store.NewAlerts(pool).Create(context.Background(), &notificationv1.AlertSubscription{UserId: user, ListingId: listing, Type: at})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
}

type fakeNames struct {
	name  string
	calls [][2]string
}

func (f *fakeNames) ResolveSenderName(_ context.Context, senderID, sellerID string) string {
	f.calls = append(f.calls, [2]string{senderID, sellerID})
	return f.name
}
