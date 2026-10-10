package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/team-payment/internal/repository"
)

func credited(t *testing.T, s stores, order string, amount int64) repository.PaymentTransaction {
	t.Helper()
	tx := paidTx(t, s, order, amount)
	if _, err := s.settle.CreditSettlement(context.Background(), order, "seller"); err != nil {
		t.Fatalf("credit: %v", err)
	}
	return tx
}

func returnRefund(ctx context.Context, s stores, txID, returnID string, amount int64) (repository.RefundResult, error) {
	return s.settle.ApplyRefund(ctx, repository.RefundRequest{
		PaymentID: txID, Key: "return:" + returnID, Source: repository.RefundSourceReturn, SourceID: returnID,
		Requested: amount, Reason: "return_refunded", Mode: repository.RefundClamp,
	})
}

func cancelRefund(ctx context.Context, s stores, txID, orderID string) (repository.RefundResult, error) {
	return s.settle.ApplyRefund(ctx, repository.RefundRequest{
		PaymentID: txID, Key: "cancel:" + orderID, Source: repository.RefundSourceOrderCancel, SourceID: orderID,
		Reason: "order_cancelled", Mode: repository.RefundRemainder,
	})
}

// deductions returns the seller's REFUND_DEDUCTION amounts by reference.
func deductions(t *testing.T, s stores) map[string]int64 {
	t.Helper()
	got, _ := ledgerOf(t, s, "seller")
	out := map[string]int64{}
	for _, e := range got[repository.LedgerTypeRefundDeduction] {
		if _, dup := out[e.ReferenceID]; dup {
			t.Fatalf("two deductions reference %s", e.ReferenceID)
		}
		out[e.ReferenceID] = e.Amount
	}
	return out
}

func wantDeductions(t *testing.T, s stores, want map[string]int64) {
	t.Helper()
	got := deductions(t, s)
	if len(got) != len(want) {
		t.Fatalf("deductions %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("deductions %v, want %v", got, want)
		}
	}
}

func TestApplyRefund_TwoPartialRefunds(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := credited(t, s, "o-2p", 500000)
		res, err := rpcRefund(ctx, s, tx.ID, "r1", 200000)
		if err != nil || !res.Created || !res.Deducted {
			t.Fatalf("first: %+v %v", res, err)
		}
		wantRefunds(t, s, tx.ID, 200000, repository.PaymentStatusPartiallyRefunded, 1)
		if _, err := rpcRefund(ctx, s, tx.ID, "r2", 300000); err != nil {
			t.Fatalf("second: %v", err)
		}
		refunds := wantRefunds(t, s, tx.ID, 500000, repository.PaymentStatusRefunded, 2)
		if refunds[0].ID != "rpc:r1" || refunds[1].ID != "rpc:r2" || refunds[0].SourceID != "r1" ||
			refunds[0].Source != repository.RefundSourceSellerOrAdmin || refunds[1].RequestedAmount != 300000 {
			t.Fatalf("refunds not oldest first / wrong fields: %+v", refunds)
		}
		wantDeductions(t, s, map[string]int64{"rpc:r1": -200000, "rpc:r2": -300000})
		if _, err := rpcRefund(ctx, s, tx.ID, "r3", 1); !errors.Is(err, repository.ErrNotRefundable) {
			t.Fatalf("refund of a REFUNDED payment: want ErrNotRefundable, got %v", err)
		}
	})
}

func TestApplyRefund_AboveRemainderWritesNothing(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := credited(t, s, "o-above", 500000)
		if _, err := rpcRefund(ctx, s, tx.ID, "r1", 200000); err != nil {
			t.Fatal(err)
		}
		if _, err := rpcRefund(ctx, s, tx.ID, "r2", 300001); !errors.Is(err, repository.ErrExceedsRemainder) {
			t.Fatalf("want ErrExceedsRemainder, got %v", err)
		}
		wantRefunds(t, s, tx.ID, 200000, repository.PaymentStatusPartiallyRefunded, 1)
		wantDeductions(t, s, map[string]int64{"rpc:r1": -200000})
		// The refused key is free: it was never stored.
		if _, err := rpcRefund(ctx, s, tx.ID, "r2", 300000); err != nil {
			t.Fatalf("r2 with the remainder: %v", err)
		}
	})
}

