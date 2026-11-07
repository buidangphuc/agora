package migrations_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	"github.com/buidangphuc/team-engagement/migrations"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

const (
	upFile   = "0014_engagement_outbox_events.up.sql"
	downFile = "0014_engagement_outbox_events.down.sql"
	newTable = "engagement_outbox_events"
)

var versionPrefix = regexp.MustCompile(`^(\d{4})_`)

func upTo(t *testing.T, maxVersion int) fs.FS {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	out := fstest.MapFS{}
	for _, e := range entries {
		m := versionPrefix.FindStringSubmatch(e.Name())
		if m == nil || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		if v > maxVersion {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out[e.Name()] = &fstest.MapFile{Data: b}
	}
	return out
}

type env struct {
	url  string
	pool *pgxpool.Pool
}

func setup(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(ctx, url, upTo(t, 13)); err != nil {
		t.Fatalf("migrate up to 0013: %v", err)
	}
	pool, err := postgres.Open(ctx, config.Postgres{URL: url, MaxConns: 4})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return env{url: url, pool: pool}
}

func (e env) apply0014(t *testing.T) {
	t.Helper()
	if err := migrate.Up(context.Background(), e.url, os.DirFS(".")); err != nil {
		t.Fatalf("apply 0014: %v", err)
	}
}

type legacy struct {
	seq           int64
	eventID       string
	typ           string
	key           string
	payload       []byte
	principalID   string
	principalType string
	requestID     string
	occurredAt    time.Time
	published     bool
}

func (e env) seed(t *testing.T, rows ...legacy) {
	t.Helper()
	for _, r := range rows {
		var pub any
		if r.published {
			pub = r.occurredAt.Add(time.Minute)
		}
		_, err := e.pool.Exec(context.Background(), `
INSERT INTO outbox (seq, event_id, type, key, payload, principal_id, principal_type, request_id, occurred_at, published_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			r.seq, r.eventID, r.typ, r.key, r.payload, r.principalID, r.principalType, r.requestID, r.occurredAt, pub)
		if err != nil {
			t.Fatalf("seed legacy row %d: %v", r.seq, err)
		}
	}
}

func (e env) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func (e env) exists(t *testing.T, table string) bool {
	t.Helper()
	var ok bool
	if err := e.pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok); err != nil {
		t.Fatalf("to_regclass %s: %v", table, err)
	}
	return ok
}

func at(sec int) time.Time {
	return time.Date(2026, 10, 1, 12, 0, sec, 123456000, time.UTC)
}

func uuidFor(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

func fact(t *testing.T, name string) []byte {
	t.Helper()
	b, err := proto.Marshal(timestamppb.New(at(0)))
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return append([]byte(name), b...)
}

type copied struct {
	eventID       string
	aggregateType string
	aggregateID   string
	eventType     string
	payload       []byte
	requestID     string
	status        string
	attempts      int
	seq           int64
	availableAt   time.Time
}

func (e env) copiedRows(t *testing.T) []copied {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `
SELECT event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, seq, available_at
FROM `+newTable+` ORDER BY seq`)
	if err != nil {
		t.Fatalf("select copied: %v", err)
	}
	defer rows.Close()
	var out []copied
	for rows.Next() {
		var c copied
		if err := rows.Scan(&c.eventID, &c.aggregateType, &c.aggregateID, &c.eventType, &c.payload, &c.requestID, &c.status, &c.attempts, &c.seq, &c.availableAt); err != nil {
			t.Fatalf("scan copied: %v", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate copied: %v", err)
	}
	return out
}

func wantEnvelope(l legacy, ptype commonv1.PrincipalType) *eventsv1.EventEnvelope {
	env := &eventsv1.EventEnvelope{
		EventId:    l.eventID,
		Type:       l.typ,
		OccurredAt: timestamppb.New(l.occurredAt),
		RequestId:  l.requestID,
		Payload:    l.payload,
	}
	if l.principalID != "" || l.principalType != "" {
		env.Principal = &commonv1.Principal{Id: l.principalID, Type: ptype}
	}
	return env
}

func assertEnvelope(t *testing.T, got []byte, want *eventsv1.EventEnvelope) {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(got, &env); err != nil {
		t.Fatalf("payload is not an EventEnvelope: %v", err)
	}
	if env.GetEventId() != want.GetEventId() {
		t.Errorf("envelope event id = %q, want %q", env.GetEventId(), want.GetEventId())
	}
	if env.GetType() != want.GetType() {
		t.Errorf("envelope type = %q, want %q", env.GetType(), want.GetType())
	}
	if env.GetRequestId() != want.GetRequestId() {
		t.Errorf("envelope request id = %q, want %q", env.GetRequestId(), want.GetRequestId())
	}
	if string(env.GetPayload()) != string(want.GetPayload()) {
		t.Errorf("envelope payload = %x, want %x", env.GetPayload(), want.GetPayload())
	}
	if env.GetOccurredAt() == nil || !env.GetOccurredAt().AsTime().Equal(want.GetOccurredAt().AsTime()) {
		t.Errorf("envelope occurred_at = %v, want %v", env.GetOccurredAt().AsTime(), want.GetOccurredAt().AsTime())
	}
	if want.GetPrincipal() == nil {
		if env.GetPrincipal() != nil {
			t.Errorf("envelope principal = %v, want none", env.GetPrincipal())
		}
		return
	}
	if env.GetPrincipal().GetId() != want.GetPrincipal().GetId() || env.GetPrincipal().GetType() != want.GetPrincipal().GetType() {
		t.Errorf("envelope principal = %v, want %v", env.GetPrincipal(), want.GetPrincipal())
	}
}

func threeUnpublished(t *testing.T) []legacy {
	return []legacy{
		{
			seq: 5, eventID: uuidFor(5), typ: "platform.engagement.v1.FavoriteAdded", key: "L1", payload: fact(t, "a"),
			principalID: "u-1", principalType: "PRINCIPAL_TYPE_USER", requestID: "r-5", occurredAt: at(5),
		},
		{
			seq: 6, eventID: uuidFor(6), typ: "platform.engagement.v1.SellerFollowed", key: "s-1", payload: fact(t, "b"),
			principalID: "u-2", principalType: "PRINCIPAL_TYPE_USER", requestID: "", occurredAt: at(6),
		},
		{
			seq: 7, eventID: uuidFor(7), typ: "platform.engagement.v1.DisputeResolved", key: "d-1", payload: fact(t, "c"),
			principalID: "", principalType: "", requestID: "r-7", occurredAt: at(7),
		},
	}
}

func TestMigration0014CopiesUnpublishedFacts(t *testing.T) {
	e := setup(t)
	un := threeUnpublished(t)
	published := []legacy{
		{
			seq: 3, eventID: uuidFor(3), typ: "platform.engagement.v1.FavoriteRemoved", key: "L0", payload: fact(t, "p3"),
			principalID: "u-9", principalType: "PRINCIPAL_TYPE_USER", requestID: "r-3", occurredAt: at(3), published: true,
		},
		{
			seq: 4, eventID: uuidFor(4), typ: "platform.engagement.v1.ReviewCreated", key: "L0", payload: fact(t, "p4"),
			principalID: "u-9", principalType: "PRINCIPAL_TYPE_USER", requestID: "r-4", occurredAt: at(4), published: true,
		},
	}
	e.seed(t, published[0], un[2], published[1], un[0], un[1])
	e.apply0014(t)

	got := e.copiedRows(t)
	if len(got) != 3 {
		t.Fatalf("Engagement SDK: Unpublished legacy facts are copied in order: copied %d rows, want 3", len(got))
	}
	wantAgg := []string{"Listing", "Seller", "Dispute"}
	for i, l := range un {
		c := got[i]
		if c.eventID != l.eventID {
			t.Errorf("row %d event id = %s, want %s (claim order must follow legacy seq)", i, c.eventID, l.eventID)
		}
		if c.aggregateType != wantAgg[i] || c.aggregateID != l.key {
			t.Errorf("row %d aggregate = %s/%s, want %s/%s", i, c.aggregateType, c.aggregateID, wantAgg[i], l.key)
		}
		if c.eventType != l.typ {
			t.Errorf("row %d event type = %s, want %s", i, c.eventType, l.typ)
		}
		if c.status != "pending" || c.attempts != 0 {
			t.Errorf("row %d status/attempts = %s/%d, want pending/0", i, c.status, c.attempts)
		}
		if c.requestID != l.requestID {
			t.Errorf("row %d request id = %q, want %q", i, c.requestID, l.requestID)
		}
		ptype := commonv1.PrincipalType_PRINCIPAL_TYPE_USER
		assertEnvelope(t, c.payload, wantEnvelope(l, ptype))
		if i > 0 && !c.availableAt.After(got[i-1].availableAt) {
			t.Errorf("row %d available_at %v must be strictly after row %d %v", i, c.availableAt, i-1, got[i-1].availableAt)
		}
	}
	if n := e.count(t, "outbox"); n != 5 {
		t.Errorf("legacy outbox holds %d rows, want all 5 untouched", n)
	}
}

func TestMigration0014MapsAggregatesAndPrincipalTypes(t *testing.T) {
	e := setup(t)
	rows := []legacy{
		{
			seq: 1, eventID: uuidFor(1), typ: "platform.engagement.v1.SellerUnfollowed", key: "s-2", payload: fact(t, "1"),
			principalID: "u-1", principalType: "PRINCIPAL_TYPE_USER", occurredAt: at(1),
		},
		{
			seq: 2, eventID: uuidFor(2), typ: "platform.engagement.v1.ReviewHidden", key: "L2", payload: fact(t, "2"),
			principalID: "svc-mod", principalType: "PRINCIPAL_TYPE_SERVICE", occurredAt: at(2),
		},
		{
			seq: 3, eventID: uuidFor(3), typ: "platform.engagement.v1.ReviewCreated", key: "L3", payload: fact(t, "3"),
			principalID: "anon-1", principalType: "PRINCIPAL_TYPE_ANONYMOUS", occurredAt: at(3),
		},
		{
			seq: 4, eventID: uuidFor(4), typ: "platform.engagement.v1.FavoriteRemoved", key: "L4", payload: fact(t, "4"),
			principalID: "u-4", principalType: "PRINCIPAL_TYPE_USER", occurredAt: at(4),
		},
	}
	e.seed(t, rows...)
	e.apply0014(t)

	got := e.copiedRows(t)
	if len(got) != len(rows) {
		t.Fatalf("Engagement SDK: Unpublished legacy facts are copied in order: copied %d rows, want %d", len(got), len(rows))
	}
	wantAgg := []string{"Seller", "Listing", "Listing", "Listing"}
	wantType := []commonv1.PrincipalType{
		commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE,
		commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS,
		commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
	}
	for i, l := range rows {
		if got[i].aggregateType != wantAgg[i] || got[i].aggregateID != l.key {
			t.Errorf("%s aggregate = %s/%s, want %s/%s", l.typ, got[i].aggregateType, got[i].aggregateID, wantAgg[i], l.key)
		}
		assertEnvelope(t, got[i].payload, wantEnvelope(l, wantType[i]))
	}
}

func TestMigration0014CopyIsIdempotent(t *testing.T) {
	e := setup(t)
	e.seed(t, threeUnpublished(t)...)
	e.apply0014(t)
	before := e.copiedRows(t)
	if len(before) != 3 {
		t.Fatalf("Engagement SDK: Copy is idempotent: first run copied %d rows, want 3", len(before))
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE `+newTable+` SET status = 'published', published_at = now() WHERE event_id = $1`, uuidFor(5)); err != nil {
		t.Fatalf("mark published: %v", err)
	}
	body, err := os.ReadFile(upFile)
	if err != nil {
		t.Fatalf("read %s: %v", upFile, err)
	}
	if _, err := e.pool.Exec(context.Background(), string(body)); err != nil {
		t.Fatalf("Engagement SDK: Copy is idempotent: second run failed: %v", err)
	}
	after := e.copiedRows(t)
	if len(after) != len(before) {
		t.Fatalf("row count after second run = %d, want %d", len(after), len(before))
	}
	for i := range before {
		if after[i].eventID != before[i].eventID || after[i].seq != before[i].seq {
			t.Errorf("row %d changed: %s/%d -> %s/%d", i, before[i].eventID, before[i].seq, after[i].eventID, after[i].seq)
		}
	}
	if after[0].status != "published" {
		t.Errorf("a published copied row was reset to %s by the second run", after[0].status)
	}
}

func TestMigration0014EmptyLegacyOutbox(t *testing.T) {
	e := setup(t)
	e.apply0014(t)
	if !e.exists(t, newTable) {
		t.Fatalf("Engagement SDK: Empty legacy outbox: %s does not exist after 0014", newTable)
	}
	if n := e.count(t, newTable); n != 0 {
		t.Errorf("%s holds %d rows, want 0", newTable, n)
	}
}

func TestMigration0014DownKeepsLegacyOutbox(t *testing.T) {
	e := setup(t)
	e.seed(t, threeUnpublished(t)...)
	e.apply0014(t)
	if !e.exists(t, newTable) {
		t.Fatalf("Engagement SDK: Down migration keeps the legacy table: %s missing after up", newTable)
	}
	body, err := os.ReadFile(downFile)
	if err != nil {
		t.Fatalf("read %s: %v", downFile, err)
	}
	if _, err := e.pool.Exec(context.Background(), string(body)); err != nil {
		t.Fatalf("down 0014: %v", err)
	}
	if e.exists(t, newTable) {
		t.Errorf("%s still exists after down", newTable)
	}
	if !e.exists(t, "outbox") {
		t.Fatalf("legacy outbox table was dropped by down")
	}
	if n := e.count(t, "outbox"); n != 3 {
		t.Errorf("legacy outbox holds %d rows after down, want 3", n)
	}
}

func TestMigration0014RequeuesParkedRows(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	schema, err := outbox.SchemaSQL(newTable)
	if err != nil {
		t.Fatalf("schema sql: %v", err)
	}
	if _, err := e.pool.Exec(ctx, schema); err != nil {
		t.Fatalf("pre-create %s: %v", newTable, err)
	}
	_, err = e.pool.Exec(ctx, `
INSERT INTO `+newTable+` (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, available_at, locked_until, error)
VALUES ('parked-1', 'Listing', 'L1', 'platform.engagement.v1.FavoriteAdded', '\x00', 'failed', 5, now() + interval '1 day', now() + interval '1 hour', 'boom')`)
	if err != nil {
		t.Fatalf("seed parked: %v", err)
	}
	_, err = e.pool.Exec(ctx, `
INSERT INTO `+newTable+` (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, published_at)
VALUES ('done-1', 'Listing', 'L2', 'platform.engagement.v1.FavoriteAdded', '\x01', 'published', 2, '2026-10-01T12:00:00Z')`)
	if err != nil {
		t.Fatalf("seed published: %v", err)
	}
	e.apply0014(t)

	var status string
	var attempts int
	var errText, locked *string
	var availableInFuture bool
	if err := e.pool.QueryRow(ctx, `
SELECT status, attempts, error, locked_until::text, available_at > now() FROM `+newTable+` WHERE event_id = 'parked-1'`).
		Scan(&status, &attempts, &errText, &locked, &availableInFuture); err != nil {
		t.Fatalf("read parked: %v", err)
	}
	if status != "pending" || attempts != 0 || errText != nil || locked != nil || availableInFuture {
		t.Errorf("Engagement SDK: Parked rows are requeued by the migration: got status=%s attempts=%d error=%v locked=%v availableInFuture=%v",
			status, attempts, errText, locked, availableInFuture)
	}
	var pubStatus string
	var pubAttempts int
	var pubAt *time.Time
	if err := e.pool.QueryRow(ctx, `SELECT status, attempts, published_at FROM `+newTable+` WHERE event_id = 'done-1'`).Scan(&pubStatus, &pubAttempts, &pubAt); err != nil {
		t.Fatalf("read published: %v", err)
	}
	if pubStatus != "published" || pubAttempts != 2 || pubAt == nil || !pubAt.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("published row changed: status=%s attempts=%d published_at=%v", pubStatus, pubAttempts, pubAt)
	}
}

func TestMigration0014ServiceRoleKeepsGrants(t *testing.T) {
	ctx := context.Background()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	admin, err := postgres.Open(ctx, config.Postgres{URL: url, MaxConns: 4})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(admin.Close)
	var schema string
	if err := admin.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current schema: %v", err)
	}
	role := fmt.Sprintf("eng_svc_%d", time.Now().UnixNano())
	for _, stmt := range []string{
		fmt.Sprintf(`CREATE ROLE %s NOLOGIN`, role),
		fmt.Sprintf(`GRANT USAGE ON SCHEMA %s TO %s`, schema, role),
		fmt.Sprintf(`ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT ALL ON TABLES TO %s`, schema, role),
		fmt.Sprintf(`ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT ALL ON SEQUENCES TO %s`, schema, role),
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("provision role (%s): %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, fmt.Sprintf(`ALTER DEFAULT PRIVILEGES IN SCHEMA %s REVOKE ALL ON TABLES FROM %s`, schema, role))
		_, _ = admin.Exec(ctx, fmt.Sprintf(`ALTER DEFAULT PRIVILEGES IN SCHEMA %s REVOKE ALL ON SEQUENCES FROM %s`, schema, role))
		_, _ = admin.Exec(ctx, fmt.Sprintf(`REVOKE ALL ON SCHEMA %s FROM %s`, schema, role))
		_, _ = admin.Exec(ctx, fmt.Sprintf(`DROP OWNED BY %s`, role))
		_, _ = admin.Exec(ctx, fmt.Sprintf(`DROP ROLE IF EXISTS %s`, role))
	})
	if err := migrate.Up(ctx, url, upTo(t, 13)); err != nil {
		t.Fatalf("migrate up to 0013: %v", err)
	}
	if err := migrate.Up(ctx, url, os.DirFS(".")); err != nil {
		t.Fatalf("apply 0014: %v", err)
	}

	conn, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, fmt.Sprintf(`SET ROLE %s`, role)); err != nil {
		t.Fatalf("set role: %v", err)
	}
	defer func() { _, _ = conn.Exec(ctx, `RESET ROLE`) }()

	if _, err := conn.Exec(ctx, `
INSERT INTO `+newTable+` (event_id, aggregate_type, aggregate_id, event_type, payload)
VALUES ('grant-1', 'Listing', 'L1', 'platform.engagement.v1.FavoriteAdded', '\x00')`); err != nil {
		t.Errorf("Engagement SDK: Service role keeps its grants after 0014: insert into %s as service role: %v", newTable, err)
	}
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM `+newTable).Scan(&n); err != nil {
		t.Errorf("select from %s as service role: %v", newTable, err)
	} else if n != 1 {
		t.Errorf("service role sees %d rows, want 1", n)
	}
	if _, err := conn.Exec(ctx, `
