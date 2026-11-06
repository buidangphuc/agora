package voucher

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

const (
	reasonNotEligible  = "VOUCHER_NOT_ELIGIBLE"
	reasonPerUserLimit = "VOUCHER_PER_USER_LIMIT"
	reasonMinSpend     = "order subtotal below minimum spend"
)

var userT0 = time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)

type fakeOrders struct {
	mu    sync.Mutex
	calls int
	buyer []string
	has   bool
	err   error
}

func (f *fakeOrders) HasCompletedOrder(_ context.Context, buyerID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.buyer = append(f.buyer, buyerID)
	return f.has, f.err
}

func (f *fakeOrders) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type countingReservations struct {
	store.ReservationRepository
	mu     sync.Mutex
	counts int
}

func (c *countingReservations) CountActiveByBuyer(ctx context.Context, voucherID, buyerID string, now time.Time) (int, error) {
	c.mu.Lock()
	c.counts++
	c.mu.Unlock()
	return c.ReservationRepository.CountActiveByBuyer(ctx, voucherID, buyerID, now)
}

type userEnv struct {
	svc    *Service
	vrepo  *storefake.InMemoryVoucherRepository
	rrepo  *storefake.InMemoryReservationRepository
	orders *fakeOrders
	clock  *time.Time
}

func newUserEnv(t *testing.T) *userEnv {
	t.Helper()
	vrepo := storefake.NewInMemoryVoucherRepository()
	rrepo := storefake.NewInMemoryReservationRepository()
	orders := &fakeOrders{}
	svc := New(vrepo, rrepo, auditx.NewGate(&recordingAuditWriter{}, nil), (&eventLog{}).build, nil).WithOrderClient(orders)
	clock := userT0
	svc.nowFn = func() time.Time { return clock }
	return &userEnv{svc: svc, vrepo: vrepo, rrepo: rrepo, orders: orders, clock: &clock}
}

func (e *userEnv) at(t time.Time) { *e.clock = t }

func (e *userEnv) seed(t *testing.T, v store.Voucher) store.Voucher {
	t.Helper()
	if v.Scope == 0 {
		v.Scope = scopePlatformWant
	}
	if v.DiscountType == 0 {
		v.DiscountType = discountTypeFixed
	}
	if v.StartsAt.IsZero() {
		v.StartsAt = userT0.Add(-time.Hour)
	}
	if v.EndsAt.IsZero() {
		v.EndsAt = userT0.Add(336 * time.Hour)
	}
	if v.Kind == 0 {
		v.Kind = store.VoucherKindStandard
	}
	out, err := e.vrepo.Create(t.Context(), v, func(store.Voucher) (outbox.Row, error) { return outbox.Row{}, nil })
	if err != nil {
		t.Fatalf("seed %s: %v", v.Code, err)
	}
	return out
}

func (e *userEnv) newUser(t *testing.T, code, owner string, amount int64) store.Voucher {
	t.Helper()
	return e.seed(t, store.Voucher{
		Code: code, DiscountValue: amount, MinSpend: 100000, Quota: 1, PerUserLimit: 1,
		OwnerUserID: owner, Kind: store.VoucherKindNewUser,
	})
}

func (e *userEnv) reserve(t *testing.T, id, code, buyer string, subtotal int64) ReserveResult {
	t.Helper()
	res, err := e.svc.ValidateAndReserve(t.Context(), id, code, buyer, subtotal, "")
	if err != nil {
		t.Fatalf("reserve %s: %v", id, err)
	}
	return res
}

func assertRefused(t *testing.T, got ReserveResult, reason string) {
	t.Helper()
	if got.Valid || got.Reason != reason {
		t.Fatalf("result = %+v, want valid=false reason %q", got, reason)
	}
}

func (e *userEnv) assertNoHold(t *testing.T, id string) {
	t.Helper()
	if _, err := e.rrepo.GetByReservationID(t.Context(), id); err == nil {
		t.Fatalf("hold %q exists, want none", id)
	}
}

