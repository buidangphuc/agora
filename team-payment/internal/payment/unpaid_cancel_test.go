package payment_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
)

type cancelFixture struct {
	svc      *payment.Service
	payments *fake.InMemoryPaymentRepository
	writer   *fake.InMemoryTxWriter
	orders   *mockOrderClient
}

func newCancelFixture(t *testing.T) cancelFixture {
	t.Helper()
	p := fake.NewInMemoryPaymentRepository()
	w := fake.NewInMemoryTxWriter(p)
	oc := &mockOrderClient{orders: map[string]*orderv1.Order{}}
	svc := newSvc(p, fake.NewInMemoryWalletRepository(), oc,
		slog.New(slog.DiscardHandler),
		withWriter(w),
		withSettlement(fake.NewInMemorySettlementLedger(p, fake.NewInMemoryLedgerRepository())))
	return cancelFixture{svc: svc, payments: p, writer: w, orders: oc}
}

func (f cancelFixture) order(id string, st orderv1.OrderStatus) {
	f.orders.orders[id] = &orderv1.Order{Id: id, Status: st}
}

func (f cancelFixture) tx(ctx context.Context, t *testing.T, id, orderID string, amount int64, st store.PaymentStatus) store.PaymentTransaction {
	t.Helper()
	tx, err := f.payments.CreateTransaction(ctx,
		store.PaymentTransaction{ID: id, OrderID: orderID, BuyerID: "b", Amount: amount, Status: st})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func (f cancelFixture) read(ctx context.Context, t *testing.T, orderID string) store.PaymentTransaction {
	t.Helper()
	tx, err := f.svc.GetPayment(ctx, "", orderID)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func cancelRefundRows(tx store.PaymentTransaction, key string) int {
	n := 0
	for _, r := range tx.Refunds {
		if r.ID == key {
			n++
		}
	}
	return n
}

// staleReadRepo returns a PENDING snapshot from the first GetTransaction of a payment and
// then runs hook, modelling a concurrent writer committing between the read and the write.
type staleReadRepo struct {
	*fake.InMemoryPaymentRepository
	mu   sync.Mutex
	done bool
	hook func()
}

func (r *staleReadRepo) GetTransaction(ctx context.Context, id string) (store.PaymentTransaction, error) {
	cur, err := r.InMemoryPaymentRepository.GetTransaction(ctx, id)
	r.mu.Lock()
	first := !r.done
	r.done = true
	r.mu.Unlock()
	if err == nil && first {
		if r.hook != nil {
			r.hook()
		}
		cur.Status = store.PaymentStatusPending
	}
	return cur, err
}

// Scenario: Settle that loses the race to the void is refused.
func TestSettleLosingToVoidIsRefused(t *testing.T) {
	ctx := context.Background()
	inner := fake.NewInMemoryPaymentRepository()
	w := fake.NewInMemoryTxWriter(inner)
	oc := &mockOrderClient{orders: map[string]*orderv1.Order{"o1": {Id: "o1", Status: orderv1.OrderStatus_ORDER_STATUS_PENDING}}}
	repo := &staleReadRepo{InMemoryPaymentRepository: inner}
	svc := newSvc(repo, fake.NewInMemoryWalletRepository(), oc,
		slog.New(slog.DiscardHandler), withWriter(w))
	if _, err := inner.CreateTransaction(ctx, store.PaymentTransaction{ID: "p1", OrderID: "o1", BuyerID: "b", Amount: 1000, Status: store.PaymentStatusPending}); err != nil {
		t.Fatal(err)
	}
	// The void commits right after the settle's read.
	repo.hook = func() {
		if _, err := inner.CancelUnpaid(ctx, "o1"); err != nil {
			t.Errorf("void: %v", err)
		}
	}
	_, _, _, err := svc.ProcessMockPayment(ctx, "p1", true)
	if !errors.Is(err, payment.ErrPaymentCancelled) {
		t.Fatalf("settle losing to the void: %v, want ErrPaymentCancelled", err)
	}
	got, _ := inner.GetTransaction(ctx, "p1")
	if got.Status != store.PaymentStatusCancelled {
		t.Fatalf("status %v, want CANCELLED", got.Status)
	}
	if n := len(w.Rows()); n != 0 {
		t.Fatalf("%d outbox rows, want 0", n)
	}
}

// Scenario: Settle on an order already cancelled upstream is refused and voided.
func TestProcessMockPaymentRereadsCancelledOrder(t *testing.T) {
	ctx := context.Background()
	for _, st := range []store.PaymentStatus{store.PaymentStatusPending, store.PaymentStatusFailed} {
		f := newCancelFixture(t)
		f.order("o4", orderv1.OrderStatus_ORDER_STATUS_CANCELLED)
		f.tx(ctx, t, "P-4", "o4", 1000, st)
		_, _, _, err := f.svc.ProcessMockPayment(context.Background(), "P-4", true)
		if !errors.Is(err, payment.ErrPaymentCancelled) {
			t.Fatalf("from %v: %v, want ErrPaymentCancelled", st, err)
		}
		got, _ := f.payments.GetTransaction(context.Background(), "P-4")
		if got.Status != store.PaymentStatusCancelled {
			t.Fatalf("from %v: status %v, want CANCELLED", st, got.Status)
		}
		if n := len(f.writer.Rows()); n != 0 {
			t.Fatalf("%d outbox rows, want 0", n)
		}
	}
}

// Scenario: Settle is not attempted when team-order is unreachable (service half).
func TestProcessMockPaymentOrderUnreachableDoesNotSettle(t *testing.T) {
	ctx := context.Background()
	f := newCancelFixture(t)
	f.tx(ctx, t, "p1", "o1", 1000, store.PaymentStatusPending)
	f.orders.err = status.Error(codes.Unavailable, "down")
	_, _, _, err := f.svc.ProcessMockPayment(context.Background(), "p1", true)
	if !errors.Is(err, payment.ErrOrderServiceUnavailable) {
		t.Fatalf("got %v, want ErrOrderServiceUnavailable", err)
	}
	got, _ := f.payments.GetTransaction(context.Background(), "p1")
	if got.Status != store.PaymentStatusPending || len(f.writer.Rows()) != 0 {
		t.Fatalf("status %v rows %d: nothing may settle", got.Status, len(f.writer.Rows()))
	}
}

// Scenario: Settle that loses the race to the void is refused (terminal CANCELLED, both simulations).
func TestProcessMockPaymentRefusesCancelledTransaction(t *testing.T) {
	ctx := context.Background()
	for _, success := range []bool{true, false} {
		f := newCancelFixture(t)
		f.order("o1", orderv1.OrderStatus_ORDER_STATUS_PENDING)
		f.tx(ctx, t, "p1", "o1", 1000, store.PaymentStatusCancelled)
		_, _, _, err := f.svc.ProcessMockPayment(context.Background(), "p1", success)
		if !errors.Is(err, payment.ErrPaymentCancelled) {
			t.Fatalf("success=%v: %v, want ErrPaymentCancelled", success, err)
		}
		got, _ := f.payments.GetTransaction(context.Background(), "p1")
		if got.Status != store.PaymentStatusCancelled || len(f.writer.Rows()) != 0 {
			t.Fatalf("success=%v: status %v rows %d", success, got.Status, len(f.writer.Rows()))
		}
	}
}

// Scenario: Retry payment still works before the cancel.
func TestFailedTransactionCanBeResettledBeforeCancel(t *testing.T) {
	ctx := context.Background()
	f := newCancelFixture(t)
	f.order("o1", orderv1.OrderStatus_ORDER_STATUS_PENDING)
	f.tx(ctx, t, "p1", "o1", 1000, store.PaymentStatusFailed)
	got, ok, _, err := f.svc.ProcessMockPayment(ctx, "p1", true)
	if err != nil || !ok || got.Status != store.PaymentStatusPaid {
		t.Fatalf("retry: %+v ok=%v err=%v", got, ok, err)
	}
	if n := len(f.writer.Rows()); n != 1 {
		t.Fatalf("%d outbox rows, want 1", n)
	}
	// The cancel arriving afterwards finds a settled payment and refunds it (no void).
	if err := f.svc.VoidCancelledOrder(ctx, "o1"); err != nil {
		t.Fatal(err)
	}
	if p := f.read(ctx, t, "o1"); p.Status != store.PaymentStatusRefunded {
		t.Fatalf("after late cancel: %v, want REFUNDED", p.Status)
	}
}

// Scenario: Pending/Failed voided; absent, CANCELLED and redelivery are no-ops.
func TestVoidCancelledOrderVoidsUnpaidAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	for _, st := range []store.PaymentStatus{store.PaymentStatusPending, store.PaymentStatusFailed} {
		f := newCancelFixture(t)
		f.tx(ctx, t, "p1", "o1", 1000, st)
		for i := 0; i < 2; i++ {
			if err := f.svc.VoidCancelledOrder(ctx, "o1"); err != nil {
				t.Fatalf("from %v delivery %d: %v", st, i, err)
			}
		}
		p := f.read(ctx, t, "o1")
		if p.Status != store.PaymentStatusCancelled || len(p.Refunds) != 0 || len(f.writer.Rows()) != 0 {
			t.Fatalf("from %v: %+v rows=%d", st, p, len(f.writer.Rows()))
		}
	}
	f := newCancelFixture(t)
	if err := f.svc.VoidCancelledOrder(ctx, "no-such-order"); err != nil {
		t.Fatalf("absent transaction: %v", err)
	}
}

// Scenario: Void that loses the compare-and-set falls back to the refund.
func TestVoidLosingToSettleRefunds(t *testing.T) {
	ctx := context.Background()
	inner := fake.NewInMemoryPaymentRepository()
	repo := &settleBeforeVoidRepo{InMemoryPaymentRepository: inner}
	svc := newSvc(repo, fake.NewInMemoryWalletRepository(), &mockOrderClient{},
		slog.New(slog.DiscardHandler),
		withSettlement(fake.NewInMemorySettlementLedger(inner, fake.NewInMemoryLedgerRepository())))
	if _, err := inner.CreateTransaction(ctx, store.PaymentTransaction{ID: "p1", OrderID: "o1", BuyerID: "b", Amount: 500000, Status: store.PaymentStatusPending}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // second delivery moves no money
		if err := svc.VoidCancelledOrder(ctx, "o1"); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	p, err := svc.GetPayment(ctx, "", "o1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != store.PaymentStatusRefunded || p.RefundedAmount != 500000 || len(p.Refunds) != 1 ||
		cancelRefundRows(p, "cancel:o1") != 1 || p.Refunds[0].Amount != 500000 {
		t.Fatalf("payment: %+v", p)
	}
}

// settleBeforeVoidRepo makes the transaction PAID right before the void's compare-and-set.
type settleBeforeVoidRepo struct {
	*fake.InMemoryPaymentRepository
}

func (r *settleBeforeVoidRepo) CancelUnpaid(ctx context.Context, orderID string) (store.CancelUnpaidOutcome, error) {
	if tx, err := r.GetTransactionByOrderID(ctx, orderID); err == nil && tx.Status == store.PaymentStatusPending {
		if _, err := r.UpdateStatus(ctx, tx.ID, store.PaymentStatusPaid, "ref"); err != nil {
			return store.CancelAbsent, err
		}
	}
	return r.InMemoryPaymentRepository.CancelUnpaid(ctx, orderID)
}

// Scenario: Already refunded payment gets no second refund (and partial gets the remainder once).
func TestVoidCancelledOrderOnRefundedPayments(t *testing.T) {
	ctx := context.Background()
	t.Run("fully refunded is a no-op", func(t *testing.T) {
		f := newCancelFixture(t)
		f.tx(ctx, t, "p1", "o1", 500000, store.PaymentStatusPaid)
		if _, _, _, err := f.svc.RefundPayment(ctx, "p1", "R1", 500000, "x"); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.VoidCancelledOrder(ctx, "o1"); err != nil {
			t.Fatal(err)
		}
		p := f.read(ctx, t, "o1")
		if p.Status != store.PaymentStatusRefunded || len(p.Refunds) != 1 || p.RefundedAmount != 500000 {
			t.Fatalf("payment: %+v", p)
		}
	})
	t.Run("partially refunded gets the remainder once", func(t *testing.T) {
		f := newCancelFixture(t)
		f.tx(ctx, t, "p1", "o1", 500000, store.PaymentStatusPaid)
		if _, _, _, err := f.svc.RefundPayment(ctx, "p1", "R1", 200000, "x"); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if err := f.svc.VoidCancelledOrder(ctx, "o1"); err != nil {
				t.Fatal(err)
			}
		}
		p := f.read(ctx, t, "o1")
		if p.Status != store.PaymentStatusRefunded || p.RefundedAmount != 500000 || cancelRefundRows(p, "cancel:o1") != 1 {
			t.Fatalf("payment: %+v", p)
		}
	})
}

// settleAndCancelRace runs settle and cancel concurrently on a PENDING transaction.
func settleAndCancelRace(t *testing.T, svc *payment.Service, repo store.PaymentRepository, newTx func(id, orderID string)) {
	t.Helper()
	ctx := context.Background()
	const amount = 300000
	for i := 0; i < 50; i++ {
		id, orderID := fmt.Sprintf("race-p-%d", i), fmt.Sprintf("race-o-%d", i)
		newTx(id, orderID)
		var wg sync.WaitGroup
		var settleErr, cancelErr error
		wg.Add(2)
		go func() { defer wg.Done(); _, _, _, settleErr = svc.ProcessMockPayment(ctx, id, true) }()
		go func() { defer wg.Done(); cancelErr = svc.VoidCancelledOrder(ctx, orderID) }()
		wg.Wait()
		if cancelErr != nil {
			t.Fatalf("run %d: cancel: %v", i, cancelErr)
		}
		if settleErr != nil && !errors.Is(settleErr, payment.ErrPaymentCancelled) {
			t.Fatalf("run %d: settle: %v", i, settleErr)
		}
		// Redelivery of the cancel after the dust settles must not change the end state.
		if err := svc.VoidCancelledOrder(ctx, orderID); err != nil {
			t.Fatalf("run %d: redelivery: %v", i, err)
		}
		p, err := svc.GetPayment(ctx, "", orderID)
		if err != nil {
			t.Fatal(err)
		}
		switch p.Status {
		case store.PaymentStatusCancelled:
			if len(p.Refunds) != 0 || p.RefundedAmount != 0 {
				t.Fatalf("run %d: CANCELLED with refunds %+v", i, p)
			}
		case store.PaymentStatusRefunded:
			if len(p.Refunds) != 1 || p.Refunds[0].Amount != amount || p.RefundedAmount != amount {
				t.Fatalf("run %d: REFUNDED with %+v", i, p)
			}
		default:
			t.Fatalf("run %d: ended %v, want CANCELLED or REFUNDED", i, p.Status)
		}
	}
	_ = repo
}

// Scenario: Concurrent settle and cancel converge (in-memory).
func TestSettleAndCancelRaceConverges_InMemory(t *testing.T) {
	ctx := context.Background()
	f := newCancelFixture(t)
	for i := 0; i < 50; i++ {
		f.order(fmt.Sprintf("race-o-%d", i), orderv1.OrderStatus_ORDER_STATUS_PENDING)
	}
	settleAndCancelRace(t, f.svc, f.payments, func(id, orderID string) {
		f.tx(ctx, t, id, orderID, 300000, store.PaymentStatusPending)
	})
}

// Scenario: Concurrent settle and cancel converge (Postgres integration).
func TestSettleAndCancelRaceConverges(t *testing.T) {
	pool := migratedPool(t)
	payments := store.NewPostgresPaymentRepository(pool)
	oc := &mockOrderClient{orders: map[string]*orderv1.Order{}}
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("race-o-%d", i)
		oc.orders[id] = &orderv1.Order{Id: id, Status: orderv1.OrderStatus_ORDER_STATUS_PENDING}
	}
	svc := newSvc(payments, store.NewPostgresWalletRepository(pool), oc,
		slog.New(slog.DiscardHandler),
		withWriter(store.NewPgTxWriter(pool, outboxStore(t, pool))),
		withLedger(store.NewPostgresLedgerRepository(pool)),
		withSettlement(store.NewPostgresSettlementLedger(pool)))
	settleAndCancelRace(t, svc, payments, func(id, orderID string) {
		if _, err := payments.CreateTransaction(context.Background(),
			store.PaymentTransaction{ID: id, OrderID: orderID, BuyerID: "b", Amount: 300000, Status: store.PaymentStatusPending}); err != nil {
			t.Fatal(err)
		}
	})
}
