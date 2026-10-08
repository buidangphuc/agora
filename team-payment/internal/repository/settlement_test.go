package repository_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/buidangphuc/team-payment/internal/pgtest"
	"github.com/buidangphuc/team-payment/internal/repository"
)

// stores is one backend under test: the settlement ledger plus the stores it writes.
type stores struct {
	settle   repository.SettlementLedger
	ledger   repository.LedgerRepository
	payments repository.PaymentRepository
}

func inMemoryStores(*testing.T) stores {
	p := repository.NewInMemoryPaymentRepository()
	l := repository.NewInMemoryLedgerRepository()
	return stores{settle: repository.NewInMemorySettlementLedger(p, l), ledger: l, payments: p}
}

func postgresStores(t *testing.T) stores {
	pool := pgtest.Pool(t)
	return stores{
		settle:   repository.NewPostgresSettlementLedger(pool),
		ledger:   repository.NewPostgresLedgerRepository(pool),
		payments: repository.NewPostgresPaymentRepository(pool),
	}
}

// eachBackend runs fn against the in-memory stores and, with TEST_DATABASE_URL, Postgres.
func eachBackend(t *testing.T, fn func(t *testing.T, s stores)) {
	t.Run("memory", func(t *testing.T) { fn(t, inMemoryStores(t)) })
	t.Run("postgres", func(t *testing.T) { fn(t, postgresStores(t)) })
}

func paidTx(t *testing.T, s stores, orderID string, amount int64) repository.PaymentTransaction {
	t.Helper()
	ctx := context.Background()
	tx, err := s.payments.CreateTransaction(ctx, repository.PaymentTransaction{OrderID: orderID, BuyerID: "buyer", Amount: amount})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}
	tx, err = s.payments.UpdateTransactionStatus(ctx, tx.ID, repository.PaymentStatusPaid, "MOCK-REF")
	if err != nil {
		t.Fatalf("pay tx: %v", err)
	}
	return tx
}

// countByType counts the seller's entries of typ referencing ref, and sums the balance.
func ledgerOf(t *testing.T, s stores, seller string) (map[string][]repository.LedgerEntry, int64) {
	t.Helper()
	ctx := context.Background()
	entries, _, err := s.ledger.ListEntries(ctx, seller, 0, 1000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := map[string][]repository.LedgerEntry{}
	for _, e := range entries {
		out[e.Type] = append(out[e.Type], e)
	}
	bal, err := s.ledger.Balance(ctx, seller)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return out, bal
}

func wantOne(t *testing.T, got []repository.LedgerEntry, amount int64, ref string) {
	t.Helper()
	if len(got) != 1 || got[0].Amount != amount || got[0].ReferenceID != ref {
		t.Fatalf("want exactly one entry of %d ref %s, got %+v", amount, ref, got)
	}
}

func TestCreditSettlement_DuplicateIsNoop(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-dup", 500000)
		r1, err := s.settle.CreditSettlement(ctx, "o-dup", "seller")
		if err != nil || !r1.Credited {
			t.Fatalf("first credit: %+v %v", r1, err)
		}
		r2, err := s.settle.CreditSettlement(ctx, "o-dup", "seller")
		if err != nil || r2.Credited {
			t.Fatalf("second credit must be a no-op: %+v %v", r2, err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[repository.LedgerTypeOrderSettlement], 500000, tx.ID)
		if got[repository.LedgerTypeOrderSettlement][0].Status != repository.LedgerStatusCompleted || bal != 500000 {
			t.Fatalf("balance %d, entries %+v", bal, got)
		}
	})
}

