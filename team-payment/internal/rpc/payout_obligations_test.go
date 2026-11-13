package rpc_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/rpc"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/migrations"
)

const (
	internalScope = "payment.internal.read"
	maskText      = "Đã xóa"
	hold          = 7 * 24 * time.Hour
)

type obligationsRig struct {
	t    *testing.T
	pool *pgxpool.Pool
	h    *rpc.Handler
}

func newObligationsRig(t *testing.T) *obligationsRig {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrate.Up(context.Background(), dsn, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ob, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := payment.NewService(
		store.NewPostgresPaymentRepository(pool), store.NewPostgresWalletRepository(pool),
		&mockOrderClient{}, discardLogger(), store.NewPgTxWriter(pool, ob),
		store.NewPostgresLedgerRepository(pool), store.NewPostgresSettlementLedger(pool),
		payment.WithPayoutHold(hold), payment.WithProvider(fixtureProvider()))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &obligationsRig{t: t, pool: pool, h: newHandler(t, svc, permissiveAudit)}
}

func (r *obligationsRig) exec(sql string, args ...any) {
	r.t.Helper()
	if _, err := r.pool.Exec(context.Background(), sql, args...); err != nil {
		r.t.Fatalf("exec: %v", err)
	}
}

func (r *obligationsRig) ledger(id, seller, typ string, amount int64, status, ref string, age time.Duration) {
	r.t.Helper()
	r.exec(`INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id, created_at)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''), now() - $7::interval)`,
		id, seller, typ, amount, status, ref, fmt.Sprintf("%d seconds", int(age.Seconds())))
}

func (r *obligationsRig) payout(id, seller string, amount int64, ledgerStatus string, payoutStatus int) {
	r.t.Helper()
	r.ledger("l-"+id, seller, store.LedgerTypePayout, -amount, ledgerStatus, "", time.Hour)
	r.exec(`INSERT INTO payout_requests (id, seller_id, amount, bank_code, account_number, account_name, status, ledger_entry_id)
		VALUES ($1,$2,$3,'VCB','0123456789','NGUYEN VAN A',$4,$5)`, id, seller, amount, payoutStatus, "l-"+id)
}

func (r *obligationsRig) seedPAY1() {
	r.ledger("c-old", "S", store.LedgerTypeOrderSettlement, 300000, store.LedgerStatusCompleted, "pay-old", 30*24*time.Hour)
	r.ledger("c-new", "S", store.LedgerTypeOrderSettlement, 200000, store.LedgerStatusCompleted, "pay-new", time.Hour)
	r.payout("p-1", "S", 50000, store.LedgerStatusPending, 1)
	r.payout("p-2", "S", 50000, store.LedgerStatusPending, 1)
}

func serviceCtx(scopes ...string) context.Context {
	return principalCtx("svc-identity", auth.TypeService, scopes...)
}

func (r *obligationsRig) call(ctx context.Context, user string) (*paymentv1.GetPayoutObligationsResponse, error) {
	return r.h.GetPayoutObligations(ctx, &paymentv1.GetPayoutObligationsRequest{UserId: user})
}

func TestGetPayoutObligationsReportsMoney(t *testing.T) {
	// PAY-1
	r := newObligationsRig(t)
	r.seedPAY1()
	res, err := r.call(serviceCtx(internalScope), "S")
	if err != nil {
		t.Fatalf("PAY-1 unexpected error: %v", err)
	}
	if res.GetAvailableBalance() != 200000 || res.GetHeldBalance() != 200000 || res.GetPendingPayouts() != 2 {
		t.Fatalf("PAY-1 got %v, want available 200000 held 200000 pending 2", res)
	}
}

func TestGetPayoutObligationsUnknownUserReadsZeros(t *testing.T) {
	// PAY-2
	r := newObligationsRig(t)
	res, err := r.call(serviceCtx(internalScope), "buyer-without-wallet")
	if err != nil {
		t.Fatalf("PAY-2 unexpected error: %v", err)
	}
	if res.GetAvailableBalance() != 0 || res.GetHeldBalance() != 0 || res.GetPendingPayouts() != 0 {
		t.Fatalf("PAY-2 got %v, want zeros", res)
	}
}

func TestGetPayoutObligationsOtherSellerNotIncluded(t *testing.T) {
	// PAY-5
	r := newObligationsRig(t)
	r.ledger("c-s", "S", store.LedgerTypeOrderSettlement, 100000, store.LedgerStatusCompleted, "pay-s", 30*24*time.Hour)
	r.ledger("c-t", "T", store.LedgerTypeOrderSettlement, 900000, store.LedgerStatusCompleted, "pay-t", time.Hour)
	res, err := r.call(serviceCtx(internalScope), "S")
	if err != nil {
		t.Fatal(err)
	}
	if res.GetAvailableBalance() != 100000 || res.GetHeldBalance() != 0 {
		t.Fatalf("PAY-5 got %v, want available 100000 held 0", res)
	}
}

func TestGetPayoutObligationsRejectsEmptyUserID(t *testing.T) {
	// PAY-6
	r := newObligationsRig(t)
	_, err := r.call(serviceCtx(internalScope), "")
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("PAY-6 code = %v (%v), want InvalidArgument", got, err)
	}
}

