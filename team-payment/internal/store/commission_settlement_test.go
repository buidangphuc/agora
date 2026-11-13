package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/store"
)

const commissionMigration = 19

func commissionPostgres(t *testing.T) (stores, *pgxpool.Pool) {
	t.Helper()
	pool := schemaPool(t)
	applyMigrations(t, pool, 1, commissionMigration)
	return stores{
		settle:   store.NewPostgresSettlementLedger(pool),
		ledger:   store.NewPostgresLedgerRepository(pool),
		payments: store.NewPostgresPaymentRepository(pool),
	}, pool
}

func eachCommissionBackend(t *testing.T, fn func(t *testing.T, s stores, pool *pgxpool.Pool)) {
	t.Run("memory", func(t *testing.T) { fn(t, inMemoryStores(t), nil) })
	t.Run("postgres", func(t *testing.T) {
		s, pool := commissionPostgres(t)
		fn(t, s, pool)
	})
}

func postgresOnly(t *testing.T, fn func(t *testing.T, s stores, pool *pgxpool.Pool)) {
	t.Helper()
	s, pool := commissionPostgres(t)
	fn(t, s, pool)
}

func withBreakdown(base, platform int64, bps int) store.Commissionable {
	return store.Commissionable{PlatformDiscount: platform, Base: base, RateBps: bps, OK: true}
}

func settle(t *testing.T, s stores, order, seller string, paid int64, c store.Commissionable) (store.PaymentTransaction, store.CreditResult) {
	t.Helper()
	tx := paidTx(t, s, order, paid)
	res, err := s.settle.CreditSettlement(context.Background(), order, seller, c)
	if err != nil {
		t.Fatalf("credit %s: %v", order, err)
	}
	return tx, res
}

func sumOf(es []store.LedgerEntry) int64 {
	var n int64
	for _, e := range es {
		n += e.Amount
	}
	return n
}

func wantNone(t *testing.T, got []store.LedgerEntry, what string) {
	t.Helper()
	if len(got) != 0 {
		t.Fatalf("want no %s entry, got %+v", what, got)
	}
}

func TestCommissionSettlement_BooksFivePercentOnSubtotalMinusShopVoucher(t *testing.T) {
	// scenario: A paid order books 5% on subtotal minus shop voucher
	eachCommissionBackend(t, func(t *testing.T, s stores, pool *pgxpool.Pool) {
		tx, res := settle(t, s, "o1", "seller", 310000, withBreakdown(280000, 0, 500))
		if !res.Credited {
			t.Fatalf("credit not written: %+v", res)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeOrderSettlement], 310000, tx.ID)
		wantOne(t, got[store.LedgerTypeCommission], -14000, tx.ID)
		if c := got[store.LedgerTypeCommission][0]; c.Status != store.LedgerStatusCompleted || c.SellerID != "seller" {
			t.Fatalf("commission entry %+v", c)
		}
		if bal != 296000 {
			t.Fatalf("balance %d, want 296000", bal)
		}
		if pool != nil {
			var rate *int32
			if err := pool.QueryRow(context.Background(),
				`SELECT commission_rate_bps FROM wallet_ledger WHERE type = 'COMMISSION' AND reference_id = $1`, tx.ID).Scan(&rate); err != nil {
				t.Fatalf("read rate: %v", err)
			}
			if rate == nil || *rate != 500 {
				t.Fatalf("commission_rate_bps = %v, want 500", rate)
			}
		}
	})
}

func TestCommissionSettlement_RecordsTheRateApplied(t *testing.T) {
	// scenario: A paid order books 5% on subtotal minus shop voucher
	postgresOnly(t, func(t *testing.T, s stores, pool *pgxpool.Pool) {
		tx, _ := settle(t, s, "o-rate", "seller", 100000, withBreakdown(100000, 0, 3000))
		got, _ := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeCommission], -30000, tx.ID)
		var rate *int32
		if err := pool.QueryRow(context.Background(),
			`SELECT commission_rate_bps FROM wallet_ledger WHERE type = 'COMMISSION' AND reference_id = $1`, tx.ID).Scan(&rate); err != nil {
			t.Fatalf("read rate: %v", err)
		}
		if rate == nil || *rate != 3000 {
			t.Fatalf("commission_rate_bps = %v, want 3000", rate)
		}
	})
}

