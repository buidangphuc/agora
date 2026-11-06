package voucher_test

import (
	"context"
	"io/fs"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/voucher"
)

type discardAudit struct{}

func (discardAudit) WriteAuditEvent(context.Context, auditx.Event) error { return nil }

func raceSvc(t *testing.T) (*voucher.Service, *pgxpool.Pool, store.VoucherRepository) {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	sub, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(t.Context(), dsn, sub); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ob, err := outbox.NewStore(pool, "promotion_outbox")
	if err != nil {
		t.Fatal(err)
	}
	vrepo := store.NewPostgresVoucherRepository(pool, ob)
	rrepo := store.NewPostgresReservationRepository(pool)
	noEvent := func(context.Context, store.Voucher) (outbox.Row, error) { return outbox.Row{}, nil }
	return voucher.New(vrepo, rrepo, auditx.NewGate(discardAudit{}, nil), noEvent, nil), pool, vrepo
}

func TestPerUserLimitRace(t *testing.T) {
	svc, pool, vrepo := raceSvc(t)
	ctx := t.Context()
	now := time.Now().UTC()
	if _, err := vrepo.Create(ctx, store.Voucher{
		Code: "RACE1", Scope: 2, DiscountType: 2, DiscountValue: 1000, Quota: 100, PerUserLimit: 1,
		Kind: store.VoucherKindStandard, Active: true, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, func(v store.Voucher) (outbox.Row, error) {
		return outbox.Row{EventID: "ev-" + v.ID, AggregateType: "voucher", AggregateID: v.ID, EventType: "platform.promotion.v1.VoucherChanged", Payload: []byte("x"), Topic: "promotion.events"}, nil
	}); err != nil {
		t.Fatalf("create voucher: %v", err)
	}

	const n = 10
	type outcome struct {
		res voucher.ReserveResult
		err error
	}
	results := make([]outcome, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id := "race-" + string(rune('a'+i))
			res, err := svc.ValidateAndReserve(ctx, id, "RACE1", "buyer-race", 5000, "")
			results[i] = outcome{res, err}
		}()
	}
	close(start)
	wg.Wait()

	valid, limited := 0, 0
	for i, o := range results {
		if o.err != nil {
			t.Fatalf("call %d: %v", i, o.err)
		}
		switch {
		case o.res.Valid:
			valid++
		case o.res.Reason == "VOUCHER_PER_USER_LIMIT":
			limited++
		default:
			t.Fatalf("call %d: unexpected result %+v", i, o.res)
		}
	}
	if valid != 1 || limited != n-1 {
		t.Fatalf("valid=%d limited=%d, want 1 and %d", valid, limited, n-1)
	}
	var holds int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM voucher_reservations WHERE buyer_id = 'buyer-race'`).Scan(&holds); err != nil || holds != 1 {
		t.Fatalf("hold rows = %d err=%v, want 1", holds, err)
	}
}