INSERT INTO outbox (event_id, type, key, payload) VALUES ('00000000-0000-4000-8000-000000000099', 'platform.engagement.v1.FavoriteAdded', 'L1', '\x00')`); err != nil {
		t.Errorf("insert into legacy outbox as service role: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&n); err != nil {
		t.Errorf("select from legacy outbox as service role: %v", err)
	}
}

func TestEmbeddedFSHoldsMigration0014(t *testing.T) {
	for _, name := range []string{upFile, downFile} {
		b, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Errorf("embedded FS misses %s: %v", name, err)
			continue
		}
		if len(strings.TrimSpace(string(b))) == 0 {
			t.Errorf("embedded %s is empty", name)
		}
	}
	if _, err := fs.ReadFile(migrations.FS, "0001_engagement.up.sql"); err != nil {
		t.Errorf("embedded FS misses 0001: %v", err)
	}
}

const (
	up15File         = "0015_listing_rating_backfill.up.sql"
	down15File       = "0015_listing_rating_backfill.down.sql"
	ratingEventType  = "platform.engagement.v1.ListingRatingChanged"
	backfillIDSQL    = `md5('listing-rating-backfill:' || $1::text)::uuid::text`
	backfillReviewer = "reviewer-secret-77"
)

func setup14(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(ctx, url, upTo(t, 14)); err != nil {
		t.Fatalf("migrate up to 0014: %v", err)
	}
	pool, err := postgres.Open(ctx, config.Postgres{URL: url, MaxConns: 4})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return env{url: url, pool: pool}
}

func (e env) runFile(t *testing.T, name string) {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if _, err := e.pool.Exec(context.Background(), string(body)); err != nil {
		t.Fatalf("run %s: %v", name, err)
	}
}

type review struct {
	id       string
	listing  string
	rating   int
	verified bool
	hidden   bool
}

func (e env) seedReviews(t *testing.T, rows ...review) {
	t.Helper()
	for _, r := range rows {
		var hidden any
		if r.hidden {
			hidden = at(1)
		}
		_, err := e.pool.Exec(context.Background(), `
