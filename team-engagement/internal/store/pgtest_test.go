package store_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	"github.com/buidangphuc/team-engagement/migrations"
)

const (
	factsTable  = "engagement_outbox_events"
	legacyTable = "outbox"
)

var migrationPrefix = regexp.MustCompile(`^(\d{4})_`)

func schemaURL(t *testing.T) string {
	t.Helper()
	return testkit.Schema(t, testkit.SharedPostgres(t))
}

func openPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func emptyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return openPool(t, schemaURL(t))
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := schemaURL(t)
	if err := migrate.Up(context.Background(), url, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return openPool(t, url)
}

func testOutbox(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	st, err := outbox.NewStore(pool, factsTable)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func upFiles(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") && migrationPrefix.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func applyUp(t *testing.T, pool *pgxpool.Pool, pick func(name string) bool) {
	t.Helper()
	for _, name := range upFiles(t) {
		if !pick(name) {
			continue
		}
		runMigrationFile(t, pool, name)
	}
}

func runMigrationFile(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	b, err := fs.ReadFile(migrations.FS, name)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	if _, err := pool.Exec(context.Background(), string(b)); err != nil {
		t.Fatalf("run %s: %v", filepath.Base(name), err)
	}
}

func upTo(n int) func(string) bool {
	return func(name string) bool {
		m := migrationPrefix.FindStringSubmatch(name)
		v, _ := strconv.Atoi(m[1])
		return v <= n
	}
}

func callerCtx() context.Context {
	return auth.WithPrincipal(context.Background(),
		auth.Principal{ID: "buyer-1", Type: auth.TypeUser, Scopes: []string{"engagement:write"}})
}

func adminCtx(id string) context.Context {
	return auth.WithPrincipal(context.Background(),
		auth.Principal{ID: id, Type: auth.TypeUser, Scopes: []string{"engagement:admin"}})
}

func callerCtxWithRequest(id string) context.Context {
	return reqctx.WithRequestID(callerCtx(), id)
}

type factRow struct {
	aggregateType string
	aggregateID   string
	eventType     string
	env           *eventsv1.EventEnvelope
}

func factRows(t *testing.T, pool *pgxpool.Pool) []factRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT aggregate_type, aggregate_id, event_type, payload FROM `+factsTable+` ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []factRow
	for rows.Next() {
		var f factRow
		var raw []byte
		if err := rows.Scan(&f.aggregateType, &f.aggregateID, &f.eventType, &raw); err != nil {
			t.Fatal(err)
		}
		f.env = &eventsv1.EventEnvelope{}
		if err := proto.Unmarshal(raw, f.env); err != nil {
			t.Fatalf("row payload is not an EventEnvelope: %v", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func factsOfType(t *testing.T, pool *pgxpool.Pool, typ string) []factRow {
	t.Helper()
	var out []factRow
	for _, f := range factRows(t, pool) {
		if f.eventType == typ {
			out = append(out, f)
		}
	}
	return out
}

func factTypes(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	var out []string
	for _, f := range factRows(t, pool) {
		out = append(out, strings.TrimPrefix(f.eventType, "platform.engagement.v1."))
	}
	return out
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