func TestCreditSettlement_NoSettledTransaction(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		if _, err := s.settle.CreditSettlement(ctx, "o-none", "seller"); !errors.Is(err, repository.ErrNotSettled) {
			t.Fatalf("no tx: want ErrNotSettled, got %v", err)
		}
		if _, err := s.payments.CreateTransaction(ctx, repository.PaymentTransaction{OrderID: "o-pending", BuyerID: "b", Amount: 10}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.settle.CreditSettlement(ctx, "o-pending", "seller"); !errors.Is(err, repository.ErrNotSettled) {
			t.Fatalf("pending tx: want ErrNotSettled, got %v", err)
		}
		if got, bal := ledgerOf(t, s, "seller"); len(got) != 0 || bal != 0 {
			t.Fatalf("nothing may be written: %+v %d", got, bal)
		}
	})
}

func TestCreditThenRefund_OneCreditOneDeduction(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-cr", 500000)
		if _, err := s.settle.CreditSettlement(ctx, "o-cr", "seller"); err != nil {
			t.Fatal(err)
		}
		res, err := s.settle.Refund(ctx, tx.ID, 200000, "REFUND:x")
		if err != nil || !res.Deducted || res.Transaction.Status != repository.PaymentStatusRefunded || res.Transaction.RefundedAmount != 200000 {
			t.Fatalf("refund: %+v %v", res, err)
		}
		// Redelivered credit after the refund writes nothing more.
		if r, err := s.settle.CreditSettlement(ctx, "o-cr", "seller"); err != nil || r.Credited || r.Deducted {
			t.Fatalf("redelivered credit: %+v %v", r, err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[repository.LedgerTypeOrderSettlement], 500000, tx.ID)
		wantOne(t, got[repository.LedgerTypeRefundDeduction], -200000, tx.ID)
		if bal != 300000 {
			t.Fatalf("balance %d, want 300000", bal)
		}
		if _, err := s.settle.Refund(ctx, tx.ID, 100000, "REFUND:again"); !errors.Is(err, repository.ErrNotRefundable) {
			t.Fatalf("second refund: want ErrNotRefundable, got %v", err)
		}
	})
}

func TestRefundThenCredit_OneCreditOneDeduction(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-rc", 500000)
		res, err := s.settle.Refund(ctx, tx.ID, 500000, "REFUND:x")
		if err != nil || res.Deducted {
			t.Fatalf("refund before credit must not deduct: %+v %v", res, err)
		}
		if got, bal := ledgerOf(t, s, "seller"); len(got) != 0 || bal != 0 {
			t.Fatalf("refund without credit wrote %+v (%d)", got, bal)
		}
		r, err := s.settle.CreditSettlement(ctx, "o-rc", "seller")
		if err != nil || !r.Credited || !r.Deducted {
			t.Fatalf("credit after refund: %+v %v", r, err)
		}
		if r, err := s.settle.CreditSettlement(ctx, "o-rc", "seller"); err != nil || r.Credited || r.Deducted {
			t.Fatalf("redelivered credit: %+v %v", r, err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[repository.LedgerTypeOrderSettlement], 500000, tx.ID)
		wantOne(t, got[repository.LedgerTypeRefundDeduction], -500000, tx.ID)
		if bal != 0 {
			t.Fatalf("balance %d, want 0", bal)
		}
	})
}

func TestRefundWithoutCredit_WritesNoDeduction(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-nc", 500000)
		if _, err := s.settle.Refund(ctx, tx.ID, 500000, "REFUND:x"); err != nil {
			t.Fatal(err)
		}
		if got, bal := ledgerOf(t, s, "seller"); len(got) != 0 || bal != 0 {
			t.Fatalf("got %+v (%d), want an empty ledger", got, bal)
		}
		if _, err := s.settle.Refund(ctx, "no-such-tx", 1, "x"); !errors.Is(err, repository.ErrTransactionNotFound) {
			t.Fatalf("unknown tx: want ErrTransactionNotFound, got %v", err)
		}
	})
}

