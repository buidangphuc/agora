package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-promotion/internal/producer"
	"github.com/buidangphuc/team-promotion/internal/repository"
	"github.com/buidangphuc/team-promotion/internal/upstream"
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
	svc.WithAudit(permissiveAudit{}) // existing tests create unspecified-scope (platform) vouchers
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

// ---- promotion-admin-audit: shared recording fakes -------------------------

// callLog records the order of audit writes and repository inserts.
type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) add(c string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, c)
}

func (l *callLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

// recordingAuditWriter is the AuditWriter fake: records every event and the call
// order; err makes it fail.
type recordingAuditWriter struct {
	log    *callLog
	err    error
	mu     sync.Mutex
	events []upstream.AuditEvent
}

func (w *recordingAuditWriter) WriteEvent(_ context.Context, ev upstream.AuditEvent) error {
	if w.log != nil {
		w.log.add("audit")
	}
	w.mu.Lock()
	w.events = append(w.events, ev)
	w.mu.Unlock()
	return w.err
}

func (w *recordingAuditWriter) got() []upstream.AuditEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]upstream.AuditEvent(nil), w.events...)
}

// permissiveAudit accepts everything; for tests that are not about audit.
type permissiveAudit struct{}

func (permissiveAudit) WriteEvent(context.Context, upstream.AuditEvent) error { return nil }

// auditFailures are the ways an admin audit write can fail; each must fail closed.
var auditFailures = map[string]error{
	"upstream error": status.Error(codes.Internal, "audit db down"),
	"timeout":        context.DeadlineExceeded,
	"unavailable":    status.Error(codes.Unavailable, "team-audit down"),
	"plain error":    errors.New("dial tcp: refused"),
}

type orderedVoucherRepo struct {
	repository.VoucherRepository
	log *callLog
}

func (r orderedVoucherRepo) Create(ctx context.Context, v repository.Voucher) (repository.Voucher, error) {
	r.log.add("insert")
	return r.VoucherRepository.Create(ctx, v)
}

func newAuditedVoucherSvc(t *testing.T, w AuditWriter, log *callLog) (*VoucherService, *repository.InMemoryVoucherRepository, *capturePublisher) {
	t.Helper()
	mem := repository.NewInMemoryVoucherRepository()
	svc := NewVoucherService(orderedVoucherRepo{mem, log}, repository.NewInMemoryReservationRepository(), nil, nil, nil)
	if w != nil {
		svc.WithAudit(w)
	}
	pub := &capturePublisher{}
	svc.emitter = producer.NewEmitter(pub, nil)
	return svc, mem, pub
}

func platformParams(code string) CreateVoucherParams {
	return CreateVoucherParams{
		Code: code, Scope: 2, DiscountType: discountTypePercent, DiscountValue: 10, Quota: 5,
		AuditActorID: "admin-1",
	}
}

func assertVoucherAbsent(t *testing.T, mem *repository.InMemoryVoucherRepository, code string) {
	t.Helper()
	if _, err := mem.GetByCode(context.Background(), code); !errors.Is(err, repository.ErrVoucherNotFound) {
		t.Fatalf("voucher %q must not exist, GetByCode err = %v", code, err)
	}
}

// Scenario: Promotion audit precedes the voucher insert.
func TestCreateVoucherAuditBeforeInsert(t *testing.T) {
	for name, scope := range map[string]int32{"platform": 2, "unspecified": 0} {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			w := &recordingAuditWriter{log: log}
			svc, mem, pub := newAuditedVoucherSvc(t, w, log)
			p := platformParams("PLAT10")
			p.Scope = scope
			if _, err := svc.CreateVoucher(context.Background(), p); err != nil {
				t.Fatalf("CreateVoucher: %v", err)
			}
			if got := log.get(); len(got) != 2 || got[0] != "audit" || got[1] != "insert" {
				t.Fatalf("call order = %v, want [audit insert]", got)
			}
			evs := w.got()
			if len(evs) != 1 {
				t.Fatalf("want exactly 1 audit event, got %d", len(evs))
			}
			ev := evs[0]
			if ev.Action != "voucher.platform_create" || ev.TargetType != "voucher" || ev.TargetID != "PLAT10" || ev.ActorID != "admin-1" {
				t.Errorf("event = %+v", ev)
			}
			if len(ev.Metadata) != 2 || ev.Metadata["discount_type"] != "DISCOUNT_TYPE_PERCENT" || ev.Metadata["value"] != "10" {
				t.Errorf("metadata = %v, want discount_type+value only", ev.Metadata)
			}
			if _, err := mem.GetByCode(context.Background(), "PLAT10"); err != nil {
				t.Errorf("voucher not stored: %v", err)
			}
			if pub.count() != 1 {
				t.Errorf("VoucherChanged emitted %d times, want 1", pub.count())
			}
		})
	}
}

