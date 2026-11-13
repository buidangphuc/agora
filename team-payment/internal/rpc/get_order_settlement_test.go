package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/rpc"
	"github.com/buidangphuc/team-payment/internal/store"
)

type settlementFixture struct {
	h  *rpc.Handler
	st *stores
}

func newSettlementFixture(t *testing.T) settlementFixture {
	t.Helper()
	st := newStores()
	return settlementFixture{h: newHandler(t, st.service(t, nil), permissiveAudit), st: st}
}

func (f settlementFixture) pay(t *testing.T, orderID string, amount int64, st store.PaymentStatus) store.PaymentTransaction {
	t.Helper()
	tx, err := f.st.payments.CreateTransaction(context.Background(), store.PaymentTransaction{
		OrderID: orderID, BuyerID: "buyer-S", Amount: amount, Status: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func (f settlementFixture) commissioned(t *testing.T, orderID string) store.PaymentTransaction {
	t.Helper()
	tx := f.pay(t, orderID, 310000, store.PaymentStatusPaid)
	c := store.Commissionable{OK: true, Base: 280000, RateBps: 500}
	if _, err := f.st.settlement.CreditSettlement(context.Background(), orderID, "seller-S", c); err != nil {
		t.Fatal(err)
	}
	return tx
}

func (f settlementFixture) refund(t *testing.T, paymentID, key string, amount int64) {
	t.Helper()
	_, err := f.st.settlement.ApplyRefund(context.Background(), store.RefundRequest{
		PaymentID: paymentID, Key: key, Source: store.RefundSourceSellerOrAdmin, SourceID: key,
		Requested: amount, Reason: "r", Mode: store.RefundStrict,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func getSettlement(f settlementFixture, ctx context.Context, orderID string) (*paymentv1.GetOrderSettlementResponse, error) {
	return f.h.GetOrderSettlement(ctx, &paymentv1.GetOrderSettlementRequest{OrderId: orderID})
}

func wantBreakdown(t *testing.T, got *paymentv1.GetOrderSettlementResponse, credited, commission, reversed, deducted, net int64, bps int32) {
	t.Helper()
	if got.GetCredited() != credited || got.GetCommission() != commission || got.GetCommissionReversed() != reversed ||
		got.GetRefundDeducted() != deducted || got.GetNet() != net || got.GetCommissionRateBps() != bps {
		t.Fatalf("breakdown = %v, want credited %d commission %d reversed %d deducted %d net %d bps %d",
			got, credited, commission, reversed, deducted, net, bps)
	}
}

func TestGetOrderSettlement_SellerReadsTheBreakdownOfASettledOrder(t *testing.T) {
	f := newSettlementFixture(t)
	f.commissioned(t, "order-S")
	got, err := getSettlement(f, userCtx("seller-S"), "order-S")
	if err != nil {
		t.Fatalf("seller read: %v", err)
	}
	if got.GetOrderId() != "order-S" {
		t.Fatalf("order id %q", got.GetOrderId())
	}
	wantBreakdown(t, got, 310000, 14000, 0, 0, 296000, 500)
}

func TestGetOrderSettlement_BreakdownReflectsAPartialRefund(t *testing.T) {
	f := newSettlementFixture(t)
	tx := f.commissioned(t, "order-S")
	f.refund(t, tx.ID, "R1", 124000)
	got, err := getSettlement(f, userCtx("seller-S"), "order-S")
	if err != nil {
		t.Fatalf("seller read: %v", err)
	}
	wantBreakdown(t, got, 310000, 14000, 5600, 124000, 177600, 500)
}

func TestGetOrderSettlement_OrderSettledWithoutCommissionReportsZeroCommission(t *testing.T) {
	f := newSettlementFixture(t)
	f.pay(t, "order-N", 320000, store.PaymentStatusPaid)
	if _, err := f.st.settlement.CreditSettlement(context.Background(), "order-N", "seller-S", store.Commissionable{}); err != nil {
		t.Fatal(err)
	}
	got, err := getSettlement(f, userCtx("seller-S"), "order-N")
	if err != nil {
		t.Fatalf("seller read: %v", err)
	}
	wantBreakdown(t, got, 320000, 0, 0, 0, 320000, 0)
}

func TestGetOrderSettlement_AnotherSellerAndABuyerGetNotFound(t *testing.T) {
	f := newSettlementFixture(t)
	f.commissioned(t, "order-S")
	for name, ctx := range map[string]context.Context{
		"another seller": userCtx("seller-X"),
		"buyer":          userCtx("buyer-S"),
	} {
		_, err := getSettlement(f, ctx, "order-S")
		if status.Code(err) != codes.NotFound {
			t.Fatalf("%s: want NotFound, got %v", name, err)
		}
		_, unknown := getSettlement(f, ctx, "order-unknown")
		if status.Code(unknown) != codes.NotFound || status.Convert(unknown).Message() != status.Convert(err).Message() {
			t.Fatalf("%s: must look the same as an unknown order: %v vs %v", name, err, unknown)
		}
	}
}

func TestGetOrderSettlement_AdminCanReadAnySettlement(t *testing.T) {
	f := newSettlementFixture(t)
	tx := f.commissioned(t, "order-S")
	f.refund(t, tx.ID, "R1", 124000)
	want, err := getSettlement(f, userCtx("seller-S"), "order-S")
	if err != nil {
		t.Fatalf("seller read: %v", err)
	}
	got, err := getSettlement(f, adminCtx(), "order-S")
	if err != nil {
		t.Fatalf("admin read: %v", err)
	}
	wantBreakdown(t, got, want.GetCredited(), want.GetCommission(), want.GetCommissionReversed(),
		want.GetRefundDeducted(), want.GetNet(), want.GetCommissionRateBps())
	if got.GetNet() != 177600 {
		t.Fatalf("admin net %d, want 177600", got.GetNet())
	}
}

func TestGetOrderSettlement_UnsettledOrderIsNotFound(t *testing.T) {
	f := newSettlementFixture(t)
	f.pay(t, "order-pending", 100000, store.PaymentStatusPending)
	f.pay(t, "order-void", 100000, store.PaymentStatusPending)
	if _, err := f.st.payments.CancelUnpaid(context.Background(), "order-void"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"order-pending", "order-void"} {
		if _, err := getSettlement(f, userCtx("seller-S"), id); status.Code(err) != codes.NotFound {
			t.Fatalf("%s: want NotFound, got %v", id, err)
		}
	}
}

func TestGetOrderSettlement_MissingOrderIDIsRejected(t *testing.T) {
	f := newSettlementFixture(t)
	if _, err := getSettlement(f, userCtx("seller-S"), ""); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
}

func TestGetOrderSettlement_ReadingTheSettlementWritesNothing(t *testing.T) {
	f := newSettlementFixture(t)
	tx := f.commissioned(t, "order-S")
	f.refund(t, tx.ID, "R1", 124000)
	ctx := userCtx("seller-S")
	count := func() (int64, int) {
		_, total, err := f.st.ledger.ListEntries(context.Background(), "seller-S", 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		bal, err := f.st.ledger.Balance(context.Background(), "seller-S")
		if err != nil {
			t.Fatal(err)
		}
		return bal, int(total)
	}
	balBefore, nBefore := count()
	for i := 0; i < 10; i++ {
		got, err := getSettlement(f, ctx, "order-S")
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if got.GetNet() != 177600 {
			t.Fatalf("read %d: net %d, want 177600", i, got.GetNet())
		}
	}
	if bal, n := count(); bal != balBefore || n != nBefore {
		t.Fatalf("ledger changed by reads: balance %d->%d entries %d->%d", balBefore, bal, nBefore, n)
	}
}
