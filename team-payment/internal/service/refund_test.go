package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

type refundFixture struct {
	svc      *service.PaymentService
	payments *repository.InMemoryPaymentRepository
	ledger   *repository.InMemoryLedgerRepository
}

// newRefundFixture wires a service whose order "o1" has a credited PAID payment of 500000.
func newRefundFixture(t *testing.T) (refundFixture, repository.PaymentTransaction) {
	t.Helper()
	ctx := context.Background()
	p := repository.NewInMemoryPaymentRepository()
	l := repository.NewInMemoryLedgerRepository()
	f := refundFixture{payments: p, ledger: l,
		svc: service.NewPaymentService(p, repository.NewInMemoryWalletRepository(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)),
			service.WithLedgerRepo(l), service.WithSettlementLedger(repository.NewInMemorySettlementLedger(p, l)))}
	tx, err := p.CreateTransaction(ctx, repository.PaymentTransaction{ID: "tx1", OrderID: "o1", BuyerID: "b", Amount: 500000, Status: repository.PaymentStatusPaid})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.CreditSettlement(ctx, "o1", "s1", 500000); err != nil {
		t.Fatal(err)
	}
	return f, tx
}

func (f refundFixture) payment(t *testing.T) repository.PaymentTransaction {
	t.Helper()
	tx, err := f.svc.GetPayment(context.Background(), "", "o1")
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func (f refundFixture) deductions(t *testing.T) map[string]int64 {
	t.Helper()
	entries, _, err := f.ledger.ListEntries(context.Background(), "s1", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, e := range entries {
		if e.Type == repository.LedgerTypeRefundDeduction {
			out[e.ReferenceID] += e.Amount
		}
	}
	return out
}

func TestValidRefundID(t *testing.T) {
	for id, want := range map[string]bool{
		"":                      false,
		"R1":                    true,
		"a.b_c:d-e":             true,
		strings.Repeat("x", 64): true,
		strings.Repeat("x", 65): false,
		"has space":             false,
		"slash/no":              false,
		"emojié":                false,
	} {
		if got := service.ValidRefundID(id); got != want {
			t.Errorf("ValidRefundID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestRefundPayment_StrictMode(t *testing.T) {
	ctx := context.Background()
	f, tx := newRefundFixture(t)

	for _, id := range []string{"", "bad id", strings.Repeat("x", 65)} {
		if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, id, 1000, "r"); !errors.Is(err, service.ErrInvalidRefundID) {
			t.Fatalf("refund id %q: want ErrInvalidRefundID, got %v", id, err)
		}
	}
	got, ok, _, err := f.svc.RefundPayment(ctx, tx.ID, "R1", 200000, "damaged")
	if err != nil || !ok || got.Status != repository.PaymentStatusPartiallyRefunded || got.RefundedAmount != 200000 || len(got.Refunds) != 1 {
		t.Fatalf("first refund: %+v %v", got, err)
	}
	if r := got.Refunds[0]; r.ID != "rpc:R1" || r.SourceID != "R1" || r.Source != repository.RefundSourceSellerOrAdmin || r.Reason != "damaged" {
		t.Fatalf("refund row: %+v", r)
	}
	// By order id, same id and amount: a replay.
	if got, _, _, err := f.svc.RefundPayment(ctx, "o1", "R1", 200000, "damaged"); err != nil || got.RefundedAmount != 200000 || len(got.Refunds) != 1 {
		t.Fatalf("replay: %+v %v", got, err)
	}
	if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, "R1", 100000, "x"); !errors.Is(err, service.ErrRefundIDConflict) {
		t.Fatalf("same id other amount: want ErrRefundIDConflict, got %v", err)
	}
	if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, "R2", 300001, "x"); !errors.Is(err, service.ErrExceedsRemainder) {
		t.Fatalf("above remainder: want ErrExceedsRemainder, got %v", err)
	}
	if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, "R2", 300000, "x"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, "R3", 1, "x"); !errors.Is(err, service.ErrInvalidRefund) {
		t.Fatalf("refund of a REFUNDED payment: want ErrInvalidRefund, got %v", err)
	}
	if p := f.payment(t); p.Status != repository.PaymentStatusRefunded || len(p.Refunds) != 2 {
		t.Fatalf("payment: %+v", p)
	}
}

func TestRefundCancelledOrder_RemainderMode(t *testing.T) {
	ctx := context.Background()
	t.Run("after a partial refund refunds the remainder", func(t *testing.T) {
		f, tx := newRefundFixture(t)
		if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, "R1", 200000, "x"); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.RefundCancelledOrder(ctx, "o1"); err != nil {
			t.Fatal(err)
		}
		p := f.payment(t)
		if p.Status != repository.PaymentStatusRefunded || p.RefundedAmount != 500000 || len(p.Refunds) != 2 {
			t.Fatalf("payment: %+v", p)
		}
		if c := p.Refunds[1]; c.ID != "cancel:o1" || c.Source != repository.RefundSourceOrderCancel || c.SourceID != "o1" || c.Amount != 300000 || c.Reason != "order_cancelled" {
			t.Fatalf("cancel refund: %+v", c)
		}
		// Redelivered: a no-op.
		if err := f.svc.RefundCancelledOrder(ctx, "o1"); err != nil {
			t.Fatal(err)
		}
		if d := f.deductions(t); len(d) != 2 || d["rpc:R1"] != -200000 || d["cancel:o1"] != -300000 {
			t.Fatalf("deductions %v", d)
		}
	})
	t.Run("after a full refund is a no-op", func(t *testing.T) {
		f, tx := newRefundFixture(t)
		if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, "R1", 500000, "x"); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.RefundCancelledOrder(ctx, "o1"); err != nil {
			t.Fatal(err)
		}
		if p := f.payment(t); p.Status != repository.PaymentStatusRefunded || len(p.Refunds) != 1 {
			t.Fatalf("payment: %+v", p)
		}
		if d := f.deductions(t); len(d) != 1 || d["rpc:R1"] != -500000 {
			t.Fatalf("deductions %v", d)
		}
	})
	t.Run("without a settled payment", func(t *testing.T) {
		f, _ := newRefundFixture(t)
		if err := f.svc.RefundCancelledOrder(ctx, "o-none"); !errors.Is(err, service.ErrNotSettled) {
			t.Fatalf("want ErrNotSettled, got %v", err)
		}
	})
}

