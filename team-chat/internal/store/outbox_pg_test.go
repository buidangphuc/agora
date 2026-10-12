package store

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
)

// orderRecorder records the payloads in the order the relayer publishes them.
type orderRecorder struct {
	mu       sync.Mutex
	payloads []string
}

func (r *orderRecorder) Publish(_ context.Context, _ string, _, value []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payloads = append(r.payloads, string(value))
	return nil
}

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
	}{{"evt-updated", 2 * time.Second}, {"evt-created", 0}} {
		if _, err := pool.Exec(ctx, `INSERT INTO chat_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
			VALUES ($1, 'Agg', 'agg-1', 'T', convert_to($1, 'UTF8'), '', 'pending', 0, $2, $2)`, r.id, base.Add(r.age)); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	pub := &orderRecorder{}
	relayer := outbox.NewRelayer(outbox.NewStore(pool, OutboxTable), pub, outbox.RelayerOptions{
		Topic:  "chat.events",
		Config: sdkconfig.Outbox{BatchSize: 10, ClaimLockSeconds: 30, MaxAttempts: 3},
	})
	if _, err := relayer.RelayOnce(ctx); err != nil {
		t.Fatalf("relay: %v", err)
	}
	if want := []string{"evt-created", "evt-updated"}; !slices.Equal(pub.payloads, want) {
		t.Fatalf("claim order = %v, want %v", pub.payloads, want)
	}
}
