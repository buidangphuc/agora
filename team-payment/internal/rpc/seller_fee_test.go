package rpc_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/rpc"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
)

const feeSeller = "seller-fee-1"

type feeEnv struct {
	h      *rpc.Handler
	ledger store.LedgerRepository
	seq    int
}

func feePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v (n=%d)", err, len(files))
	}
	sort.Strings(files)
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(context.Background(), string(body)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
	return pool
}

func newFeeEnv(t *testing.T, backend string, hold time.Duration) *feeEnv {
	t.Helper()
	st := newStores()
	var ledger store.LedgerRepository = st.ledger
	if backend == "postgres" {
		ledger = store.NewPostgresLedgerRepository(feePool(t))
	}
	orders := &mockOrderClient{orders: map[string]*orderv1.Order{}}
	svc, err := payment.NewService(st.payRepo, st.wallets, orders, discardLogger(), st.writer, ledger, fake.NewInMemorySettlementLedger(st.payments, st.ledger),
		payment.WithProvider(fixtureProvider()), payment.WithPayoutHold(hold))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &feeEnv{h: newHandler(t, svc, permissiveAudit), ledger: ledger}
}

func (e *feeEnv) seed(t *testing.T, amount int64, age time.Duration) {
	t.Helper()
	e.seq++
	if _, err := e.ledger.AppendEntry(context.Background(), store.LedgerEntry{
		SellerID: feeSeller, Type: store.LedgerTypeOrderSettlement, Amount: amount,
		Status: store.LedgerStatusCompleted, ReferenceID: fmt.Sprintf("seed-tx-%d", e.seq),
		CreatedAt: time.Now().Add(-age),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func svcCtx() context.Context {
	return principalCtx("svc-promotion", auth.TypeService, "payment:write")
}

func sellerCtx() context.Context {
	return principalCtx(feeSeller, auth.TypeUser, "payment:read", "payment:write", "listing.write")
}

func feeReq(amount int64, ref string) *paymentv1.ChargeSellerFeeRequest {
	return &paymentv1.ChargeSellerFeeRequest{
		SellerId: feeSeller, Amount: amount,
		Kind: paymentv1.SellerFeeKind_SELLER_FEE_KIND_SUBSCRIPTION, ReferenceId: ref,
	}
}

func (e *feeEnv) balance(t *testing.T) int64 {
	t.Helper()
	b, err := e.h.GetWalletBalance(sellerCtx(), &paymentv1.GetWalletBalanceRequest{})
	if err != nil {
		t.Fatalf("GetWalletBalance: %v", err)
	}
	return b.GetBalance()
}

func (e *feeEnv) feeEntries(t *testing.T) []*paymentv1.WalletEntry {
	t.Helper()
	res, err := e.h.ListLedgerEntries(sellerCtx(), &paymentv1.ListLedgerEntriesRequest{Page: &commonv1.PageRequest{PageSize: 100}})
	if err != nil {
		t.Fatalf("ListLedgerEntries: %v", err)
	}
	var out []*paymentv1.WalletEntry
	for _, en := range res.GetEntries() {
		if en.GetType() == "SUBSCRIPTION_FEE" {
			out = append(out, en)
		}
	}
	return out
}

func feeBackends() []string { return []string{"memory", "postgres"} }

// seller-fee-charge req1: A fee is debited from the wallet; Replaying a reference books
// nothing; Concurrent charges of one reference book once; A replay still succeeds after the
// balance dropped; Reusing a reference with a different amount is rejected.
// req2: A fee above the balance is refused.
func TestChargeSellerFee(t *testing.T) {
	ctx := svcCtx()
	for _, backend := range feeBackends() {
		t.Run(backend, func(t *testing.T) {
			t.Run("A fee is debited from the wallet", func(t *testing.T) {
				e := newFeeEnv(t, backend, 0)
				e.seed(t, 500000, 0)
				res, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:1"))
				if err != nil {
					t.Fatalf("ChargeSellerFee: %v", err)
				}
				if !res.GetCreated() || res.GetEntryId() == "" {
					t.Fatalf("want created with entry id, got %+v", res)
				}
				fees := e.feeEntries(t)
				if len(fees) != 1 || fees[0].GetAmount() != -199000 || fees[0].GetStatus() != store.LedgerStatusCompleted ||
					fees[0].GetId() != res.GetEntryId() || fees[0].GetReferenceId() != "sub:a:1" {
					t.Fatalf("fee entries: %+v", fees)
				}
				if got := e.balance(t); got != 301000 {
					t.Fatalf("balance: want 301000, got %d", got)
				}
			})

			t.Run("Replaying a reference books nothing", func(t *testing.T) {
				e := newFeeEnv(t, backend, 0)
				e.seed(t, 500000, 0)
				first, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:1"))
				if err != nil {
					t.Fatal(err)
				}
				again, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:1"))
				if err != nil {
					t.Fatalf("replay: %v", err)
				}
				if again.GetCreated() || again.GetEntryId() != first.GetEntryId() {
					t.Fatalf("replay: want created=false id %s, got %+v", first.GetEntryId(), again)
				}
				if n := len(e.feeEntries(t)); n != 1 {
					t.Fatalf("want 1 fee entry, got %d", n)
				}
				if got := e.balance(t); got != 301000 {
					t.Fatalf("balance: want 301000, got %d", got)
				}
			})

			t.Run("Concurrent charges of one reference book once", func(t *testing.T) {
				e := newFeeEnv(t, backend, 0)
				e.seed(t, 500000, 0)
				const workers = 8
				var wg sync.WaitGroup
				var mu sync.Mutex
				created, replays := 0, 0
				for i := 0; i < workers; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						res, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:conc"))
						mu.Lock()
						defer mu.Unlock()
						switch {
						case err != nil:
							t.Errorf("concurrent charge: %v", err)
						case res.GetCreated():
							created++
						default:
							replays++
						}
					}()
				}
				wg.Wait()
				if created != 1 || replays != workers-1 {
					t.Fatalf("want 1 created and %d replays, got %d and %d", workers-1, created, replays)
				}
				fees := e.feeEntries(t)
				if len(fees) != 1 || fees[0].GetAmount() != -199000 {
					t.Fatalf("fee entries: %+v", fees)
				}
			})

			t.Run("A replay still succeeds after the balance dropped", func(t *testing.T) {
				e := newFeeEnv(t, backend, 0)
				e.seed(t, 300000, 0)
				first, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:1"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := e.h.RequestWalletPayout(sellerCtx(), &paymentv1.RequestWalletPayoutRequest{Amount: 101000}); err != nil {
					t.Fatalf("payout: %v", err)
				}
				if got := e.balance(t); got != 0 {
					t.Fatalf("balance before replay: want 0, got %d", got)
				}
				again, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:1"))
				if err != nil {
					t.Fatalf("replay after balance dropped: %v", err)
				}
				if again.GetCreated() || again.GetEntryId() != first.GetEntryId() {
					t.Fatalf("replay: want created=false id %s, got %+v", first.GetEntryId(), again)
				}
			})

			t.Run("Reusing a reference with a different amount is rejected", func(t *testing.T) {
				e := newFeeEnv(t, backend, 0)
				e.seed(t, 500000, 0)
				if _, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:1")); err != nil {
					t.Fatal(err)
				}
				_, err := e.h.ChargeSellerFee(ctx, feeReq(99000, "sub:a:1"))
				if status.Code(err) != codes.AlreadyExists {
					t.Fatalf("want AlreadyExists, got %v", err)
				}
				fees := e.feeEntries(t)
				if len(fees) != 1 || fees[0].GetAmount() != -199000 {
					t.Fatalf("fee entries: %+v", fees)
				}
				if got := e.balance(t); got != 301000 {
					t.Fatalf("balance: want 301000, got %d", got)
				}
			})

			t.Run("A fee above the balance is refused", func(t *testing.T) {
				e := newFeeEnv(t, backend, 0)
				e.seed(t, 100000, 0)
				_, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:1"))
				if status.Code(err) != codes.FailedPrecondition {
					t.Fatalf("want FailedPrecondition, got %v", err)
				}
				if n := len(e.feeEntries(t)); n != 0 {
					t.Fatalf("want no fee entry, got %d", n)
				}
				if got := e.balance(t); got != 100000 {
					t.Fatalf("balance: want 100000, got %d", got)
				}
			})
		})
	}
}

