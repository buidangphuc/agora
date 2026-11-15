package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/store"
)

type kycFixture struct {
	h interface {
		RequestPayout(context.Context, *paymentv1.RequestPayoutRequest) (*paymentv1.RequestPayoutResponse, error)
		RequestWalletPayout(context.Context, *paymentv1.RequestWalletPayoutRequest) (*paymentv1.RequestWalletPayoutResponse, error)
		ListPayoutHistory(context.Context, *paymentv1.ListPayoutHistoryRequest) (*paymentv1.ListPayoutHistoryResponse, error)
	}
	st   *stores
	gate *recordingGate
}

func newKycFixture(t *testing.T, sellers ...string) kycFixture {
	t.Helper()
	st := newStores()
	gate := &recordingGate{}
	svc := st.service(t, nil, payment.WithPayoutHold(0))
	h := newHandlerGated(t, svc, permissiveAudit, discardLogger(), gate)
	for _, s := range sellers {
		if _, err := st.ledger.AppendEntry(context.Background(), store.LedgerEntry{
			SellerID: s, Type: store.LedgerTypeOrderSettlement, Amount: 1_000_000, Status: store.LedgerStatusCompleted,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return kycFixture{h: h, st: st, gate: gate}
}

func sellerCtx(id string) context.Context {
	return principalCtx(id, auth.TypeUser, "seller", "listing.write")
}

func bankReq(seller string, amount int64) *paymentv1.RequestPayoutRequest {
	return &paymentv1.RequestPayoutRequest{SellerId: seller, Amount: amount, BankCode: "VCB", AccountNumber: "1", AccountName: "A"}
}

func (f kycFixture) assertUntouched(t *testing.T, seller string, wantBalance int64) {
	t.Helper()
	ctx := context.Background()
	entries, _, err := f.st.ledger.ListEntries(ctx, seller, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type == store.LedgerTypePayout {
			t.Fatalf("payout ledger entry written for %s: %+v", seller, e)
		}
	}
	bal, err := f.st.ledger.Balance(ctx, seller)
	if err != nil || bal != wantBalance {
		t.Fatalf("balance = %d err=%v, want %d", bal, err, wantBalance)
	}
	hist, err := f.h.ListPayoutHistory(sellerCtx(seller), &paymentv1.ListPayoutHistoryRequest{SellerId: seller})
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.GetPayouts()) != 0 {
		t.Fatalf("payouts = %d, want 0", len(hist.GetPayouts()))
	}
}

func TestPayoutKycGate_InvalidAmountIsRejectedBeforeTheGate(t *testing.T) {
	f := newKycFixture(t, "seller-1")
	_, err := f.h.RequestPayout(sellerCtx("seller-1"), bankReq("seller-1", 0))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bank amount 0: %v", err)
	}
	_, err = f.h.RequestWalletPayout(sellerCtx("seller-1"), &paymentv1.RequestWalletPayoutRequest{SellerId: "seller-1", Amount: 0})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("wallet amount 0: %v", err)
	}
	_, err = f.h.RequestPayout(sellerCtx("seller-1"), &paymentv1.RequestPayoutRequest{SellerId: "seller-1", Amount: 10})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing bank fields: %v", err)
	}
	if got := f.gate.seen(); len(got) != 0 {
		t.Fatalf("gate called %v, want 0 calls", got)
	}
}

func TestPayoutKycGate_AnotherSellersWalletIsDeniedBeforeTheGate(t *testing.T) {
	f := newKycFixture(t, "seller-1")
	buyer := principalCtx("buyer-9", auth.TypeUser, "buyer", "listing.write")
	_, err := f.h.RequestPayout(buyer, bankReq("seller-1", 1000))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bank: %v", err)
	}
	_, err = f.h.RequestWalletPayout(buyer, &paymentv1.RequestWalletPayoutRequest{SellerId: "seller-1", Amount: 1000})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("wallet: %v", err)
	}
	if got := f.gate.seen(); len(got) != 0 {
		t.Fatalf("gate called %v, want 0 calls", got)
	}
	f.assertUntouched(t, "seller-1", 1_000_000)
}

func TestPayoutKycGate_MissingListingWriteScopeIsDeniedBeforeTheGate(t *testing.T) {
	f := newKycFixture(t, "seller-1")
	noScope := principalCtx("seller-1", auth.TypeUser, "buyer")
	_, err := f.h.RequestWalletPayout(noScope, &paymentv1.RequestWalletPayoutRequest{Amount: 1000})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("wallet: %v", err)
	}
	_, err = f.h.RequestPayout(noScope, bankReq("", 1000))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bank: %v", err)
	}
	if got := f.gate.seen(); len(got) != 0 {
		t.Fatalf("gate called %v, want 0 calls", got)
	}
}