INSERT INTO reviews (id, listing_id, user_id, order_id, rating, verified_purchase, hidden_at, comment)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'rất tốt')`,
			r.id, r.listing, backfillReviewer, "order-"+r.id, r.rating, r.verified, hidden)
		if err != nil {
			t.Fatalf("seed review %s: %v", r.id, err)
		}
	}
}

type ratingRow struct {
	row     copied
	env     eventsv1.EventEnvelope
	payload engagementv1.ListingRatingChanged
}

func (e env) ratingRows(t *testing.T) map[string]ratingRow {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `
SELECT event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, seq, available_at
FROM `+newTable+` WHERE event_type = $1 ORDER BY seq`, ratingEventType)
	if err != nil {
		t.Fatalf("select rating rows: %v", err)
	}
	defer rows.Close()
	out := map[string]ratingRow{}
	for rows.Next() {
		var c copied
		if err := rows.Scan(&c.eventID, &c.aggregateType, &c.aggregateID, &c.eventType, &c.payload, &c.requestID, &c.status, &c.attempts, &c.seq, &c.availableAt); err != nil {
			t.Fatalf("scan rating row: %v", err)
		}
		if _, dup := out[c.aggregateID]; dup {
			t.Errorf("listing %s has more than one rating row", c.aggregateID)
		}
		rr := ratingRow{row: c}
		if err := proto.Unmarshal(c.payload, &rr.env); err != nil {
			t.Fatalf("listing %s: outbox payload is not an EventEnvelope: %v", c.aggregateID, err)
		}
		if err := proto.Unmarshal(rr.env.GetPayload(), &rr.payload); err != nil {
			t.Fatalf("listing %s: envelope payload is not a ListingRatingChanged: %v", c.aggregateID, err)
		}
		out[c.aggregateID] = rr
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rating rows: %v", err)
	}
	return out
}

func (e env) backfillID(t *testing.T, listing string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), `SELECT `+backfillIDSQL, listing).Scan(&id); err != nil {
		t.Fatalf("backfill id: %v", err)
	}
	return id
}

func TestMigration0015BackfillsRatingPerReviewedListing(t *testing.T) {
	e := setup14(t)
	e.seedReviews(t,
		review{id: "a1", listing: "LA", rating: 5, verified: true},
		review{id: "a2", listing: "LA", rating: 3, verified: true},
		review{id: "b1", listing: "LB", rating: 4, verified: true, hidden: true},
		review{id: "d1", listing: "LD", rating: 2, verified: false},
	)
	e.runFile(t, up15File)

	got := e.ratingRows(t)
	want := map[string]struct {
		count int64
		avg   float64
	}{"LA": {2, 4.0}, "LB": {0, 0}, "LD": {0, 0}}
	if len(got) != len(want) {
		t.Fatalf("Each reviewed listing gets one backfill event: %d rating rows, want %d (LC has no reviews and must get none)", len(got), len(want))
	}
	if _, ok := got["LC"]; ok {
		t.Errorf("listing without reviews got a backfill event")
	}
	for listing, w := range want {
		r, ok := got[listing]
		if !ok {
			t.Errorf("listing %s has no backfill event", listing)
			continue
		}
		if r.row.status != "pending" || r.row.attempts != 0 {
			t.Errorf("listing %s status/attempts = %s/%d, want pending/0", listing, r.row.status, r.row.attempts)
		}
		if r.row.aggregateType != "Listing" {
			t.Errorf("listing %s aggregate type = %s, want Listing", listing, r.row.aggregateType)
		}
		if r.row.eventID != e.backfillID(t, listing) {
			t.Errorf("listing %s event id = %s, want deterministic %s", listing, r.row.eventID, e.backfillID(t, listing))
		}
		if r.env.GetEventId() != r.row.eventID {
			t.Errorf("listing %s envelope event id = %q, want %q", listing, r.env.GetEventId(), r.row.eventID)
		}
		if r.env.GetType() != ratingEventType {
			t.Errorf("listing %s envelope type = %q, want %q", listing, r.env.GetType(), ratingEventType)
		}
		if r.payload.GetListingId() != listing {
			t.Errorf("listing %s payload listing id = %q", listing, r.payload.GetListingId())
		}
		if r.payload.GetReviewCount() != w.count || r.payload.GetAverageRating() != w.avg {
			t.Errorf("listing %s payload = %d/%v, want %d/%v", listing, r.payload.GetReviewCount(), r.payload.GetAverageRating(), w.count, w.avg)
		}
	}
}

func TestMigration0015PayloadDecodesAsContractType(t *testing.T) {
	e := setup14(t)
	e.seedReviews(t,
		review{id: "p1", listing: "LP", rating: 4, verified: true},
		review{id: "p2", listing: "LP", rating: 4, verified: true},
		review{id: "p3", listing: "LP", rating: 3, verified: true},
	)
	e.runFile(t, up15File)

	r, ok := e.ratingRows(t)["LP"]
	if !ok {
		t.Fatalf("Backfill payload decodes with the contract type: no row for LP")
	}
	if r.payload.GetAverageRating() != float64(11)/float64(3) {
		t.Errorf("average_rating = %v, want %v", r.payload.GetAverageRating(), float64(11)/float64(3))
	}
	if r.payload.GetReviewCount() != 3 {
		t.Errorf("review_count = %d, want 3", r.payload.GetReviewCount())
	}
	if r.payload.GetListingId() != "LP" {
		t.Errorf("listing_id = %q, want LP", r.payload.GetListingId())
	}
	if r.payload.GetChangedAt() == nil || r.payload.GetChangedAt().AsTime().IsZero() {
		t.Errorf("changed_at is missing")
	}
	if r.env.GetOccurredAt() == nil || r.env.GetOccurredAt().AsTime().IsZero() {
		t.Errorf("envelope occurred_at is missing")
	}
}

func TestMigration0015IsIdempotent(t *testing.T) {
	e := setup14(t)
	e.seedReviews(t,
		review{id: "i1", listing: "LI1", rating: 5, verified: true},
		review{id: "i2", listing: "LI2", rating: 1, verified: true},
	)
	e.runFile(t, up15File)
	before := e.ratingRows(t)
	if len(before) != 2 {
		t.Fatalf("Backfill is idempotent: first run wrote %d rating rows, want 2", len(before))
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE `+newTable+` SET status = 'published', published_at = now() WHERE aggregate_id = 'LI1'`); err != nil {
		t.Fatalf("mark published: %v", err)
	}
	e.runFile(t, up15File)
	after := e.ratingRows(t)
	if len(after) != len(before) {
		t.Fatalf("rating rows after second run = %d, want %d", len(after), len(before))
	}
	for k, b := range before {
		if after[k].row.eventID != b.row.eventID || after[k].row.seq != b.row.seq {
			t.Errorf("listing %s changed: %s/%d -> %s/%d", k, b.row.eventID, b.row.seq, after[k].row.eventID, after[k].row.seq)
		}
	}
	if after["LI1"].row.status != "published" {
		t.Errorf("a published backfill row was reset to %s by the second run", after["LI1"].row.status)
	}
}

