package store

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	"github.com/buidangphuc/team-engagement/internal/events"
	"github.com/buidangphuc/team-engagement/migrations"
)

// pgURL is the database a Postgres test runs against: TEST_DATABASE_URL when set
// (a disposable database), otherwise a fresh container from go-sdk testkit.
func pgURL(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return testkit.Postgres(t)
}

// applyMigrations runs the up files named from <= name < to ("" = unbounded).
func applyMigrations(t *testing.T, pool *pgxpool.Pool, from, to string) {
	t.Helper()
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") && e.Name() >= from && (to == "" || e.Name() < to) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := fs.ReadFile(migrations.Files, n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(b)); err != nil {
			t.Fatalf("apply %s: %v", n, err)
		}
	}
}

type recordedPublish struct {
	topic, key string
	value      []byte
}

type recordingPublisher struct {
	mu  sync.Mutex
	got []recordedPublish
}

func (p *recordingPublisher) Publish(_ context.Context, topic string, key, value []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got = append(p.got, recordedPublish{topic, string(key), append([]byte(nil), value...)})
	return nil
}

type legacyRow struct {
	eventID, typ, key string
	payload           []byte
	pid, ptype, reqID string
	occurredAt        time.Time
	published         bool
}

// Rows that were unpublished when the migration ran are relayed by the SDK relayer
// afterwards, in the old seq order, with the same event_id and an EventEnvelope
// byte-identical to the one the old relayer built at publish time. Rows already
// published are not copied, and the old table stays in place.
func TestLegacyOutboxRowsRelayedAfterMigration(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, pgURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	applyMigrations(t, pool, "", legacyUpTo)

	tie := time.Date(2026, 10, 9, 1, 2, 3, 456789000, time.UTC) // microsecond precision
	long := bytes.Repeat([]byte{0x08, 0x01}, 100)               // 200 bytes: a two-byte length varint
	legacy := []legacyRow{
		{
			eventID: "00000000-0000-0000-0000-000000000001", typ: "platform.engagement.v1.FavoriteAdded", key: "L1",
			payload: []byte("a"), pid: "buyer-1", ptype: "PRINCIPAL_TYPE_USER", reqID: "req-1", occurredAt: tie,
		},
		{
			eventID: "00000000-0000-0000-0000-000000000002", typ: "platform.engagement.v1.FavoriteRemoved", key: "L0",
			payload: []byte("done"), occurredAt: tie.Add(-time.Hour), published: true,
		},
		// seq order wins over occurred_at: this row is older but was written later.
		{
			eventID: "00000000-0000-0000-0000-000000000003", typ: "platform.engagement.v1.SellerFollowed", key: "S1",
			payload: long, occurredAt: tie.Add(-time.Hour),
		},
		{
			eventID: "00000000-0000-0000-0000-000000000004", typ: "platform.engagement.v1.ReviewCreated", key: "L2",
			payload: []byte("r"), pid: "svc", ptype: "PRINCIPAL_TYPE_SERVICE", reqID: "req-4", occurredAt: tie,
		},
		{
			eventID: "00000000-0000-0000-0000-000000000005", typ: "platform.engagement.v1.SellerUnfollowed", key: "S2",
			payload: []byte("u"), pid: "anon", ptype: "PRINCIPAL_TYPE_ANONYMOUS", occurredAt: tie.Add(time.Second),
		},
	}
	for _, r := range legacy {
		var pub *time.Time
		if r.published {
			now := time.Now()
			pub = &now
		}
		if _, err := pool.Exec(ctx, `INSERT INTO outbox
			(event_id, type, key, payload, principal_id, principal_type, request_id, occurred_at, published_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			r.eventID, r.typ, r.key, r.payload, r.pid, r.ptype, r.reqID, r.occurredAt, pub); err != nil {
			t.Fatal(err)
		}
	}

	applyMigrations(t, pool, legacyUpTo, "")

	var oldRows, copied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&oldRows); err != nil || oldRows != len(legacy) {
		t.Fatalf("old outbox has %d rows (%v), want it left in place with all %d", oldRows, err, len(legacy))
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+OutboxTable).Scan(&copied); err != nil || copied != 4 {
		t.Fatalf("%d rows copied (%v), want the 4 unpublished ones", copied, err)
	}

	// A write after the cutover goes to the new table and is relayed after the legacy rows.
	wctx := auth.WithPrincipal(ctx, auth.Principal{ID: "buyer-9", Type: "user"})
	if _, err := NewPostgresRepository(pool).AddFavorite(wctx, "buyer-9", "L9"); err != nil {
		t.Fatal(err)
	}

	pub := &recordingPublisher{}
	relayer := outbox.NewRelayer(outbox.NewStore(pool, OutboxTable), pub, outbox.RelayerOptions{
		Topic:  "engagement.events",
		Config: sdkconfig.Outbox{PollInterval: time.Second, BatchSize: 10, ClaimLockSeconds: 60, MaxAttempts: 3},
	})
	rep, err := relayer.RelayOnce(ctx)
	if err != nil || rep.Claimed != 5 || rep.Published != 5 {
		t.Fatalf("RelayOnce = %+v, %v, want 5 claimed and published", rep, err)
	}

	wantKeys := []string{"L1", "S1", "L2", "S2", "L9"}
	if len(pub.got) != len(wantKeys) {
		t.Fatalf("published %d records, want %d", len(pub.got), len(wantKeys))
	}
	for i, k := range wantKeys {
		if pub.got[i].key != k || pub.got[i].topic != "engagement.events" {
			t.Fatalf("record %d = %s/%s, want engagement.events/%s (old seq order, new write last)", i, pub.got[i].topic, pub.got[i].key, k)
		}
	}

	unpublished := []legacyRow{legacy[0], legacy[2], legacy[3], legacy[4]}
	for i, r := range unpublished {
		var principal *commonv1.Principal
		switch r.ptype {
		case "PRINCIPAL_TYPE_USER":
			principal = &commonv1.Principal{Id: r.pid, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}
		case "PRINCIPAL_TYPE_SERVICE":
			principal = &commonv1.Principal{Id: r.pid, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE}
		case "PRINCIPAL_TYPE_ANONYMOUS":
			principal = &commonv1.Principal{Id: r.pid, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS}
		}
		want, err := events.BuildEnvelope(r.eventID, r.typ, r.occurredAt, principal, r.reqID, r.payload)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(pub.got[i].value, want) {
			t.Fatalf("legacy row %s: envelope differs from the one the old relayer built\n got %x\nwant %x", r.eventID, pub.got[i].value, want)
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(pub.got[i].value, &env); err != nil {
			t.Fatal(err)
		}
		if env.GetEventId() != r.eventID || !env.GetOccurredAt().AsTime().Equal(r.occurredAt) {
			t.Fatalf("legacy row %s: envelope = %v", r.eventID, &env)
		}
	}

	// Nothing is left to relay, and the published legacy row never came across.
	if rep, err := relayer.RelayOnce(ctx); err != nil || rep.Claimed != 0 {
		t.Fatalf("second RelayOnce = %+v, %v, want nothing claimed", rep, err)
	}
	var ghost int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+OutboxTable+` WHERE event_id = $1`, legacy[1].eventID).Scan(&ghost); err != nil || ghost != 0 {
		t.Fatalf("the already published row was copied (%d, %v)", ghost, err)
	}
	// Re-running the copy must not duplicate events.
	var again int
	b, err := fs.ReadFile(migrations.Files, legacyUpTo+"_engagement_outbox_events.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(b)); err != nil {
		t.Fatalf("re-applying the migration: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+OutboxTable).Scan(&again); err != nil || again != 5 {
		t.Fatalf("after re-applying, %d rows (%v), want 5", again, err)
	}
}

// legacyUpTo is the first migration of the SDK-shaped outbox.
const legacyUpTo = "0011"

// The deploy job applies the migrations with golang-migrate, not by running the
// files by hand: the same files must apply that way, and the copy must work on a
// database that already holds unpublished rows.
func TestMigrationsApplyWithGoMigrateOverLegacyRows(t *testing.T) {
	ctx := context.Background()
	url := pgURL(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	applyMigrations(t, pool, "", legacyUpTo)
	if _, err := pool.Exec(ctx, `INSERT INTO outbox (event_id, type, key, payload) VALUES
		('00000000-0000-0000-0000-0000000000a1', 'platform.engagement.v1.FavoriteAdded', 'L1', '\x0a01')`); err != nil {
		t.Fatal(err)
	}

	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+OutboxTable+` WHERE status = 'pending'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pending copied rows = %d, %v, want 1", n, err)
	}
}