func TestRefundReturn_ClampMode(t *testing.T) {
	ctx := context.Background()
	t.Run("refunds once", func(t *testing.T) {
		f, _ := newRefundFixture(t)
		for i := 0; i < 2; i++ {
			if err := f.svc.RefundReturn(ctx, "o1", "ret-1", 200000); err != nil {
				t.Fatal(err)
			}
		}
		p := f.payment(t)
		if p.Status != repository.PaymentStatusPartiallyRefunded || p.RefundedAmount != 200000 || len(p.Refunds) != 1 {
			t.Fatalf("payment: %+v", p)
		}
		if r := p.Refunds[0]; r.ID != "return:ret-1" || r.Source != repository.RefundSourceReturn || r.SourceID != "ret-1" || r.Reason != "return_refunded" {
			t.Fatalf("return refund: %+v", r)
		}
		if d := f.deductions(t); len(d) != 1 || d["return:ret-1"] != -200000 {
			t.Fatalf("deductions %v", d)
		}
	})
	t.Run("clamps to what a direct refund left", func(t *testing.T) {
		f, tx := newRefundFixture(t)
		if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, "D", 400000, "x"); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.RefundReturn(ctx, "o1", "ret-1", 300000); err != nil {
			t.Fatal(err)
		}
		p := f.payment(t)
		if p.Status != repository.PaymentStatusRefunded || p.RefundedAmount != 500000 {
			t.Fatalf("payment: %+v", p)
		}
		if r := p.Refunds[1]; r.RequestedAmount != 300000 || r.Amount != 100000 {
			t.Fatalf("return refund: %+v", r)
		}
	})
	t.Run("after a full refund records an applied 0", func(t *testing.T) {
		f, tx := newRefundFixture(t)
		if _, _, _, err := f.svc.RefundPayment(ctx, tx.ID, "D", 500000, "x"); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.RefundReturn(ctx, "o1", "ret-1", 200000); err != nil {
			t.Fatalf("an applied 0 is not an error: %v", err)
		}
		p := f.payment(t)
		if p.Status != repository.PaymentStatusRefunded || p.RefundedAmount != 500000 || len(p.Refunds) != 2 {
			t.Fatalf("payment: %+v", p)
		}
		if r := p.Refunds[1]; r.RequestedAmount != 200000 || r.Amount != 0 {
			t.Fatalf("return refund: %+v", r)
		}
		if d := f.deductions(t); len(d) != 1 {
			t.Fatalf("an applied 0 must not deduct: %v", d)
		}
	})
	t.Run("invalid or unsettled", func(t *testing.T) {
		f, _ := newRefundFixture(t)
		if err := f.svc.RefundReturn(ctx, "o-none", "ret", 1); !errors.Is(err, service.ErrNotSettled) {
			t.Fatalf("no payment: want ErrNotSettled, got %v", err)
		}
		if err := f.svc.RefundReturn(ctx, "o1", "ret", 0); !errors.Is(err, service.ErrInvalidAmount) {
			t.Fatalf("zero amount: want ErrInvalidAmount, got %v", err)
		}
		if err := f.svc.RefundReturn(ctx, "o1", "", 1); err == nil {
			t.Fatal("missing return id accepted")
		}
	})
}
