package store_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

const outboxTable = "payment_outbox_events"

func migrationFiles(t *testing.T, from, to int) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v (n=%d)", err, len(files))
	}
	sort.Strings(files)
	var out []string
	for _, f := range files {
		n, err := strconv.Atoi(strings.SplitN(filepath.Base(f), "_", 2)[0])
		if err != nil {
			t.Fatalf("migration %s has no numeric prefix", f)
		}
		if n >= from && n <= to {
			out = append(out, f)
		}
	}
	return out
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool, from, to int) {
	t.Helper()
	for _, f := range migrationFiles(t, from, to) {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(context.Background(), string(body)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
}

func schemaPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := schemaPool(t)
	applyMigrations(t, pool, 1, 17)
	return pool
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func newOutbox(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	s, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return s
}

func newRelayer(t *testing.T, s *outbox.Store, pub outbox.Publisher) *outbox.Relayer {
	t.Helper()
	r, err := outbox.NewRelayer(s, pub, outbox.RelayerOptions{
		Topic:       "payment.events",
		Config:      config.Outbox{BatchSize: 100},
		BaseBackoff: 1,
		MaxBackoff:  2,
	})
	if err != nil {
		t.Fatalf("relayer: %v", err)
	}
	return r
}

func relayUntilQuiet(t *testing.T, r *outbox.Relayer, passes int) {
	t.Helper()
	for i := 0; i < passes; i++ {
		rep, err := r.RelayOnce(context.Background())
		if err != nil {
			t.Fatalf("relay pass %d: %v", i, err)
		}
		if rep.Claimed == 0 {
			return
		}
	}
}

func makeDue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	mustExec(t, pool, `UPDATE `+outboxTable+` SET available_at = now() - interval '1 second', locked_until = NULL WHERE status = 'pending'`)
}

type scriptedPublisher struct {
	mu      sync.Mutex
	calls   map[string]int
	order   []string
	failFn  func(value string, call int) error
	publish []string
}

func newScriptedPublisher(failFn func(value string, call int) error) *scriptedPublisher {
	return &scriptedPublisher{calls: map[string]int{}, failFn: failFn}
}

func (p *scriptedPublisher) PublishRecord(_ context.Context, rec *kgo.Record) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := string(rec.Value)
	p.calls[v]++
	if p.failFn != nil {
		if err := p.failFn(v, p.calls[v]); err != nil {
			return err
		}
	}
	p.publish = append(p.publish, v)
	return nil
}

func (p *scriptedPublisher) published() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.publish...)
}

func (p *scriptedPublisher) callCount(v string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[v]
}

func applyFile(t *testing.T, pool *pgxpool.Pool, file string) {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if _, err := pool.Exec(context.Background(), string(body)); err != nil {
		t.Fatalf("apply %s: %v", filepath.Base(file), err)
	}
}