// seller-fee-charge req2: A fee against funds still on hold is refused; A fee equal to the
// withdrawable balance is accepted.
func TestChargeSellerFeeWithdrawable(t *testing.T) {
	ctx := svcCtx()
	const hold = 24 * time.Hour
	for _, backend := range feeBackends() {
		t.Run(backend, func(t *testing.T) {
			t.Run("A fee against funds still on hold is refused", func(t *testing.T) {
				e := newFeeEnv(t, backend, hold)
				e.seed(t, 50000, 3*hold)
				e.seed(t, 250000, 0)
				_, err := e.h.ChargeSellerFee(ctx, feeReq(199000, "sub:a:1"))
				if status.Code(err) != codes.FailedPrecondition {
					t.Fatalf("want FailedPrecondition, got %v", err)
				}
				if n := len(e.feeEntries(t)); n != 0 {
					t.Fatalf("want no fee entry, got %d", n)
				}
				if got := e.balance(t); got != 300000 {
					t.Fatalf("balance: want 300000, got %d", got)
				}
			})

			t.Run("A fee equal to the withdrawable balance is accepted", func(t *testing.T) {
				e := newFeeEnv(t, backend, hold)
				e.seed(t, 50000, 3*hold)
				e.seed(t, 250000, 0)
				res, err := e.h.ChargeSellerFee(ctx, feeReq(50000, "sub:a:1"))
				if err != nil || !res.GetCreated() {
					t.Fatalf("want created, got %+v err=%v", res, err)
				}
				if got := e.balance(t); got != 250000 {
					t.Fatalf("balance: want 250000 (held amount), got %d", got)
				}
				_, err = e.h.ChargeSellerFee(ctx, feeReq(1, "sub:a:2"))
				if status.Code(err) != codes.FailedPrecondition {
					t.Fatalf("fee above the withdrawable remainder: want FailedPrecondition, got %v", err)
				}
			})
		})
	}
}