func TestOwnerMismatchRefused(t *testing.T) {
	e := newUserEnv(t)
	e.newUser(t, "NEWU-A", "user-a", 20000)
	got := e.reserve(t, "r-b", "NEWU-A", "user-b", 200000)
	assertRefused(t, got, reasonNotEligible)
	e.assertNoHold(t, "r-b")
	if n := e.orders.count(); n != 0 {
		t.Fatalf("order client called %d times for an owner mismatch", n)
	}
}

func TestSingleUseOwnerVoucher(t *testing.T) {
	for _, amount := range []int64{20000, 30000} {
		e := newUserEnv(t)
		e.newUser(t, "NEWU-A", "user-a", amount)
		first := e.reserve(t, "r1", "NEWU-A", "user-a", 200000)
		if !first.Valid || first.DiscountAmount != amount {
			t.Fatalf("first reserve = %+v, want valid with discount %d", first, amount)
		}
		if ok, err := e.svc.CommitReservation(t.Context(), "r1"); err != nil || !ok {
			t.Fatalf("commit: %v %v", ok, err)
		}
		second := e.reserve(t, "r2", "NEWU-A", "user-a", 200000)
		assertRefused(t, second, reasonPerUserLimit)
		e.assertNoHold(t, "r2")
	}
}

func TestNewUserMinSpendBoundary(t *testing.T) {
	e := newUserEnv(t)
	e.newUser(t, "NEWU-A", "user-a", 20000)
	low := e.reserve(t, "r1", "NEWU-A", "user-a", 99999)
	assertRefused(t, low, reasonMinSpend)
	e.assertNoHold(t, "r1")
	ok := e.reserve(t, "r2", "NEWU-A", "user-a", 100000)
	if !ok.Valid || ok.DiscountAmount != 20000 {
		t.Fatalf("at min spend = %+v, want valid with 20000", ok)
	}
}

func TestPerUserLimitCountsLiveHolds(t *testing.T) {
	e := newUserEnv(t)
	e.seed(t, store.Voucher{Code: "PU2", DiscountValue: 1000, Quota: 10, PerUserLimit: 2})
	for _, id := range []string{"r1", "r2"} {
		if got := e.reserve(t, id, "PU2", "buyer-b", 5000); !got.Valid {
			t.Fatalf("%s = %+v, want valid", id, got)
		}
	}
	if ok, err := e.svc.ReleaseReservation(t.Context(), "r1"); err != nil || !ok {
		t.Fatalf("release r1: %v %v", ok, err)
	}
	if got := e.reserve(t, "r3", "PU2", "buyer-b", 5000); !got.Valid {
		t.Fatalf("r3 after release = %+v, want valid (count was 1)", got)
	}
	assertRefused(t, e.reserve(t, "r4", "PU2", "buyer-b", 5000), reasonPerUserLimit)
	e.assertNoHold(t, "r4")
	if got := e.reserve(t, "r5", "PU2", "buyer-c", 5000); !got.Valid {
		t.Fatalf("another buyer must have its own count: %+v", got)
	}
}