func TestGetPayoutObligationsRefusesCallers(t *testing.T) {
	r := newObligationsRig(t)
	r.seedPAY1()
	cases := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"PAY-7 user principal", principalCtx("S", auth.TypeUser, "seller", "listing.write", internalScope), codes.PermissionDenied},
		{"PAY-7 admin user principal", principalCtx("admin-1", auth.TypeUser, "admin"), codes.PermissionDenied},
		{"PAY-8 service without the scope", serviceCtx("order.read"), codes.PermissionDenied},
		{"PAY-8 service with no scopes", serviceCtx(), codes.PermissionDenied},
		{"PAY-9 anonymous principal", principalCtx("anonymous", auth.TypeAnonymous), codes.Unauthenticated},
		{"PAY-9 no principal", context.Background(), codes.Unauthenticated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := r.call(c.ctx, "S")
			if got := status.Code(err); got != c.want {
				t.Fatalf("code = %v (%v), want %v", got, err, c.want)
			}
			if res != nil {
				t.Fatalf("a refused call returned balances: %v", res)
			}
		})
	}
}

func TestGetPayoutObligationsStoreFailure(t *testing.T) {
	// PAY-11
	r := newObligationsRig(t)
	r.seedPAY1()
	r.pool.Close()
	res, err := r.call(serviceCtx(internalScope), "S")
	if err == nil || status.Code(err) == codes.OK {
		t.Fatalf("PAY-11 err = %v, want a non-OK status", err)
	}
	if res != nil {
		t.Fatalf("PAY-11 failure carried balances: %v", res)
	}
}

func TestGetPayoutObligationsOverGRPC(t *testing.T) {
	// PAY-9 through the real interceptor chain
	r := newObligationsRig(t)
	r.seedPAY1()
	client := startServer(t, r.h)
	if _, err := client.GetPayoutObligations(context.Background(), &paymentv1.GetPayoutObligationsRequest{UserId: "S"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("PAY-9 over gRPC code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestListPayoutHistoryShowsMaskAfterErasure(t *testing.T) {
	// PER-8
	r := newObligationsRig(t)
	r.seedPAY1()
	if err := store.NewPostgresWalletRepository(r.pool).EraseSellerPayouts(context.Background(), "S"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	res, err := r.h.ListPayoutHistory(principalCtx("admin-1", auth.TypeUser, "admin"), &paymentv1.ListPayoutHistoryRequest{SellerId: "S"})
	if err != nil {
		t.Fatalf("PER-8 ListPayoutHistory: %v", err)
	}
	if len(res.GetPayouts()) != 2 {
		t.Fatalf("PER-8 history entries = %d, want 2", len(res.GetPayouts()))
	}
	for _, p := range res.GetPayouts() {
		if p.GetAccountNumber() != maskText || p.GetAccountName() != maskText {
			t.Errorf("PER-8 %s shows %q / %q, want the mask", p.GetId(), p.GetAccountNumber(), p.GetAccountName())
		}
		if p.GetAmount() != 50000 {
			t.Errorf("PER-8 %s amount = %d, want 50000", p.GetId(), p.GetAmount())
		}
	}
}