func TestMigration0015EmptyReviews(t *testing.T) {
	e := setup14(t)
	e.runFile(t, up15File)
	if n := len(e.ratingRows(t)); n != 0 {
		t.Errorf("Empty database backfills nothing: %d rating rows, want 0", n)
	}
	if n := e.count(t, newTable); n != 0 {
		t.Errorf("%s holds %d rows, want 0", newTable, n)
	}
}

func TestMigration0015DownRemovesOnlyPendingBackfill(t *testing.T) {
	e := setup14(t)
	ctx := context.Background()
	insert := func(eventID, aggregate, status string) {
		t.Helper()
		_, err := e.pool.Exec(ctx, `
INSERT INTO `+newTable+` (event_id, aggregate_type, aggregate_id, event_type, payload, status, published_at)
VALUES ($1, 'Listing', $2, $3, '\x00', $4, CASE WHEN $4 = 'published' THEN now() END)`,
			eventID, aggregate, ratingEventType, status)
		if err != nil {
			t.Fatalf("seed %s: %v", eventID, err)
		}
	}
	publishedID := e.backfillID(t, "LPUB")
	pendingID := e.backfillID(t, "LPEND")
	liveID := uuidFor(900)
	insert(publishedID, "LPUB", "published")
	insert(pendingID, "LPEND", "pending")
	insert(liveID, "LPEND", "pending")

	e.runFile(t, down15File)

	rows := e.copiedRows(t)
	have := map[string]bool{}
	for _, r := range rows {
		have[r.eventID] = true
	}
	if have[pendingID] {
		t.Errorf("Down migration removes only pending backfill rows: pending backfill row still present")
	}
	if !have[publishedID] {
		t.Errorf("published backfill row was deleted")
	}
	if !have[liveID] {
		t.Errorf("live pending ListingRatingChanged row was deleted")
	}
	if len(rows) != 2 {
		t.Errorf("%d rows remain, want 2", len(rows))
	}
}