func TestCommissionSettlement_PlatformVoucherAndShippingOutsideBase(t *testing.T) {
	// scenario: The platform voucher and shipping are outside the base
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		tx, _ := settle(t, s, "o2", "seller", 320000, withBreakdown(300000, 10000, 500))
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeOrderSettlement], 330000, tx.ID)
		wantOne(t, got[store.LedgerTypeCommission], -15000, tx.ID)
		if bal != 315000 {
			t.Fatalf("balance %d, want 315000", bal)
		}
	})
}

func TestCommissionSettlement_RoundsHalfUp(t *testing.T) {
	// scenario: Commission rounds half up to whole VND
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		tx1, _ := settle(t, s, "o-a", "seller", 100010, withBreakdown(100010, 0, 500))
		tx2, _ := settle(t, s, "o-b", "seller", 100001, withBreakdown(100001, 0, 500))
		got, _ := ledgerOf(t, s, "seller")
		byRef := map[string]int64{}
		for _, e := range got[store.LedgerTypeCommission] {
			byRef[e.ReferenceID] = e.Amount
		}
		if byRef[tx1.ID] != -5001 || byRef[tx2.ID] != -5000 || len(byRef) != 2 {
			t.Fatalf("commissions %+v, want %s:-5001 %s:-5000", byRef, tx1.ID, tx2.ID)
		}
	})
}

func TestCommissionSettlement_ZeroCommissionWritesNoRow(t *testing.T) {
	// scenario: A zero commission writes no row
	cases := []struct {
		name string
		c    store.Commissionable
	}{
		{"base too small", withBreakdown(5, 0, 500)},
		{"rate zero", withBreakdown(300000, 0, 0)},
		{"zero base", withBreakdown(0, 0, 500)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
				tx, res := settle(t, s, "o-zero", "seller", 1000, tc.c)
				if !res.Credited {
					t.Fatalf("credit not written")
				}
				got, bal := ledgerOf(t, s, "seller")
				wantOne(t, got[store.LedgerTypeOrderSettlement], 1000, tx.ID)
				wantNone(t, got[store.LedgerTypeCommission], "COMMISSION")
				if bal != 1000 {
					t.Fatalf("balance %d, want 1000", bal)
				}
			})
		})
	}
}

func TestCommissionSettlement_PlatformDiscountCreditedWithoutBase(t *testing.T) {
	// scenario: The platform voucher and shipping are outside the base
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		tx, _ := settle(t, s, "o-pd", "seller", 1000, withBreakdown(5, 700, 500))
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeOrderSettlement], 1700, tx.ID)
		wantNone(t, got[store.LedgerTypeCommission], "COMMISSION")
		if bal != 1700 {
			t.Fatalf("balance %d, want 1700", bal)
		}
	})
}

func TestCommissionSettlement_RedeliveredEventBooksNoSecondCommission(t *testing.T) {
	// scenario: A redelivered paid event books no second commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		c := withBreakdown(280000, 0, 500)
		tx, _ := settle(t, s, "o-dup", "seller", 310000, c)
		res, err := s.settle.CreditSettlement(ctx, "o-dup", "seller", c)
		if err != nil || res.Credited {
			t.Fatalf("replay must be a no-op: %+v %v", res, err)
		}
		tx2, _ := settle(t, s, "o-sentinel", "seller", 100000, withBreakdown(100000, 0, 500))
		got, bal := ledgerOf(t, s, "seller")
		if n := len(got[store.LedgerTypeCommission]); n != 2 {
			t.Fatalf("commission entries = %d, want 2: %+v", n, got[store.LedgerTypeCommission])
		}
		for _, e := range got[store.LedgerTypeCommission] {
			if e.ReferenceID != tx.ID && e.ReferenceID != tx2.ID {
				t.Fatalf("unexpected commission %+v", e)
			}
		}
		if bal != (310000-14000)+(100000-5000) {
			t.Fatalf("balance %d, want %d", bal, (310000-14000)+(100000-5000))
		}
	})
}