func TestConcurrentRefunds_OneWinner(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-race", 500000)
		if _, err := s.settle.CreditSettlement(ctx, "o-race", "seller"); err != nil {
			t.Fatal(err)
		}
		var ok, lost atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := s.settle.Refund(ctx, tx.ID, 100000, "REFUND:race")
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, repository.ErrNotRefundable):
					lost.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 1 || lost.Load() != 7 {
			t.Fatalf("ok=%d lost=%d, want 1/7", ok.Load(), lost.Load())
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[repository.LedgerTypeRefundDeduction], -100000, tx.ID)
		if bal != 400000 {
			t.Fatalf("balance %d, want 400000", bal)
		}
	})
}

// Credit and refund racing each other end with one credit and one deduction whichever
// wins the payment row.
func TestConcurrentCreditAndRefund(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		for i := 0; i < 10; i++ {
			order := "o-cr-race-" + string(rune('a'+i))
			tx := paidTx(t, s, order, 1000)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				if _, err := s.settle.CreditSettlement(ctx, order, "seller-cr"); err != nil {
					t.Errorf("credit: %v", err)
				}
			}()
			go func() {
				defer wg.Done()
				if _, err := s.settle.Refund(ctx, tx.ID, 1000, "REFUND:x"); err != nil {
					t.Errorf("refund: %v", err)
				}
			}()
			wg.Wait()
		}
		got, bal := ledgerOf(t, s, "seller-cr")
		if len(got[repository.LedgerTypeOrderSettlement]) != 10 || len(got[repository.LedgerTypeRefundDeduction]) != 10 || bal != 0 {
			t.Fatalf("credits=%d deductions=%d balance=%d, want 10/10/0",
				len(got[repository.LedgerTypeOrderSettlement]), len(got[repository.LedgerTypeRefundDeduction]), bal)
		}
	})
}

func TestAppendEntryOnce(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		e := repository.LedgerEntry{SellerID: "s1", Type: repository.LedgerTypeOrderSettlement, Amount: 7, ReferenceID: "ref-1"}
		first, created, err := s.ledger.AppendEntryOnce(ctx, e)
		if err != nil || !created {
			t.Fatalf("first: %v %v", created, err)
		}
		again, created, err := s.ledger.AppendEntryOnce(ctx, e)
		if err != nil || created || again.ID != first.ID {
			t.Fatalf("second: created=%v id=%s/%s err=%v", created, again.ID, first.ID, err)
		}
		if _, _, err := s.ledger.AppendEntryOnce(ctx, repository.LedgerEntry{SellerID: "s1", Type: repository.LedgerTypeOrderSettlement, Amount: 7}); !errors.Is(err, repository.ErrReferenceRequired) {
			t.Fatalf("no reference: want ErrReferenceRequired, got %v", err)
		}
	})
}

// The store itself refuses the invalid rows of the seller-settlement-credit store
// scenarios, through the repository's own insert path.
func TestLedgerStoreRejectsInvalidRows_Postgres(t *testing.T) {
	s := postgresStores(t)
	ctx := context.Background()
	tx := paidTx(t, s, "o-store", 500)
	if _, err := s.settle.CreditSettlement(ctx, "o-store", "seller"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		e    repository.LedgerEntry
		code string
	}{
		{"second settlement for one payment", repository.LedgerEntry{SellerID: "seller", Type: repository.LedgerTypeOrderSettlement, Amount: 500, ReferenceID: tx.ID}, "23505"},
		{"settlement that is not positive", repository.LedgerEntry{SellerID: "seller", Type: repository.LedgerTypeOrderSettlement, Amount: -1, ReferenceID: "fresh-ref"}, "23514"},
		{"refund deduction without reference", repository.LedgerEntry{SellerID: "seller", Type: repository.LedgerTypeRefundDeduction, Amount: -1}, "23514"},
	}
	for _, c := range cases {
		if _, err := s.ledger.AppendEntry(ctx, c.e); pgCode(err) != c.code {
			t.Errorf("%s: want SQLSTATE %s, got %v", c.name, c.code, err)
		}
	}
	if _, bal := ledgerOf(t, s, "seller"); bal != 500 {
		t.Fatalf("balance %d, want 500 (unchanged)", bal)
	}
}
