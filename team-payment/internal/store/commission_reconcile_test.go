package store_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/store"
)

type reconOrder struct {
	id         string
	paid       int64
	platform   int64
	base       int64
	commission bool
	refunds    []int64
}

type reconEvent struct {
	order  int
	refund int
}

func TestReconcile_LedgerReconcilesAfterAnyOrderOfEvents(t *testing.T) {
	// scenario: The ledger reconciles after any order of events
	for _, seed := range []int64{1, 2, 3} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
				ctx := context.Background()
				orders := []reconOrder{
					{id: "p1", paid: 310000, base: 280000, commission: true, refunds: []int64{124000, 186000}},
					{id: "p2", paid: 330000, platform: 10000, base: 300000, commission: true, refunds: []int64{1, 99999}},
					{id: "p3", paid: 100010, base: 100010, commission: true, refunds: []int64{33333, 33333, 33333}},
					{id: "p4", paid: 320000, commission: false, refunds: []int64{128000}},
					{id: "p5", paid: 50000, base: 3, commission: true},
					{id: "p6", paid: 200000, base: 200000, commission: true, refunds: []int64{200000}},
				}
				txs := make([]store.PaymentTransaction, len(orders))
				var events []reconEvent
				for i, o := range orders {
					txs[i] = paidTx(t, s, o.id, o.paid-o.platform)
					events = append(events, reconEvent{order: i, refund: -1})
					for k := range o.refunds {
						events = append(events, reconEvent{order: i, refund: k})
					}
				}
				rng := rand.New(rand.NewSource(seed))
				base := append([]reconEvent(nil), events...)
				events = append(events, base...)
				events = append(events, base[:len(base)/2]...)
				rng.Shuffle(len(events), func(i, j int) { events[i], events[j] = events[j], events[i] })

				for _, ev := range events {
					o := orders[ev.order]
					if ev.refund < 0 {
						c := store.Commissionable{}
						if o.commission {
							c = withBreakdown(o.base, o.platform, 500)
						}
						if _, err := s.settle.CreditSettlement(ctx, o.id, "seller", c); err != nil {
							t.Fatalf("credit %s: %v", o.id, err)
						}
						continue
					}
					if _, err := rpcRefund(ctx, s, txs[ev.order].ID, fmt.Sprintf("%s-%d", o.id, ev.refund), o.refunds[ev.refund]); err != nil {
						t.Fatalf("refund %s/%d: %v", o.id, ev.refund, err)
					}
				}

				entries, _, err := s.ledger.ListEntries(ctx, "seller", 0, 1000)
				if err != nil {
					t.Fatal(err)
				}
				bal, err := s.ledger.Balance(ctx, "seller")
				if err != nil {
					t.Fatal(err)
				}
				if sumOf(entries) != bal {
					t.Fatalf("balance %d != sum of entries %d", bal, sumOf(entries))
				}

				refKeys := map[string]string{}
				for i, o := range orders {
					refKeys[txs[i].ID] = o.id
					for k := range o.refunds {
						refKeys[fmt.Sprintf("rpc:%s-%d", o.id, k)] = o.id
					}
				}
				netByOrder := map[string]int64{}
				for _, e := range entries {
					o, ok := refKeys[e.ReferenceID]
					if !ok {
						t.Fatalf("entry with unknown reference: %+v", e)
					}
					netByOrder[o] += e.Amount
				}
				var wantTotal int64
				for _, o := range orders {
					credited := o.paid
					var commission, refunded int64
					if o.commission {
						commission = bigCommission(o.base, 500)
					}
					for _, r := range o.refunds {
						refunded += r
					}
					want := credited - commission - refunded + bigTarget(commission, refunded, o.paid-o.platform)
					if got := netByOrder[o.id]; got != want {
						t.Fatalf("order %s net %d, want %d", o.id, got, want)
					}
					wantTotal += want
				}
				if bal != wantTotal {
					t.Fatalf("seller balance %d, want %d", bal, wantTotal)
				}
			})
		})
	}
}