func TestCommissionSettlement_ReplayWithAnotherRateKeepsTheFirstCommission(t *testing.T) {
	// scenario: A redelivered paid event books no second commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		tx, _ := settle(t, s, "o-rr", "seller", 310000, withBreakdown(280000, 0, 500))
		if _, err := s.settle.CreditSettlement(context.Background(), "o-rr", "seller", withBreakdown(280000, 0, 1000)); err != nil {
			t.Fatalf("replay: %v", err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeCommission], -14000, tx.ID)
		if bal != 296000 {
			t.Fatalf("balance %d, want 296000", bal)
		}
	})
}

func TestCommissionSettlement_ConcurrentSettlementsBookOneCommission(t *testing.T) {
	// scenario: Concurrent settlements of one order book one commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-race", 310000)
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := s.settle.CreditSettlement(ctx, "o-race", "seller", withBreakdown(280000, 0, 500))
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent credit: %v", err)
			}
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeOrderSettlement], 310000, tx.ID)
		wantOne(t, got[store.LedgerTypeCommission], -14000, tx.ID)
		if bal != 296000 {
			t.Fatalf("balance %d, want 296000", bal)
		}
	})
}

func TestCommissionSettlement_FailedCommissionInsertRollsBackTheCredit(t *testing.T) {
	// scenario: A failed commission insert rolls back the credit
	postgresOnly(t, func(t *testing.T, s stores, pool *pgxpool.Pool) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-fail", 310000)
		mustExec(t, pool, `CREATE FUNCTION fail_commission() RETURNS trigger LANGUAGE plpgsql AS
			$$ BEGIN RAISE EXCEPTION 'commission insert blocked'; END $$`)
		mustExec(t, pool, `CREATE TRIGGER fail_commission BEFORE INSERT ON wallet_ledger
			FOR EACH ROW WHEN (NEW.type = 'COMMISSION') EXECUTE FUNCTION fail_commission()`)
		if _, err := s.settle.CreditSettlement(ctx, "o-fail", "seller", withBreakdown(280000, 0, 500)); err == nil {
			t.Fatal("credit succeeded although the commission insert failed")
		}
		got, bal := ledgerOf(t, s, "seller")
		wantNone(t, got[store.LedgerTypeOrderSettlement], "ORDER_SETTLEMENT")
		wantNone(t, got[store.LedgerTypeCommission], "COMMISSION")
		if bal != 0 {
			t.Fatalf("balance %d after a failed settlement, want 0", bal)
		}
		mustExec(t, pool, `DROP TRIGGER fail_commission ON wallet_ledger`)
		res, err := s.settle.CreditSettlement(ctx, "o-fail", "seller", withBreakdown(280000, 0, 500))
		if err != nil || !res.Credited {
			t.Fatalf("retry: %+v %v", res, err)
		}
		got, bal = ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeOrderSettlement], 310000, tx.ID)
		wantOne(t, got[store.LedgerTypeCommission], -14000, tx.ID)
		if bal != 296000 {
			t.Fatalf("balance %d, want 296000", bal)
		}
	})
}

func commissioned(t *testing.T, s stores, order string) store.PaymentTransaction {
	t.Helper()
	tx, _ := settle(t, s, order, "seller", 310000, withBreakdown(280000, 0, 500))
	return tx
}

