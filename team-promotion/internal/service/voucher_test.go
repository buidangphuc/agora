package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/producer"
	"github.com/buidangphuc/team-promotion/internal/repository"
)

// capturePublisher records every published record so tests can assert emission.
type capturePublisher struct {
	mu      sync.Mutex
	records []struct {
		Key   string
		Value []byte
	}
}

func (p *capturePublisher) Publish(_ context.Context, key string, value []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.records = append(p.records, struct {
		Key   string
		Value []byte
	}{key, value})
	return nil
}

func (p *capturePublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.records)
}

// newVoucherSvc builds a service over in-memory repos with a capturing emitter.
func newVoucherSvc(t *testing.T) (*VoucherService, *repository.InMemoryVoucherRepository, *capturePublisher) {
	t.Helper()
	vrepo := repository.NewInMemoryVoucherRepository()
	rrepo := repository.NewInMemoryReservationRepository()
	svc := NewVoucherService(vrepo, rrepo, nil, nil, nil)
	pub := &capturePublisher{}
	svc.emitter = producer.NewEmitter(pub, nil)
	return svc, vrepo, pub
}

func mustCreatePercentVoucher(t *testing.T, svc *VoucherService, p CreateVoucherParams) repository.Voucher {
	t.Helper()
	v, err := svc.CreateVoucher(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	return v
}

func TestCreateVoucherEmitsEvent(t *testing.T) {
	svc, _, pub := newVoucherSvc(t)
	v := mustCreatePercentVoucher(t, svc, CreateVoucherParams{
		Code:          "SAVE10",
		DiscountType:  discountTypePercent,
		DiscountValue: 10,
	})
	if v.ID == "" {
		t.Fatal("expected generated voucher id")
	}
	if pub.count() != 1 {
		t.Fatalf("expected 1 promotion.events record, got %d", pub.count())
	}
}

func TestValidateAndReserveIdempotent(t *testing.T) {
	svc, _, _ := newVoucherSvc(t)
	ctx := context.Background()
	mustCreatePercentVoucher(t, svc, CreateVoucherParams{
		Code:          "PCT20",
		DiscountType:  discountTypePercent,
		DiscountValue: 20,
		Quota:         5,
	})

	first, err := svc.ValidateAndReserve(ctx, "resv-1", "PCT20", "buyer-1", 100000, "")
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if !first.Valid || first.DiscountAmount != 20000 {
		t.Fatalf("first reserve unexpected: %+v", first)
	}

	// Same reservation_id again → same discount, one hold, quota untouched.
	second, err := svc.ValidateAndReserve(ctx, "resv-1", "PCT20", "buyer-1", 100000, "")
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if second.DiscountAmount != first.DiscountAmount || second.VoucherID != first.VoucherID {
		t.Fatalf("idempotency broken: first=%+v second=%+v", first, second)
	}

	// used must still be 0 (reserve never counts; only commit does).
	v, _ := svc.GetVoucher(ctx, "PCT20")
	if v.Used != 0 {
		t.Fatalf("expected used=0 after reserve, got %d", v.Used)
	}
}

func TestValidateAndReserveRejections(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		voucher    CreateVoucherParams
		subtotal   int64
		sellerID   string
		wantReason string
	}{
		{
			name:       "expired",
			voucher:    CreateVoucherParams{Code: "OLD", DiscountType: discountTypeFixed, DiscountValue: 500, EndsAt: now.Add(-time.Hour)},
			subtotal:   10000,
			wantReason: "voucher expired",
		},
		{
			name:       "not yet active",
			voucher:    CreateVoucherParams{Code: "FUTURE", DiscountType: discountTypeFixed, DiscountValue: 500, StartsAt: now.Add(time.Hour)},
			subtotal:   10000,
			wantReason: "voucher not yet active",
		},
		{
			name:       "below min spend",
			voucher:    CreateVoucherParams{Code: "MIN", DiscountType: discountTypeFixed, DiscountValue: 500, MinSpend: 50000},
			subtotal:   10000,
			wantReason: "order subtotal below minimum spend",
		},
		{
			name:       "unknown code",
			voucher:    CreateVoucherParams{Code: "EXISTS", DiscountType: discountTypeFixed, DiscountValue: 500},
			subtotal:   10000,
			sellerID:   "",
			wantReason: "voucher code not found",
		},
		{
			name:       "shop scope mismatch",
			voucher:    CreateVoucherParams{Code: "SHOP", Scope: voucherScopeShop, SellerID: "seller-A", DiscountType: discountTypeFixed, DiscountValue: 500},
			subtotal:   10000,
			sellerID:   "seller-B",
			wantReason: "voucher not valid for this seller",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newVoucherSvc(t)
			svc.nowFn = func() time.Time { return now }
			mustCreatePercentVoucher(t, svc, tc.voucher)

			code := tc.voucher.Code
			if tc.name == "unknown code" {
				code = "DOES-NOT-EXIST"
			}

			res, err := svc.ValidateAndReserve(ctx, "r-"+tc.name, code, "buyer", tc.subtotal, tc.sellerID)
			if err != nil {
				t.Fatalf("reserve: %v", err)
			}
			if res.Valid {
				t.Fatalf("expected rejection, got valid result: %+v", res)
			}
			if res.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", res.Reason, tc.wantReason)
			}
		})
	}
}