func TestApplyRefund_ReplayAndConflict(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := credited(t, s, "o-replay", 500000)
		other := credited(t, s, "o-replay-other", 500000)
		if _, err := rpcRefund(ctx, s, tx.ID, "R", 200000); err != nil {
			t.Fatal(err)
		}
		res, err := rpcRefund(ctx, s, tx.ID, "R", 200000)
		if err != nil || res.Created || res.Deducted || res.Transaction.RefundedAmount != 200000 {
			t.Fatalf("replay must be a no-op returning the current state: %+v %v", res, err)
		}
		if _, err := rpcRefund(ctx, s, tx.ID, "R", 100000); !errors.Is(err, repository.ErrRefundIDConflict) {
			t.Fatalf("same key, other amount: want ErrRefundIDConflict, got %v", err)
		}
		if _, err := rpcRefund(ctx, s, other.ID, "R", 200000); !errors.Is(err, repository.ErrRefundIDConflict) {
			t.Fatalf("same key, other payment: want ErrRefundIDConflict, got %v", err)
		}
		wantRefunds(t, s, tx.ID, 200000, repository.PaymentStatusPartiallyRefunded, 1)
		wantRefunds(t, s, other.ID, 0, repository.PaymentStatusPaid, 0)
		wantDeductions(t, s, map[string]int64{"rpc:R": -200000})
	})
}

// Eight concurrent refunds of 125000 on 500000, each with its own key: exactly four win.
func TestApplyRefund_ConcurrentDistinctKeysCapAtAmount(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := credited(t, s, "o-8x", 500000)
		var ok, refused atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := rpcRefund(ctx, s, tx.ID, fmt.Sprintf("k%d", i), 125000)
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, repository.ErrExceedsRemainder), errors.Is(err, repository.ErrNotRefundable):
					refused.Add(1)
				default:
					t.Errorf("unexpected error (CHECK backstop?): %v", err)
				}
			}(i)
		}
		wg.Wait()
		if ok.Load() != 4 || refused.Load() != 4 {
			t.Fatalf("ok=%d refused=%d, want 4/4", ok.Load(), refused.Load())
		}
		refunds := wantRefunds(t, s, tx.ID, 500000, repository.PaymentStatusRefunded, 4)
		want := map[string]int64{}
		for _, r := range refunds {
			want[r.ID] = -125000
		}
		wantDeductions(t, s, want)
	})
}

// Two concurrent refunds of 150000 with 200000 left: one wins.
func TestApplyRefund_ConcurrentNearTheCap(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := credited(t, s, "o-cap", 500000)
		if _, err := rpcRefund(ctx, s, tx.ID, "first", 300000); err != nil {
			t.Fatal(err)
		}
		var ok, refused atomic.Int32
		var wg sync.WaitGroup
		for _, k := range []string{"a", "b"} {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				_, err := rpcRefund(ctx, s, tx.ID, k, 150000)
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, repository.ErrExceedsRemainder):
					refused.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}(k)
		}
		wg.Wait()
		if ok.Load() != 1 || refused.Load() != 1 {
			t.Fatalf("ok=%d refused=%d, want 1/1", ok.Load(), refused.Load())
		}
		wantRefunds(t, s, tx.ID, 450000, repository.PaymentStatusPartiallyRefunded, 2)
		if d := deductions(t, s); len(d) != 2 || d["rpc:first"] != -300000 {
			t.Fatalf("deductions %v", d)
		}
	})
}

// The same key on two payments at once: one refund is stored, the other is a conflict.
func TestApplyRefund_ConcurrentSameKeyTwoPayments(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		a := credited(t, s, "o-ka", 500000)
		b := credited(t, s, "o-kb", 500000)
		var ok, conflict atomic.Int32
		var wg sync.WaitGroup
		for _, id := range []string{a.ID, b.ID} {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				_, err := rpcRefund(ctx, s, id, "shared", 100000)
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, repository.ErrRefundIDConflict):
					conflict.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}(id)
		}
		wg.Wait()
		if ok.Load() != 1 || conflict.Load() != 1 {
			t.Fatalf("ok=%d conflict=%d, want 1/1", ok.Load(), conflict.Load())
		}
		if d := deductions(t, s); len(d) != 1 || d["rpc:shared"] != -100000 {
			t.Fatalf("deductions %v", d)
		}
	})
}