func TestReversal_PartialRefundReturnsProportionOfCommission(t *testing.T) {
	// scenario: A 40% partial refund returns 40% of the commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := commissioned(t, s, "o-40")
		res, err := rpcRefund(ctx, s, tx.ID, "r1", 124000)
		if err != nil || !res.Deducted {
			t.Fatalf("refund: %+v %v", res, err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeRefundDeduction], -124000, "rpc:r1")
		wantOne(t, got[store.LedgerTypeCommissionReversal], 5600, "rpc:r1")
		if e := got[store.LedgerTypeCommissionReversal][0]; e.Status != store.LedgerStatusCompleted || e.SellerID != "seller" {
			t.Fatalf("reversal entry %+v", e)
		}
		if bal != 177600 {
			t.Fatalf("net %d, want 177600", bal)
		}
	})
}

func TestReversal_FullRefundReturnsExactlyTheCommission(t *testing.T) {
	// scenario: A full refund returns exactly the commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := commissioned(t, s, "o-full")
		if _, err := rpcRefund(ctx, s, tx.ID, "r1", 124000); err != nil {
			t.Fatalf("first refund: %v", err)
		}
		if _, err := rpcRefund(ctx, s, tx.ID, "r2", 186000); err != nil {
			t.Fatalf("second refund: %v", err)
		}
		got, bal := ledgerOf(t, s, "seller")
		revs := map[string]int64{}
		for _, e := range got[store.LedgerTypeCommissionReversal] {
			revs[e.ReferenceID] = e.Amount
		}
		if len(revs) != 2 || revs["rpc:r1"] != 5600 || revs["rpc:r2"] != 8400 || sumOf(got[store.LedgerTypeCommissionReversal]) != 14000 {
			t.Fatalf("reversals %+v, want rpc:r1:5600 rpc:r2:8400", revs)
		}
		if bal != 0 {
			t.Fatalf("balance for the order %d, want 0", bal)
		}
		wantRefunds(t, s, tx.ID, 310000, store.PaymentStatusRefunded, 2)
	})
}

func TestReversal_CumulativeRoundingNeverExceedsCommission(t *testing.T) {
	// scenario: Cumulative rounding never exceeds the commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := commissioned(t, s, "o-310")
		for i := 1; i <= 310; i++ {
			if _, err := rpcRefund(ctx, s, tx.ID, fmt.Sprintf("r%03d", i), 1000); err != nil {
				t.Fatalf("refund %d: %v", i, err)
			}
			refunded := int64(i) * 1000
			limit := (2*14000*refunded + 310000) / (2 * 310000)
			got, _ := ledgerOf(t, s, "seller")
			if sum := sumOf(got[store.LedgerTypeCommissionReversal]); sum > limit {
				t.Fatalf("after %d refunds reversals total %d, above round_half_up %d", i, sum, limit)
			}
			if i == 310 {
				if sum := sumOf(got[store.LedgerTypeCommissionReversal]); sum != 14000 {
					t.Fatalf("final reversals total %d, want 14000", sum)
				}
			}
		}
	})
}

func TestReversal_TinyRefundBooksNoReversal(t *testing.T) {
	// scenario: A tiny refund whose share rounds to zero books no reversal
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		tx := commissioned(t, s, "o-tiny")
		if _, err := rpcRefund(context.Background(), s, tx.ID, "r1", 1); err != nil {
			t.Fatalf("refund: %v", err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeRefundDeduction], -1, "rpc:r1")
		wantNone(t, got[store.LedgerTypeCommissionReversal], "COMMISSION_REVERSAL")
		if bal != 310000-14000-1 {
			t.Fatalf("balance %d, want %d", bal, 310000-14000-1)
		}
	})
}