func TestQuotaExhaustedRejected(t *testing.T) {
	svc, _, _ := newVoucherSvc(t)
	ctx := context.Background()
	mustCreatePercentVoucher(t, svc, CreateVoucherParams{
		Code:          "ONE",
		DiscountType:  discountTypeFixed,
		DiscountValue: 1000,
		Quota:         1,
	})

	// Reserve + commit once → used becomes 1 == quota.
	if _, err := svc.ValidateAndReserve(ctx, "resv-A", "ONE", "b1", 5000, ""); err != nil {
		t.Fatalf("reserve A: %v", err)
	}
	committed, err := svc.CommitReservation(ctx, "resv-A")
	if err != nil || !committed {
		t.Fatalf("commit A: committed=%v err=%v", committed, err)
	}
	v, _ := svc.GetVoucher(ctx, "ONE")
	if v.Used != 1 {
		t.Fatalf("expected used=1 after commit, got %d", v.Used)
	}

	// A fresh reservation now hits the quota wall.
	res, err := svc.ValidateAndReserve(ctx, "resv-B", "ONE", "b2", 5000, "")
	if err != nil {
		t.Fatalf("reserve B: %v", err)
	}
	if res.Valid || res.Reason != "voucher quota exhausted" {
		t.Fatalf("expected quota exhausted rejection, got %+v", res)
	}
}

func TestCommitIdempotentAndRelease(t *testing.T) {
	svc, _, _ := newVoucherSvc(t)
	ctx := context.Background()
	mustCreatePercentVoucher(t, svc, CreateVoucherParams{Code: "C", DiscountType: discountTypeFixed, DiscountValue: 100, Quota: 10})

	if _, err := svc.ValidateAndReserve(ctx, "r", "C", "b", 5000, ""); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	// Commit twice → used incremented exactly once.
	for i := 0; i < 2; i++ {
		ok, err := svc.CommitReservation(ctx, "r")
		if err != nil || !ok {
			t.Fatalf("commit %d: ok=%v err=%v", i, ok, err)
		}
	}
	v, _ := svc.GetVoucher(ctx, "C")
	if v.Used != 1 {
		t.Fatalf("expected used=1 after double commit, got %d", v.Used)
	}
	// Releasing a committed hold is refused.
	released, err := svc.ReleaseReservation(ctx, "r")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released {
		t.Fatal("committed hold must not be releasable")
	}
}

func TestReleaseFreesHold(t *testing.T) {
	svc, _, _ := newVoucherSvc(t)
	ctx := context.Background()
	mustCreatePercentVoucher(t, svc, CreateVoucherParams{Code: "R", DiscountType: discountTypeFixed, DiscountValue: 100, Quota: 10})
	if _, err := svc.ValidateAndReserve(ctx, "r", "R", "b", 5000, ""); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	released, err := svc.ReleaseReservation(ctx, "r")
	if err != nil || !released {
		t.Fatalf("release: released=%v err=%v", released, err)
	}
	// A re-reserve on a released id is rejected (the hold is closed).
	res, err := svc.ValidateAndReserve(ctx, "r", "R", "b", 5000, "")
	if err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if res.Valid {
		t.Fatalf("expected released reservation to reject, got %+v", res)
	}
}

func TestComputeDiscount(t *testing.T) {
	cases := []struct {
		name     string
		voucher  repository.Voucher
		subtotal int64
		want     int64
	}{
		{"percent", repository.Voucher{DiscountType: discountTypePercent, DiscountValue: 10}, 100000, 10000},
		{"percent capped", repository.Voucher{DiscountType: discountTypePercent, DiscountValue: 50, MaxDiscount: 15000}, 100000, 15000},
		{"fixed", repository.Voucher{DiscountType: discountTypeFixed, DiscountValue: 20000}, 100000, 20000},
		{"fixed over subtotal", repository.Voucher{DiscountType: discountTypeFixed, DiscountValue: 20000}, 5000, 5000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeDiscount(tc.voucher, tc.subtotal); got != tc.want {
				t.Fatalf("computeDiscount = %d, want %d", got, tc.want)
			}
		})
	}
}