// Scenario: Promotion audit precedes the voucher insert (target is the trimmed
// code; fixed discount maps to its enum name).
func TestCreateVoucherAuditTargetIsTrimmedCodeAndFixedType(t *testing.T) {
	w := &recordingAuditWriter{}
	svc, _, _ := newAuditedVoucherSvc(t, w, &callLog{})
	p := platformParams("  FIX5  ")
	p.DiscountType, p.DiscountValue = discountTypeFixed, 500
	if _, err := svc.CreateVoucher(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	evs := w.got()
	if len(evs) != 1 || evs[0].TargetID != "FIX5" || evs[0].Metadata["discount_type"] != "DISCOUNT_TYPE_FIXED" || evs[0].Metadata["value"] != "500" {
		t.Fatalf("events = %+v", evs)
	}
}

// Scenario: Promotion audit upstream not configured.
func TestCreateVoucherNilAuditUnavailable(t *testing.T) {
	for _, scope := range []int32{2, 0} {
		svc, mem, pub := newAuditedVoucherSvc(t, nil, &callLog{})
		p := platformParams("NOAUDIT")
		p.Scope = scope
		_, err := svc.CreateVoucher(context.Background(), p)
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("scope %d: code = %v, want Unavailable (err=%v)", scope, status.Code(err), err)
		}
		assertVoucherAbsent(t, mem, "NOAUDIT")
		if pub.count() != 0 {
			t.Errorf("nothing may be emitted, got %d", pub.count())
		}
	}
}

// Scenario: Promotion audit timeout blocks the voucher (service half: any audit
// failure, including a deadline, fails closed).
func TestCreateVoucherAuditErrorFailsClosed(t *testing.T) {
	for name, cause := range auditFailures {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			svc, mem, pub := newAuditedVoucherSvc(t, &recordingAuditWriter{log: log, err: cause}, log)
			_, err := svc.CreateVoucher(context.Background(), platformParams("PLAT-DOWN"))
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("code = %v, want Unavailable (err=%v)", status.Code(err), err)
			}
			if got := log.get(); len(got) != 1 || got[0] != "audit" {
				t.Fatalf("call order = %v, want [audit] (no insert)", got)
			}
			assertVoucherAbsent(t, mem, "PLAT-DOWN")
			if pub.count() != 0 {
				t.Errorf("nothing may be emitted, got %d", pub.count())
			}
		})
	}
}