func TestExpiredHoldNotCounted(t *testing.T) {
	e := newUserEnv(t)
	e.seed(t, store.Voucher{Code: "PU1", DiscountValue: 1000, Quota: 10, PerUserLimit: 1})
	if got := e.reserve(t, "r1", "PU1", "buyer-b", 5000); !got.Valid {
		t.Fatalf("r1 = %+v", got)
	}
	hold, err := e.rrepo.GetByReservationID(t.Context(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	expires := userT0.Add(15 * time.Minute)
	if !hold.ExpiresAt.Equal(expires) {
		t.Fatalf("hold expires_at = %v, want now + 15m = %v", hold.ExpiresAt, expires)
	}
	e.at(expires.Add(-time.Second))
	assertRefused(t, e.reserve(t, "r2", "PU1", "buyer-b", 5000), reasonPerUserLimit)
	e.at(expires)
	if got := e.reserve(t, "r3", "PU1", "buyer-b", 5000); !got.Valid {
		t.Fatalf("reserve at expiry = %+v, want valid", got)
	}
}

func TestCommittedHoldCountsPastExpiry(t *testing.T) {
	e := newUserEnv(t)
	e.seed(t, store.Voucher{Code: "PU1", DiscountValue: 1000, Quota: 10, PerUserLimit: 1})
	if got := e.reserve(t, "r1", "PU1", "buyer-b", 5000); !got.Valid {
		t.Fatalf("r1 = %+v", got)
	}
	if ok, err := e.svc.CommitReservation(t.Context(), "r1"); err != nil || !ok {
		t.Fatalf("commit: %v %v", ok, err)
	}
	e.at(userT0.Add(15*time.Minute + time.Hour))
	assertRefused(t, e.reserve(t, "r2", "PU1", "buyer-b", 5000), reasonPerUserLimit)
}

func TestZeroPerUserLimitUnchanged(t *testing.T) {
	vrepo := storefake.NewInMemoryVoucherRepository()
	rrepo := &countingReservations{ReservationRepository: storefake.NewInMemoryReservationRepository()}
	orders := &fakeOrders{}
	svc := New(vrepo, rrepo, auditx.NewGate(&recordingAuditWriter{}, nil), (&eventLog{}).build, nil).WithOrderClient(orders)
	svc.nowFn = func() time.Time { return userT0 }
	if _, err := vrepo.Create(t.Context(), store.Voucher{
		Code: "Q5", Scope: scopePlatformWant, DiscountType: discountTypeFixed, DiscountValue: 1000, Quota: 5,
		Kind: store.VoucherKindStandard,
	}, func(store.Voucher) (outbox.Row, error) { return outbox.Row{}, nil }); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"r1", "r2", "r3"} {
		got, err := svc.ValidateAndReserve(t.Context(), id, "Q5", "buyer-b", 5000, "")
		if err != nil || !got.Valid || got.DiscountAmount != 1000 {
			t.Fatalf("%s = %+v err=%v, want valid 1000", id, got, err)
		}
	}
	if rrepo.counts != 0 {
		t.Fatalf("per-user count queries = %d, want 0 when per_user_limit is 0", rrepo.counts)
	}
	if n := orders.count(); n != 0 {
		t.Fatalf("order client calls = %d, want 0", n)
	}
}

func TestNewUserOnlyRefusesExistingCustomer(t *testing.T) {
	e := newUserEnv(t)
	e.orders.has = true
	e.seed(t, store.Voucher{Code: "NUO", DiscountValue: 1000, Quota: 10, NewUserOnly: true})
	got := e.reserve(t, "r1", "NUO", "buyer-b", 5000)
	assertRefused(t, got, reasonNotEligible)
	e.assertNoHold(t, "r1")
	if n := e.orders.count(); n != 1 {
		t.Fatalf("order client calls = %d, want 1", n)
	}
}

func TestNewUserOnlyAcceptsNewBuyer(t *testing.T) {
	e := newUserEnv(t)
	e.seed(t, store.Voucher{Code: "NUO", DiscountValue: 1000, MinSpend: 1000, Quota: 10, NewUserOnly: true})
	got := e.reserve(t, "r1", "NUO", "buyer-b", 5000)
	if !got.Valid || got.DiscountAmount != 1000 {
		t.Fatalf("result = %+v, want valid with discount 1000", got)
	}
	if n := e.orders.count(); n != 1 {
		t.Fatalf("order client calls = %d, want exactly 1", n)
	}
	if e.orders.buyer[0] != "buyer-b" {
		t.Fatalf("order client asked about %q, want buyer-b", e.orders.buyer[0])
	}
}

func TestNewUserOnlyFailsClosed(t *testing.T) {
	for name, orderErr := range map[string]error{
		"plain error":       errors.New("dial tcp: refused"),
		"permission":        status.Error(codes.PermissionDenied, "missing order.read"),
		"unauthenticated":   status.Error(codes.Unauthenticated, "no principal"),
		"unavailable":       status.Error(codes.Unavailable, "order down"),
		"deadline":          context.DeadlineExceeded,
		"status deadline":   status.Error(codes.DeadlineExceeded, "slow"),
		"internal":          status.Error(codes.Internal, "boom"),
		"nil client (addr)": nil,
	} {
		t.Run(name, func(t *testing.T) {
			e := newUserEnv(t)
			e.seed(t, store.Voucher{Code: "NUO", DiscountValue: 1000, Quota: 10, NewUserOnly: true})
			if orderErr == nil {
				e.svc.WithOrderClient(nil)
			} else {
				e.orders.err = orderErr
			}
			got, err := e.svc.ValidateAndReserve(t.Context(), "r1", "NUO", "buyer-b", 5000, "")
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("err = %v (result %+v), want gRPC Unavailable", err, got)
			}
			if got.Valid {
				t.Fatalf("result %+v is valid, want fail closed", got)
			}
			e.assertNoHold(t, "r1")
		})
	}
}

