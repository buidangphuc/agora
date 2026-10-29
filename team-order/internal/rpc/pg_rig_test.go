package rpc_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-order/internal/store"
)

const outboxTable = "order_outbox_events"

type pgOutbox struct {
	t     testing.TB
	pool  *pgxpool.Pool
	store *outbox.Store
}

func newPGOutbox(t testing.TB) *pgOutbox {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrate.Up(t.Context(), dsn, os.DirFS("../../migrations")); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	st, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return &pgOutbox{t: t, pool: pool, store: st}
}

func (p *pgOutbox) orders(opts ...store.OrderRepoOption) *store.PostgresOrderRepository {
	return store.NewPostgresOrderRepository(p.pool, p.store, opts...)
}

func (p *pgOutbox) returns(opts ...store.ReturnRepoOption) *store.PostgresReturnRepository {
	return store.NewPostgresReturnRepository(p.pool, p.store, opts...)
}

func (p *pgOutbox) shipments(opts ...store.ShipmentRepoOption) *store.PostgresShipmentRepository {
	return store.NewPostgresShipmentRepository(p.pool, p.store, opts...)
}

func (p *pgOutbox) sagas() *store.PostgresSagaRepository {
	return store.NewPostgresSagaRepository(p.pool)
}

func (p *pgOutbox) EnqueuedRows() []outbox.Row {
	p.t.Helper()
	rs, err := p.pool.Query(p.t.Context(),
		`SELECT event_id, aggregate_type, aggregate_id, event_type, payload FROM `+outboxTable+` ORDER BY seq`)
	if err != nil {
		p.t.Fatalf("read outbox: %v", err)
	}
	defer rs.Close()
	var out []outbox.Row
	for rs.Next() {
		var r outbox.Row
		if err := rs.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &r.EventType, &r.Payload); err != nil {
			p.t.Fatalf("scan outbox: %v", err)
		}
		out = append(out, r)
	}
	if err := rs.Err(); err != nil {
		p.t.Fatalf("outbox rows: %v", err)
	}
	return out
}
