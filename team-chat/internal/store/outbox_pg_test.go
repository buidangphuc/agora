package store

import (
	"context"
	"slices"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

// Regression: UPDATE ... RETURNING does not preserve the claim subquery's ORDER BY,
// so two events for one aggregate in the same batch could be relayed newest-first
// (a consumer diffing snapshots then misses a price drop). Rows are inserted so the
// physical order is the reverse of created_at; the claim must still return them
// oldest-first, which shows as the order the SDK relayer publishes them in. Runs
// against a testkit Postgres container.
func TestClaimPendingKeepsCreationOrder_Postgres(t *testing.T) {
	ctx := context.Background()
	pool := chatPool(t)

	base := time.Now().Add(-time.Minute)
	// Inserted newer-first so heap order disagrees with created_at.
	for _, r := range []struct {
		id  string
		age time.Duration
		seq int // creation order, as the v1 default would number them
	}{{"evt-updated", 2 * time.Second, 2}, {"evt-created", 0, 1}} {
		if _, err := pool.Exec(ctx, `INSERT INTO chat_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at, seq)
			VALUES ($1, 'Agg', 'agg-1', 'T', convert_to($1, 'UTF8'), '', 'pending', 0, $2, $2, $3)`, r.id, base.Add(r.age), r.seq); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	pub := testkit.NewRecorder()
	ob, err := outbox.NewStore(pool, OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	relayer, err := outbox.NewRelayer(ob, pub, outbox.RelayerOptions{
		Topic:  "chat.events",
		Config: sdkconfig.Outbox{BatchSize: 10, ClaimLock: 30 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relayer.RelayOnce(ctx); err != nil {
		t.Fatalf("relay: %v", err)
	}
	var got []string
	for _, r := range pub.Records() {
		got = append(got, string(r.Value))
	}
	if want := []string{"evt-created", "evt-updated"}; !slices.Equal(got, want) {
		t.Fatalf("claim order = %v, want %v", got, want)
	}
}