func TestPayoutKycGate_StatusComesFromThePrincipalNotTheRequest(t *testing.T) {
	f := newKycFixture(t, "seller-v", "seller-u")
	f.gate.errFn = func(id string) error {
		if id == "seller-u" {
			return status.Error(codes.FailedPrecondition, "seller verification required")
		}
		return nil
	}
	ctx := sellerCtx("seller-u")
	_, err := f.h.RequestPayout(ctx, bankReq("seller-v", 1000))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bank: %v", err)
	}
	_, err = f.h.RequestWalletPayout(ctx, &paymentv1.RequestWalletPayoutRequest{SellerId: "seller-v", Amount: 1000})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("wallet: %v", err)
	}
	if got := f.gate.seen(); len(got) != 0 {
		t.Fatalf("gate called %v, want 0 calls", got)
	}
	f.assertUntouched(t, "seller-v", 1_000_000)
	f.assertUntouched(t, "seller-u", 1_000_000)
}

func TestPayoutKycGate_SellerWhoLosesVerificationIsRefusedOnTheNextCall(t *testing.T) {
	f := newKycFixture(t, "seller-1")
	ctx := sellerCtx("seller-1")
	if _, err := f.h.RequestPayout(ctx, bankReq("seller-1", 100_000)); err != nil {
		t.Fatalf("first payout: %v", err)
	}
	f.gate.errFn = func(string) error { return status.Error(codes.FailedPrecondition, "seller verification required") }
	_, err := f.h.RequestPayout(ctx, bankReq("seller-1", 100_000))
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "seller verification required" {
		t.Fatalf("second bank payout: %v", err)
	}
	_, err = f.h.RequestWalletPayout(ctx, &paymentv1.RequestWalletPayoutRequest{Amount: 100_000})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("wallet payout: %v", err)
	}
	hist, err := f.h.ListPayoutHistory(ctx, &paymentv1.ListPayoutHistoryRequest{SellerId: "seller-1"})
	if err != nil || len(hist.GetPayouts()) != 1 {
		t.Fatalf("history = %v err=%v, want exactly 1 payout", hist.GetPayouts(), err)
	}
	bal, _ := f.st.ledger.Balance(context.Background(), "seller-1")
	if bal != 900_000 {
		t.Fatalf("balance = %d, want 900000", bal)
	}
}

func TestPayoutKycGate_VerifiedSellerRequestsABankPayout(t *testing.T) {
	f := newKycFixture(t, "seller-1")
	ctx := sellerCtx("seller-1")
	res, err := f.h.RequestPayout(ctx, bankReq("", 300_000))
	if err != nil || res.GetPayout().GetAmount() != 300_000 {
		t.Fatalf("payout: %v %v", res, err)
	}
	if got := f.gate.seen(); len(got) != 1 || got[0] != "seller-1" {
		t.Fatalf("gate calls = %v, want [seller-1] (principal id)", got)
	}
	hist, err := f.h.ListPayoutHistory(ctx, &paymentv1.ListPayoutHistoryRequest{SellerId: "seller-1"})
	if err != nil || len(hist.GetPayouts()) != 1 || hist.GetPayouts()[0].GetAmount() != 300_000 {
		t.Fatalf("history = %v err=%v", hist.GetPayouts(), err)
	}
}

func TestPayoutKycGate_VerifiedSellerRequestsAWalletPayout(t *testing.T) {
	f := newKycFixture(t, "seller-1")
	ctx := sellerCtx("seller-1")
	res, err := f.h.RequestWalletPayout(ctx, &paymentv1.RequestWalletPayoutRequest{Amount: 250_000})
	if err != nil || res.GetEntry().GetAmount() != -250_000 || res.GetEntry().GetStatus() != store.LedgerStatusPending {
		t.Fatalf("entry: %v %v", res, err)
	}
	if got := f.gate.seen(); len(got) != 1 || got[0] != "seller-1" {
		t.Fatalf("gate calls = %v, want [seller-1]", got)
	}
}

func TestPayoutKycGate_GateRecoversAfterTheOutage(t *testing.T) {
	f := newKycFixture(t, "seller-1")
	ctx := sellerCtx("seller-1")
	f.gate.errFn = func(string) error { return status.Error(codes.Unavailable, "verification unavailable") }
	_, err := f.h.RequestPayout(ctx, bankReq("seller-1", 100_000))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("during outage: %v", err)
	}
	_, err = f.h.RequestWalletPayout(ctx, &paymentv1.RequestWalletPayoutRequest{Amount: 100_000})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("wallet during outage: %v", err)
	}
	f.assertUntouched(t, "seller-1", 1_000_000)
	f.gate.errFn = nil
	if _, err := f.h.RequestPayout(ctx, bankReq("seller-1", 100_000)); err != nil {
		t.Fatalf("after recovery: %v", err)
	}
}