func TestApplyRefund_ClampRecordsRequestedAndApplied(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := credited(t, s, "o-clamp", 500000)
		if _, err := rpcRefund(ctx, s, tx.ID, "direct", 400000); err != nil {
			t.Fatal(err)
		}
		res, err := returnRefund(ctx, s, tx.ID, "ret-1", 300000)
		if err != nil || !res.Created || res.Refund.RequestedAmount != 300000 || res.Refund.Amount != 100000 {
			t.Fatalf("clamp: %+v %v", res, err)
		}
		wantRefunds(t, s, tx.ID, 500000, repository.PaymentStatusRefunded, 2)
		// Nothing left: an applied 0 is recorded, with no deduction.
		res, err = returnRefund(ctx, s, tx.ID, "ret-2", 200000)
		if err != nil || !res.Created || res.Deducted || res.Refund.Amount != 0 || res.Refund.RequestedAmount != 200000 {
			t.Fatalf("applied 0: %+v %v", res, err)
		}
		refunds := wantRefunds(t, s, tx.ID, 500000, repository.PaymentStatusRefunded, 3)
		if r := refunds[2]; r.ID != "return:ret-2" || r.Source != repository.RefundSourceReturn || r.SourceID != "ret-2" || r.Reason != "return_refunded" {
			t.Fatalf("return refund fields: %+v", r)
		}
		wantDeductions(t, s, map[string]int64{"rpc:direct": -400000, "return:ret-1": -100000})
		// A redelivered return is a no-op, also after the clamp.
		if res, err := returnRefund(ctx, s, tx.ID, "ret-1", 300000); err != nil || res.Created {
			t.Fatalf("redelivered return: %+v %v", res, err)
		}
		wantRefunds(t, s, tx.ID, 500000, repository.PaymentStatusRefunded, 3)
	})
}

func TestApplyRefund_ClampOnUnpaidIsNotRefundable(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx, err := s.payments.CreateTransaction(ctx, repository.PaymentTransaction{OrderID: "o-pend", BuyerID: "b", Amount: 10})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := returnRefund(ctx, s, tx.ID, "ret", 5); !errors.Is(err, repository.ErrNotRefundable) {
			t.Fatalf("want ErrNotRefundable, got %v", err)
		}
		if res, err := cancelRefund(ctx, s, tx.ID, "o-pend"); err != nil || res.Created {
			t.Fatalf("remainder on a pending payment: %+v %v", res, err)
		}
		wantRefunds(t, s, tx.ID, 0, repository.PaymentStatusPending, 0)
	})
}

func TestApplyRefund_RemainderMode(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		// After a partial refund: the remainder.
		tx := credited(t, s, "o-rem", 500000)
		if _, err := rpcRefund(ctx, s, tx.ID, "part", 200000); err != nil {
			t.Fatal(err)
		}
		res, err := cancelRefund(ctx, s, tx.ID, "o-rem")
		if err != nil || !res.Created || res.Refund.Amount != 300000 || res.Refund.RequestedAmount != 300000 {
			t.Fatalf("cancel remainder: %+v %v", res, err)
		}
		wantRefunds(t, s, tx.ID, 500000, repository.PaymentStatusRefunded, 2)
		// Redelivered: finds its key.
		if res, err := cancelRefund(ctx, s, tx.ID, "o-rem"); err != nil || res.Created || res.Deducted {
			t.Fatalf("redelivered cancel: %+v %v", res, err)
		}
		wantDeductions(t, s, map[string]int64{"rpc:part": -200000, "cancel:o-rem": -300000})

		// On a REFUNDED payment: nothing written.
		full := credited(t, s, "o-rem-full", 500000)
		if _, err := rpcRefund(ctx, s, full.ID, "all", 500000); err != nil {
			t.Fatal(err)
		}
		if res, err := cancelRefund(ctx, s, full.ID, "o-rem-full"); err != nil || res.Created || res.Transaction.Status != repository.PaymentStatusRefunded {
			t.Fatalf("cancel of a REFUNDED payment: %+v %v", res, err)
		}
		wantRefunds(t, s, full.ID, 500000, repository.PaymentStatusRefunded, 1)
	})
}