// seller-fee-charge req2: Concurrent fees and payouts cannot overdraw.
func TestChargeSellerFeeConcurrentWithPayout(t *testing.T) {
	ctx := svcCtx()
	for _, backend := range feeBackends() {
		t.Run(backend, func(t *testing.T) {
			e := newFeeEnv(t, backend, 0)
			e.seed(t, 300000, 0)

			var wg sync.WaitGroup
			var mu sync.Mutex
			ok, refused := 0, 0
			record := func(err error) {
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case status.Code(err) == codes.FailedPrecondition:
					refused++
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}
			for i := 0; i < 4; i++ {
				wg.Add(2)
				go func(i int) {
					defer wg.Done()
					_, err := e.h.ChargeSellerFee(ctx, feeReq(100000, fmt.Sprintf("sub:conc:%d", i)))
					record(err)
				}(i)
				go func() {
					defer wg.Done()
					_, err := e.h.RequestWalletPayout(sellerCtx(), &paymentv1.RequestWalletPayoutRequest{Amount: 100000})
					record(err)
				}()
			}
			wg.Wait()

			if ok > 3 || ok+refused != 8 {
				t.Fatalf("want at most 3 successes of 8, got ok=%d refused=%d", ok, refused)
			}
			bal, err := e.ledger.Balance(context.Background(), feeSeller)
			if err != nil {
				t.Fatal(err)
			}
			entries, _, err := e.ledger.ListEntries(context.Background(), feeSeller, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			var sum int64
			for _, en := range entries {
				sum += en.Amount
			}
			if bal < 0 || sum != bal || bal != 300000-int64(ok)*100000 {
				t.Fatalf("balance %d, ledger sum %d, successes %d", bal, sum, ok)
			}
			for _, en := range entries {
				if en.Type == "SUBSCRIPTION_FEE" && !strings.HasPrefix(en.ReferenceID, "sub:conc:") {
					t.Fatalf("unexpected fee reference %q", en.ReferenceID)
				}
			}
		})
	}
}
