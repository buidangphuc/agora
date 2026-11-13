package store_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/buidangphuc/team-payment/internal/store"
)

func p95(d []time.Duration) time.Duration {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[(len(d)*95+99)/100-1]
}

func TestCommissionOverheadAtMostFiveMsP95(t *testing.T) {
	// scenario: Commission adds at most 5 ms p95 to settlement
	s, _ := commissionPostgres(t)
	ctx := context.Background()
	const n, runs = 200, 3
	timeRun := func(prefix string, c store.Commissionable) time.Duration {
		d := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			order := fmt.Sprintf("%s-%d", prefix, i)
			tx, err := s.payments.CreateTransaction(ctx, store.PaymentTransaction{OrderID: order, BuyerID: "buyer", Amount: 310000})
			if err != nil {
				t.Fatalf("create tx %s: %v", order, err)
			}
			if _, err := s.payments.UpdateTransactionStatus(ctx, tx.ID, store.PaymentStatusPaid, "MOCK-REF"); err != nil {
				t.Fatalf("pay tx %s: %v", order, err)
			}
			start := time.Now()
			if _, err := s.settle.CreditSettlement(ctx, order, "seller-perf", c); err != nil {
				t.Fatalf("credit %s: %v", order, err)
			}
			d = append(d, time.Since(start))
		}
		return p95(d)
	}
	var deltas []time.Duration
	for r := 0; r < runs; r++ {
		without := timeRun(fmt.Sprintf("n%d", r), store.Commissionable{})
		with := timeRun(fmt.Sprintf("w%d", r), withBreakdown(280000, 0, 500))
		deltas = append(deltas, with-without)
	}
	sort.Slice(deltas, func(i, j int) bool { return deltas[i] < deltas[j] })
	if median := deltas[runs/2]; median > 5*time.Millisecond {
		t.Fatalf("median p95 overhead %s, want at most 5ms (%v)", median, deltas)
	}
	bal, err := s.ledger.Balance(ctx, "seller-perf")
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(n*runs) * (310000 + 296000); bal != want {
		t.Fatalf("balance %d, want %d: the with-breakdown settlements did not book commissions", bal, want)
	}
}