func TestMigration0015EnvelopeCarriesNoPrincipal(t *testing.T) {
	e := setup14(t)
	e.seedReviews(t,
		review{id: "c1", listing: "LC1", rating: 5, verified: true},
		review{id: "c2", listing: "LC2", rating: 2, verified: true, hidden: true},
	)
	e.runFile(t, up15File)

	got := e.ratingRows(t)
	if len(got) != 2 {
		t.Fatalf("Backfill envelope carries no principal: %d rating rows, want 2", len(got))
	}
	for listing, r := range got {
		if r.env.GetPrincipal() != nil {
			t.Errorf("listing %s envelope principal = %v, want none", listing, r.env.GetPrincipal())
		}
		for _, needle := range []string{backfillReviewer, "c1", "c2", "rất tốt"} {
			if strings.Contains(string(r.row.payload), needle) {
				t.Errorf("listing %s outbox payload contains %q", listing, needle)
			}
		}
	}
}

func TestEmbeddedFSHoldsMigration0015(t *testing.T) {
	for _, name := range []string{up15File, down15File} {
		b, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Errorf("embedded FS misses %s: %v", name, err)
			continue
		}
		if len(strings.TrimSpace(string(b))) == 0 {
			t.Errorf("embedded %s is empty", name)
		}
	}
}