// Scenario: Shop voucher by a seller writes no event (and needs no audit upstream).
func TestCreateVoucherShopScopeNotAudited(t *testing.T) {
	for name, w := range map[string]*recordingAuditWriter{"writer wired": {}, "writer failing": {err: errors.New("down")}} {
		t.Run(name, func(t *testing.T) {
			svc, mem, _ := newAuditedVoucherSvc(t, w, &callLog{})
			p := platformParams("SHOP1")
			p.Scope, p.SellerID = voucherScopeShop, "seller-1"
			if _, err := svc.CreateVoucher(context.Background(), p); err != nil {
				t.Fatalf("shop voucher: %v", err)
			}
			if n := len(w.got()); n != 0 {
				t.Fatalf("shop voucher wrote %d audit events, want 0", n)
			}
			if _, err := mem.GetByCode(context.Background(), "SHOP1"); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Nil writer: still fine for the shop path.
	svc, _, _ := newAuditedVoucherSvc(t, nil, &callLog{})
	p := platformParams("SHOP2")
	p.Scope, p.SellerID = voucherScopeShop, "seller-1"
	if _, err := svc.CreateVoucher(context.Background(), p); err != nil {
		t.Fatalf("shop voucher with nil audit: %v", err)
	}
}

// Scenario: Invalid platform voucher writes no event (service half).
func TestCreateVoucherInvalidCodeNotAudited(t *testing.T) {
	for _, code := range []string{"", "   "} {
		w := &recordingAuditWriter{}
		svc, _, _ := newAuditedVoucherSvc(t, w, &callLog{})
		if _, err := svc.CreateVoucher(context.Background(), platformParams(code)); !errors.Is(err, ErrInvalidVoucher) {
			t.Fatalf("code %q: err = %v, want ErrInvalidVoucher", code, err)
		}
		if n := len(w.got()); n != 0 {
			t.Fatalf("code %q: invalid voucher wrote %d events", code, n)
		}
	}
	// Nil writer + invalid input stays InvalidArgument-class, not Unavailable.
	svc, _, _ := newAuditedVoucherSvc(t, nil, &callLog{})
	if _, err := svc.CreateVoucher(context.Background(), platformParams("")); !errors.Is(err, ErrInvalidVoucher) {
		t.Fatalf("nil audit + empty code: err = %v, want ErrInvalidVoucher", err)
	}
}

// Scenario: Promotion audit precedes the voucher insert (insert failure after a
// successful audit is not Unavailable; the trail keeps the entry, design D7).
func TestCreateVoucherInsertFailureAfterAuditIsNotUnavailable(t *testing.T) {
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	svc, _, _ := newAuditedVoucherSvc(t, w, log)
	if _, err := svc.CreateVoucher(context.Background(), platformParams("DUP")); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateVoucher(context.Background(), platformParams("DUP"))
	if err == nil || status.Code(err) == codes.Unavailable || strings.TrimSpace(err.Error()) == "" {
		t.Fatalf("duplicate insert: err = %v, want a non-Unavailable error", err)
	}
	if n := len(w.got()); n != 2 {
		t.Fatalf("audit events = %d, want 2 (audit precedes every insert attempt)", n)
	}
}

func shopAdminParams(code string) CreateVoucherParams {
	return CreateVoucherParams{
		Code: code, Scope: voucherScopeShop, SellerID: "admin-1", DiscountType: discountTypePercent,
		DiscountValue: 10, Quota: 5, AuditActorID: "admin-1", AuditAdmin: true,
	}
}

// Scenario: Shop voucher audit: audit precedes the insert.
func TestCreateVoucherShopAdminAuditBeforeInsert(t *testing.T) {
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	svc, mem, pub := newAuditedVoucherSvc(t, w, log)
	if _, err := svc.CreateVoucher(context.Background(), shopAdminParams("SHOP-ADM10")); err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	if got := log.get(); len(got) != 2 || got[0] != "audit" || got[1] != "insert" {
		t.Fatalf("call order = %v, want [audit insert]", got)
	}
	evs := w.got()
	if len(evs) != 1 {
		t.Fatalf("want exactly 1 audit event, got %d", len(evs))
	}
	ev := evs[0]
	if ev.Action != "voucher.admin_create" || ev.TargetType != "voucher" || ev.TargetID != "SHOP-ADM10" || ev.ActorID != "admin-1" {
		t.Errorf("event = %+v", ev)
	}
	if len(ev.Metadata) != 3 || ev.Metadata["discount_type"] != "DISCOUNT_TYPE_PERCENT" ||
		ev.Metadata["value"] != "10" || ev.Metadata["seller_id"] != "admin-1" {
		t.Errorf("metadata = %v, want discount_type, value, seller_id", ev.Metadata)
	}
	if _, err := mem.GetByCode(context.Background(), "SHOP-ADM10"); err != nil {
		t.Errorf("voucher not stored: %v", err)
	}
	if pub.count() != 1 {
		t.Errorf("VoucherChanged emitted %d times, want 1", pub.count())
	}
}

// Scenario: Shop voucher audit: audit not configured.
func TestCreateVoucherShopAdminNilAuditUnavailable(t *testing.T) {
	svc, mem, pub := newAuditedVoucherSvc(t, nil, &callLog{})
	_, err := svc.CreateVoucher(context.Background(), shopAdminParams("SHOP-NOAUDIT"))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable (err=%v)", status.Code(err), err)
	}
	assertVoucherAbsent(t, mem, "SHOP-NOAUDIT")
	if pub.count() != 0 {
		t.Errorf("nothing may be emitted, got %d", pub.count())
	}
}

// Scenario: Shop voucher audit: audit error blocks the voucher.
func TestCreateVoucherShopAdminAuditErrorFailsClosed(t *testing.T) {
	for name, cause := range auditFailures {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			svc, mem, pub := newAuditedVoucherSvc(t, &recordingAuditWriter{log: log, err: cause}, log)
			_, err := svc.CreateVoucher(context.Background(), shopAdminParams("SHOP-DOWN"))
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("code = %v, want Unavailable (err=%v)", status.Code(err), err)
			}
			if got := log.get(); len(got) != 1 || got[0] != "audit" {
				t.Fatalf("call order = %v, want [audit] (no insert)", got)
			}
			assertVoucherAbsent(t, mem, "SHOP-DOWN")
			if pub.count() != 0 {
				t.Errorf("nothing may be emitted, got %d", pub.count())
			}
		})
	}
}

