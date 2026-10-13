package store

import (
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-payment/internal/pgtest"
)

// Regression: UPDATE ... RETURNING does not preserve the claim subquery's ORDER BY, and a
// retried row is rewritten (it moves in the heap), so two events for one aggregate in the same
// batch could be relayed newest-first (a consumer diffing snapshots then misses a price drop).
// The first event is updated after both are enqueued, so its physical position is after the
// second; the relayer must still publish them oldest-first (v1 orders by seq). Runs against a
// disposable Postgres (pgtest).
func TestClaimPendingKeepsCreationOrder_Postgres(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := context.Background()
	st, err := outbox.NewStore(pool, "payment_outbox_events")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"evt-created", "evt-updated"} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		row := outbox.Row{EventID: id, AggregateType: "Agg", AggregateID: "agg-1", EventType: "T", Payload: []byte(id)}
		if err := st.EnqueueTx(ctx, tx, row); err != nil {
			t.Fatalf("enqueue %s: %v", id, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// The retry: rewrites the first row so it now sits after the second in the heap.
	if _, err := pool.Exec(ctx, `UPDATE payment_outbox_events SET attempts = 1, available_at = now() WHERE event_id = 'evt-created'`); err != nil {
		t.Fatal(err)
	}

	pub := &orderPublisher{}
	rl, err := outbox.NewRelayer(st, pub, outbox.RelayerOptions{Topic: "payment.events"})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := rl.RelayOnce(ctx)
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

func (p *orderPublisher) PublishRecord(_ context.Context, r *kgo.Record) error {
	p.keys = append(p.keys, string(r.Value))
	return nil
}