func TestReversal_CancellingPaidOrderReturnsWholeCommission(t *testing.T) {
	// scenario: Cancelling a paid order returns the whole commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := commissioned(t, s, "o-cancel")
		cancel := store.RefundRequest{
			PaymentID: tx.ID, Key: "cancel:o-cancel", Source: store.RefundSourceOrderCancel, SourceID: "o-cancel",
			Reason: "cancel", Mode: store.RefundRemainder,
		}
		for i := 0; i < 2; i++ {
			if _, err := s.settle.ApplyRefund(ctx, cancel); err != nil {
				t.Fatalf("cancel %d: %v", i, err)
			}
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeCommissionReversal], 14000, "cancel:o-cancel")
		wantOne(t, got[store.LedgerTypeRefundDeduction], -310000, "cancel:o-cancel")
		if bal != 0 {
			t.Fatalf("net %d, want 0", bal)
		}
	})
}

func TestReversal_CancelAfterPartialRefundReturnsTheRest(t *testing.T) {
	// scenario: Cancelling a paid order returns the whole commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := commissioned(t, s, "o-cancel2")
		if _, err := rpcRefund(ctx, s, tx.ID, "r1", 124000); err != nil {
			t.Fatal(err)
		}
		if _, err := s.settle.ApplyRefund(ctx, store.RefundRequest{
			PaymentID: tx.ID, Key: "cancel:o-cancel2", Source: store.RefundSourceOrderCancel, SourceID: "o-cancel2",
			Reason: "cancel", Mode: store.RefundRemainder,
		}); err != nil {
			t.Fatal(err)
		}
		got, bal := ledgerOf(t, s, "seller")
		if sumOf(got[store.LedgerTypeCommissionReversal]) != 14000 || len(got[store.LedgerTypeCommissionReversal]) != 2 || bal != 0 {
			t.Fatalf("reversals %+v balance %d", got[store.LedgerTypeCommissionReversal], bal)
		}
	})
}

func TestReversal_RefundBeforeCreditStillGetsItsReversal(t *testing.T) {
	// scenario: A refund that landed before the credit still gets its reversal
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-before", 310000)
		if _, err := rpcRefund(ctx, s, tx.ID, "r1", 124000); err != nil {
			t.Fatalf("early refund: %v", err)
		}
		got, _ := ledgerOf(t, s, "seller")
		wantNone(t, got[store.LedgerTypeCommissionReversal], "COMMISSION_REVERSAL")
		for i := 0; i < 2; i++ {
			if _, err := s.settle.CreditSettlement(ctx, "o-before", "seller", withBreakdown(280000, 0, 500)); err != nil {
				t.Fatalf("credit %d: %v", i, err)
			}
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeOrderSettlement], 310000, tx.ID)
		wantOne(t, got[store.LedgerTypeCommission], -14000, tx.ID)
		wantOne(t, got[store.LedgerTypeRefundDeduction], -124000, "rpc:r1")
		wantOne(t, got[store.LedgerTypeCommissionReversal], 5600, "rpc:r1")
		if bal != 177600 {
			t.Fatalf("balance %d, want 177600", bal)
		}
	})
}

func TestReversal_SeveralRefundsBeforeCreditUseTheCumulativeRule(t *testing.T) {
	// scenario: A refund that landed before the credit still gets its reversal
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-before2", 310000)
		for _, r := range []struct {
			id string
			n  int64
		}{{"r1", 124000}, {"r2", 186000}} {
			if _, err := rpcRefund(ctx, s, tx.ID, r.id, r.n); err != nil {
				t.Fatalf("refund %s: %v", r.id, err)
			}
		}
		if _, err := s.settle.CreditSettlement(ctx, "o-before2", "seller", withBreakdown(280000, 0, 500)); err != nil {
			t.Fatal(err)
		}
		got, bal := ledgerOf(t, s, "seller")
		revs := map[string]int64{}
		for _, e := range got[store.LedgerTypeCommissionReversal] {
			revs[e.ReferenceID] = e.Amount
		}
		if len(revs) != 2 || revs["rpc:r1"] != 5600 || revs["rpc:r2"] != 8400 || bal != 0 {
			t.Fatalf("reversals %+v balance %d", revs, bal)
		}
	})
}