// Two refunds before the credit, then the credit: one credit and two deductions.
func TestApplyRefund_TwoRefundsBeforeTheCredit(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-before", 500000)
		for k, a := range map[string]int64{"r1": 100000, "r2": 150000} {
			if res, err := rpcRefund(ctx, s, tx.ID, k, a); err != nil || res.Deducted {
				t.Fatalf("refund %s before credit: %+v %v", k, res, err)
			}
		}
		if got, _ := ledgerOf(t, s, "seller"); len(got) != 0 {
			t.Fatalf("refund before credit wrote %+v", got)
		}
		res, err := s.settle.CreditSettlement(ctx, "o-before", "seller")
		if err != nil || !res.Credited || !res.Deducted || res.Transaction.Status != repository.PaymentStatusPartiallyRefunded {
			t.Fatalf("credit: %+v %v", res, err)
		}
		if res, err := s.settle.CreditSettlement(ctx, "o-before", "seller"); err != nil || res.Credited || res.Deducted {
			t.Fatalf("redelivered credit: %+v %v", res, err)
		}
		got, bal := ledgerOf(t, s, "seller")
		wantOne(t, got[repository.LedgerTypeOrderSettlement], 500000, tx.ID)
		wantDeductions(t, s, map[string]int64{"rpc:r1": -100000, "rpc:r2": -150000})
		if bal != 250000 {
			t.Fatalf("balance %d, want 250000", bal)
		}
		wantRefunds(t, s, tx.ID, 250000, repository.PaymentStatusPartiallyRefunded, 2)
	})
}

// An applied-0 refund is never deducted by the credit path.
func TestCreditSettlement_SkipsAppliedZeroRefunds(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-zero", 500000)
		if _, err := rpcRefund(ctx, s, tx.ID, "all", 500000); err != nil {
			t.Fatal(err)
		}
		if res, err := returnRefund(ctx, s, tx.ID, "ret", 100000); err != nil || res.Refund.Amount != 0 {
			t.Fatalf("return: %+v %v", res, err)
		}
		if _, err := s.settle.CreditSettlement(ctx, "o-zero", "seller"); err != nil {
			t.Fatal(err)
		}
		wantDeductions(t, s, map[string]int64{"rpc:all": -500000})
	})
}

// seller-payout-holdback: two partial refunds of a held sale both reduce its held amount.
func TestHold_TwoPartialRefundsOfAHeldSale(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		now := time.Now()
		creditAt(t, s, "s", "o-old", 300000, now.Add(-window-time.Hour))
		fresh := creditAt(t, s, "s", "o-new", 500000, now.Add(-time.Minute))
		var wg sync.WaitGroup
		for k, a := range map[string]int64{"h1": 200000, "h2": 100000} {
			wg.Add(1)
			go func(k string, a int64) {
				defer wg.Done()
				if _, err := rpcRefund(ctx, s, fresh.ID, k, a); err != nil {
					t.Errorf("refund %s: %v", k, err)
				}
			}(k, a)
		}
		wg.Wait()
		// balance 500000, held 200000 (500000-300000): 300000 withdrawable, then nothing.
		if err := payout(s, "s", 300000, now, window); err != nil {
			t.Fatalf("payout 300000: %v", err)
		}
		if err := payout(s, "s", 1, now, window); !errors.Is(err, repository.ErrFundsOnHold) {
			t.Fatalf("payout 1: want ErrFundsOnHold, got %v", err)
		}
	})
}

func TestApplyRefund_InvalidRequests(t *testing.T) {
	eachBackend(t, func(t *testing.T, s stores) {
		ctx := context.Background()
		tx := paidTx(t, s, "o-inv", 500)
		if _, err := rpcRefund(ctx, s, tx.ID, "z", 0); !errors.Is(err, repository.ErrInvalidAmount) {
			t.Fatalf("zero amount: %v", err)
		}
		if _, err := s.settle.ApplyRefund(ctx, repository.RefundRequest{PaymentID: tx.ID, Source: repository.RefundSourceSellerOrAdmin, SourceID: "x", Requested: 1, Mode: repository.RefundStrict}); err == nil {
			t.Fatal("missing key accepted")
		}
		wantRefunds(t, s, tx.ID, 0, repository.PaymentStatusPaid, 0)
	})
}