// Scenario: Shop voucher audit: platform scope keeps its action (AuditAdmin set
// must not switch the action).
func TestCreateVoucherPlatformStillPlatformAction(t *testing.T) {
	w := &recordingAuditWriter{}
	svc, _, _ := newAuditedVoucherSvc(t, w, &callLog{})
	p := platformParams("PLAT-KEEP")
	p.AuditAdmin = true
	if _, err := svc.CreateVoucher(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	evs := w.got()
	if len(evs) != 1 || evs[0].Action != "voucher.platform_create" {
		t.Fatalf("events = %+v, want exactly one voucher.platform_create", evs)
	}
}

const (
	scopeUnspecifiedWant int32 = 0
	scopeShopWant        int32 = 1
	scopePlatformWant    int32 = 2
)

type scopeVoucherRepo struct {
	repository.VoucherRepository
	getByIDErr error
}

func (r scopeVoucherRepo) GetByID(ctx context.Context, id string) (repository.Voucher, error) {
	if r.getByIDErr != nil {
		return repository.Voucher{}, r.getByIDErr
	}
	return r.VoucherRepository.GetByID(ctx, id)
}

func newScopeSvc(t *testing.T, getByIDErr error) (*VoucherService, *repository.InMemoryReservationRepository) {
	t.Helper()
	vrepo := scopeVoucherRepo{VoucherRepository: repository.NewInMemoryVoucherRepository(), getByIDErr: getByIDErr}
	rrepo := repository.NewInMemoryReservationRepository()
	svc := NewVoucherService(vrepo, rrepo, nil, nil, nil)
	svc.WithAudit(permissiveAudit{})
	svc.emitter = producer.NewEmitter(&capturePublisher{}, nil)
	return svc, rrepo
}

func scopeShopVoucher(t *testing.T, svc *VoucherService, code, seller string, mod func(*CreateVoucherParams)) repository.Voucher {
	t.Helper()
	p := CreateVoucherParams{Code: code, Scope: scopeShopWant, SellerID: seller, DiscountType: discountTypeFixed, DiscountValue: 1000, Quota: 5}
	if mod != nil {
		mod(&p)
	}
	return mustCreatePercentVoucher(t, svc, p)
}

func scopePlatformVoucher(t *testing.T, svc *VoucherService, code string, mod func(*CreateVoucherParams)) repository.Voucher {
	t.Helper()
	p := CreateVoucherParams{Code: code, Scope: scopePlatformWant, DiscountType: discountTypeFixed, DiscountValue: 1000, Quota: 5, AuditActorID: "admin-1"}
	if mod != nil {
		mod(&p)
	}
	return mustCreatePercentVoucher(t, svc, p)
}

func assertScope(t *testing.T, got ReserveResult, scope int32, seller string) {
	t.Helper()
	if got.Scope != scope || got.SellerID != seller {
		t.Fatalf("scope/seller = %v/%q, want %v/%q (result %+v)", got.Scope, got.SellerID, scope, seller, got)
	}
}

func assertNoHold(t *testing.T, rrepo *repository.InMemoryReservationRepository, id string) {
	t.Helper()
	if _, err := rrepo.GetByReservationID(context.Background(), id); err == nil {
		t.Fatalf("hold %q exists, want none", id)
	}
}

func TestValidateAndReserveReportsShopScope(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopeShopVoucher(t, svc, "SHOP", "S1", func(p *CreateVoucherParams) { p.MinSpend = 1000 })
	res, err := svc.ValidateAndReserve(context.Background(), "r1", "SHOP", "b", 5000, "S1")
	if err != nil || !res.Valid {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	assertScope(t, res, scopeShopWant, "S1")
}

func TestValidateAndReservePlatformScopeNoSeller(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopePlatformVoucher(t, svc, "PLAT", nil)
	res, err := svc.ValidateAndReserve(context.Background(), "r1", "PLAT", "b", 5000, "")
	if err != nil || !res.Valid {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	assertScope(t, res, scopePlatformWant, "")
}

func TestValidateAndReservePlatformIgnoresRequestSeller(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopePlatformVoucher(t, svc, "PLAT", nil)
	res, err := svc.ValidateAndReserve(context.Background(), "r1", "PLAT", "b", 5000, "S9")
	if err != nil || !res.Valid {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	assertScope(t, res, scopePlatformWant, "")
}

func TestValidateAndReservePlatformMinSpendBoundary(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopePlatformVoucher(t, svc, "PLAT", func(p *CreateVoucherParams) { p.MinSpend = 500000 })
	ctx := context.Background()
	ok, err := svc.ValidateAndReserve(ctx, "r1", "PLAT", "b", 500000, "")
	if err != nil || !ok.Valid {
		t.Fatalf("at min spend: %+v err=%v", ok, err)
	}
	assertScope(t, ok, scopePlatformWant, "")
	low, err := svc.ValidateAndReserve(ctx, "r2", "PLAT", "b", 499999, "")
	if err != nil {
		t.Fatal(err)
	}
	if low.Valid || low.Reason != "order subtotal below minimum spend" {
		t.Fatalf("below min spend: %+v", low)
	}
	assertScope(t, low, scopePlatformWant, "")
}

func TestValidateAndReserveWrongSellerReportsOwnerScope(t *testing.T) {
	svc, rrepo := newScopeSvc(t, nil)
	scopeShopVoucher(t, svc, "SHOP", "S1", nil)
	res, err := svc.ValidateAndReserve(context.Background(), "r1", "SHOP", "b", 5000, "S2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid || res.Reason != "voucher not valid for this seller" {
		t.Fatalf("res=%+v", res)
	}
	assertScope(t, res, scopeShopWant, "S1")
	assertNoHold(t, rrepo, "r1")
}

func TestValidateAndReserveShopVoucherRequiresSeller(t *testing.T) {
	svc, rrepo := newScopeSvc(t, nil)
	scopeShopVoucher(t, svc, "SHOP", "S1", nil)
	res, err := svc.ValidateAndReserve(context.Background(), "r1", "SHOP", "b", 5000, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid || res.Reason != "voucher not valid for this seller" {
		t.Fatalf("res=%+v", res)
	}
	if res.Scope != scopeShopWant {
		t.Fatalf("scope=%v, want shop", res.Scope)
	}
	assertNoHold(t, rrepo, "r1")
}

func TestValidateAndReserveRejectionsReportScope(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name       string
		create     func(*testing.T, *VoucherService)
		code       string
		sellerID   string
		wantReason string
		wantScope  int32
		wantSeller string
	}{
		{"expired platform", func(t *testing.T, s *VoucherService) {
			scopePlatformVoucher(t, s, "OLD", func(p *CreateVoucherParams) { p.EndsAt = now.Add(-time.Hour) })
		}, "OLD", "", "voucher expired", scopePlatformWant, ""},
		{"not yet active platform", func(t *testing.T, s *VoucherService) {
			scopePlatformVoucher(t, s, "FUT", func(p *CreateVoucherParams) { p.StartsAt = now.Add(time.Hour) })
		}, "FUT", "", "voucher not yet active", scopePlatformWant, ""},
		{"below min spend shop", func(t *testing.T, s *VoucherService) {
			scopeShopVoucher(t, s, "MIN", "S1", func(p *CreateVoucherParams) { p.MinSpend = 999999 })
		}, "MIN", "S1", "order subtotal below minimum spend", scopeShopWant, "S1"},
		{"quota exhausted shop", func(t *testing.T, s *VoucherService) {
			v := scopeShopVoucher(t, s, "FULL", "S1", func(p *CreateVoucherParams) { p.Quota = 1 })
			if err := s.vouchers.IncrementUsed(context.Background(), v.ID, 1); err != nil {
				t.Fatal(err)
			}
		}, "FULL", "S1", "voucher quota exhausted", scopeShopWant, "S1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newScopeSvc(t, nil)
			tc.create(t, svc)
			res, err := svc.ValidateAndReserve(context.Background(), "r1", tc.code, "b", 5000, tc.sellerID)
			if err != nil {
				t.Fatal(err)
			}
			if res.Valid || res.Reason != tc.wantReason {
				t.Fatalf("res=%+v, want reason %q", res, tc.wantReason)
			}
			assertScope(t, res, tc.wantScope, tc.wantSeller)
		})
	}
}

func TestValidateAndReserveUnknownCodeNoScope(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	for code, reason := range map[string]string{"NOPE": "voucher code not found", "": "voucher code is required"} {
		res, err := svc.ValidateAndReserve(context.Background(), "r-"+code, code, "b", 5000, "S1")
		if err != nil {
			t.Fatal(err)
		}
		if res.Valid || res.Reason != reason {
			t.Fatalf("code %q: %+v", code, res)
		}
		assertScope(t, res, scopeUnspecifiedWant, "")
	}
}

func TestValidateAndReserveReplayReportsStoredScope(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	v := scopeShopVoucher(t, svc, "SHOP", "S1", nil)
	scopePlatformVoucher(t, svc, "OTHER", nil)
	ctx := context.Background()
	first, err := svc.ValidateAndReserve(ctx, "r1", "SHOP", "b", 5000, "S1")
	if err != nil || !first.Valid {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	again, err := svc.ValidateAndReserve(ctx, "r1", "OTHER", "b", 1, "S9")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Valid || again.DiscountAmount != first.DiscountAmount || again.VoucherID != v.ID {
		t.Fatalf("replay changed outcome: first=%+v again=%+v", first, again)
	}
	assertScope(t, again, scopeShopWant, "S1")
}

func TestValidateAndReserveReleasedReplayReportsScope(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopePlatformVoucher(t, svc, "PLAT", nil)
	ctx := context.Background()
	if _, err := svc.ValidateAndReserve(ctx, "r1", "PLAT", "b", 5000, ""); err != nil {
		t.Fatal(err)
	}
	if ok, err := svc.ReleaseReservation(ctx, "r1"); err != nil || !ok {
		t.Fatalf("release ok=%v err=%v", ok, err)
	}
	res, err := svc.ValidateAndReserve(ctx, "r1", "PLAT", "b", 5000, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid || res.Reason != "reservation already released" {
		t.Fatalf("res=%+v", res)
	}
	assertScope(t, res, scopePlatformWant, "")
}

func TestValidateAndReserveReplayMissingVoucherNoScope(t *testing.T) {
	svc, rrepo := newScopeSvc(t, nil)
	_, _, err := rrepo.Create(context.Background(), repository.Reservation{
		ReservationID: "r1", VoucherID: "ghost", BuyerID: "b", DiscountAmount: 777, Status: repository.ReservationReserved,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ValidateAndReserve(context.Background(), "r1", "ANY", "b", 5000, "S1")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !res.Valid || res.DiscountAmount != 777 || res.VoucherID != "ghost" {
		t.Fatalf("stored outcome lost: %+v", res)
	}
	assertScope(t, res, scopeUnspecifiedWant, "")
}

func TestValidateAndReserveReplayStoreErrorPropagates(t *testing.T) {
	boom := errors.New("db down")
	svc, rrepo := newScopeSvc(t, boom)
	_, _, err := rrepo.Create(context.Background(), repository.Reservation{
		ReservationID: "r1", VoucherID: "v1", BuyerID: "b", DiscountAmount: 100, Status: repository.ReservationReserved,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateAndReserve(context.Background(), "r1", "ANY", "b", 5000, ""); err == nil {
		t.Fatal("want store error, got nil")
	}
}