func TestOrderNotCalledWithoutFlag(t *testing.T) {
	e := newUserEnv(t)
	e.orders.err = status.Error(codes.Unavailable, "order down")
	e.seed(t, store.Voucher{Code: "PLAIN", DiscountValue: 1000, Quota: 10})
	if got := e.reserve(t, "r1", "PLAIN", "buyer-b", 5000); !got.Valid {
		t.Fatalf("result = %+v, want valid", got)
	}
	if n := e.orders.count(); n != 0 {
		t.Fatalf("order client calls = %d, want 0", n)
	}
	e.svc.WithOrderClient(nil)
	if got := e.reserve(t, "r2", "PLAIN", "buyer-b", 5000); !got.Valid {
		t.Fatalf("without an order client: %+v, want valid", got)
	}
}

func TestPerUserLimitBeforeOrderCall(t *testing.T) {
	e := newUserEnv(t)
	e.seed(t, store.Voucher{Code: "BOTH", DiscountValue: 1000, Quota: 10, PerUserLimit: 1, NewUserOnly: true})
	if got := e.reserve(t, "r1", "BOTH", "buyer-b", 5000); !got.Valid {
		t.Fatalf("r1 = %+v", got)
	}
	before := e.orders.count()
	assertRefused(t, e.reserve(t, "r2", "BOTH", "buyer-b", 5000), reasonPerUserLimit)
	if e.orders.count() != before {
		t.Fatal("the order service was asked after the per-user limit already refused")
	}
}

func TestReplayBeforePerUserChecks(t *testing.T) {
	e := newUserEnv(t)
	e.seed(t, store.Voucher{Code: "BOTH", DiscountValue: 1000, Quota: 10, PerUserLimit: 1, NewUserOnly: true})
	first := e.reserve(t, "r1", "BOTH", "buyer-b", 5000)
	if !first.Valid {
		t.Fatalf("first = %+v", first)
	}
	assertRefused(t, e.reserve(t, "r2", "BOTH", "buyer-b", 5000), reasonPerUserLimit)

	e.orders.err = status.Error(codes.Unavailable, "order down")
	before := e.orders.count()
	replay, err := e.svc.ValidateAndReserve(t.Context(), "r1", "BOTH", "buyer-b", 5000, "")
	if err != nil {
		t.Fatalf("replay err = %v, want none", err)
	}
	if !replay.Valid || replay.DiscountAmount != first.DiscountAmount || replay.VoucherID != first.VoucherID {
		t.Fatalf("replay = %+v, want original %+v", replay, first)
	}
	if e.orders.count() != before {
		t.Fatal("replay called the order service")
	}
}