func TestReversal_ReplayedRefundWritesNoSecondReversal(t *testing.T) {
	// scenario: A replayed refund writes no second reversal
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := commissioned(t, s, "o-replay")
		if _, err := rpcRefund(ctx, s, tx.ID, "r1", 124000); err != nil {
			t.Fatal(err)
		}
		before, balBefore := ledgerOf(t, s, "seller")
		res, err := rpcRefund(ctx, s, tx.ID, "r1", 124000)
		if err != nil || res.Created || res.Deducted {
			t.Fatalf("replay: %+v %v", res, err)
		}
		after, balAfter := ledgerOf(t, s, "seller")
		if balBefore != balAfter || len(before[store.LedgerTypeCommissionReversal]) != 1 || len(after[store.LedgerTypeCommissionReversal]) != 1 ||
			len(after[store.LedgerTypeRefundDeduction]) != 1 {
			t.Fatalf("replay changed the ledger: before %+v after %+v", before, after)
		}
	})
}

func TestReversal_PaymentWithoutCommissionIsRefundedWithoutReversal(t *testing.T) {
	// scenario: A payment without commission is refunded without a reversal
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		tx, _ := settle(t, s, "o-old", "seller", 320000, store.Commissionable{})
		if _, err := rpcRefund(context.Background(), s, tx.ID, "r1", 128000); err != nil {
			t.Fatal(err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[store.LedgerTypeOrderSettlement], 320000, tx.ID)
		wantNone(t, got[store.LedgerTypeCommission], "COMMISSION")
		wantOne(t, got[store.LedgerTypeRefundDeduction], -128000, "rpc:r1")
		wantNone(t, got[store.LedgerTypeCommissionReversal], "COMMISSION_REVERSAL")
		if bal != 192000 {
			t.Fatalf("balance %d, want 192000", bal)
		}
	})
}

func TestReversal_ZeroCommissionPaymentIsRefundedWithoutReversal(t *testing.T) {
	// scenario: A zero commission writes no row
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		tx, _ := settle(t, s, "o-z", "seller", 1000, withBreakdown(5, 0, 500))
		if _, err := rpcRefund(context.Background(), s, tx.ID, "r1", 1000); err != nil {
			t.Fatal(err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantNone(t, got[store.LedgerTypeCommissionReversal], "COMMISSION_REVERSAL")
		if bal != 0 {
			t.Fatalf("balance %d, want 0", bal)
		}
	})
}

func TestReversal_ConcurrentRefundsNeverReverseMoreThanTheCommission(t *testing.T) {
	// scenario: Concurrent refunds never reverse more than the commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		ctx := context.Background()
		tx := commissioned(t, s, "o-conc")
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := rpcRefund(ctx, s, tx.ID, fmt.Sprintf("c%d", i), 38750)
				errs <- err
			}(i)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent refund: %v", err)
			}
		}
		got, bal := ledgerOf(t, s, "seller")
		if sum := sumOf(got[store.LedgerTypeCommissionReversal]); sum != 14000 {
			t.Fatalf("reversals total %d, want 14000: %+v", sum, got[store.LedgerTypeCommissionReversal])
		}
		if bal != 0 {
			t.Fatalf("balance %d, want 0", bal)
		}
		wantRefunds(t, s, tx.ID, 310000, store.PaymentStatusRefunded, 8)
	})
}

func TestReversal_RefundOfUncreditedPaymentWritesNoReversal(t *testing.T) {
	// scenario: A refund that landed before the credit still gets its reversal
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		tx := paidTx(t, s, "o-nocredit", 310000)
		if _, err := rpcRefund(context.Background(), s, tx.ID, "r1", 124000); err != nil {
			t.Fatal(err)
		}
		got, bal := ledgerOf(t, s, "seller")
		if len(got) != 0 || bal != 0 {
			t.Fatalf("uncredited payment wrote entries %+v balance %d", got, bal)
		}
	})
}
