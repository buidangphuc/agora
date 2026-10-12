package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-payment/internal/pgtest"
)

// Regression: UPDATE ... RETURNING does not preserve the claim subquery's ORDER BY,
// so two events for one aggregate in the same batch could be relayed newest-first
// (a consumer diffing snapshots then misses a price drop). Rows are inserted so the
// physical order is the reverse of created_at; the claim must still return them
// oldest-first. Runs against a disposable Postgres (pgtest).
func TestClaimPendingKeepsCreationOrder_Postgres(t *testing.T) {
	dsn := pgtest.URL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	ddl, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0003_payment_outbox.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM payment_outbox_events`); err != nil {
		t.Fatalf("reset table: %v", err)
	}

	base := time.Now().Add(-time.Minute)
	// Inserted newer-first so heap order disagrees with created_at.
	for _, r := range []struct {
		id  string
		age time.Duration
	}{{"evt-updated", 2 * time.Second}, {"evt-created", 0}} {
		if _, err := pool.Exec(ctx, `INSERT INTO payment_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
			VALUES ($1, 'Agg', 'agg-1', 'T', convert_to($1, 'UTF8'), '', 'pending', 0, $2, $2)`, r.id, base.Add(r.age)); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	pub := &orderPublisher{}
	rep, err := outbox.NewRelayer(outbox.NewStore(pool, "payment_outbox_events"), pub, outbox.RelayerOptions{Topic: "payment.events"}).RelayOnce(ctx)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	got := pub.keys
	if rep.Published != 2 || len(got) != 2 || got[0] != "evt-created" || got[1] != "evt-updated" {
		t.Fatalf("relay order = %v (report %+v), want [evt-created evt-updated]", got, rep)
	}
}

// orderPublisher records the payload each row was published with, in order.
type orderPublisher struct{ keys []string }

func (p *orderPublisher) Publish(_ context.Context, _ string, _, value []byte) error {
	p.keys = append(p.keys, string(value))
	return nil
}
