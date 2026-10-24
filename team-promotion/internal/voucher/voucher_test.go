package voucher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

type eventLog struct {
	mu       sync.Mutex
	vouchers []store.Voucher
	rows     []outbox.Row
	reqIDs   []string
	err      error
}

func (l *eventLog) build(ctx context.Context, v store.Voucher) (outbox.Row, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return outbox.Row{}, l.err
	}
	row := outbox.Row{
		EventID:       "evt-" + v.ID,
		AggregateType: "voucher",
		AggregateID:   v.ID,
		EventType:     "platform.promotion.v1.VoucherChanged",
		Payload:       []byte(v.Code),
	}
	l.vouchers = append(l.vouchers, v)
	l.rows = append(l.rows, row)
	l.reqIDs = append(l.reqIDs, reqctx.RequestID(ctx))
	return row, nil
}

func (l *eventLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.rows)
}

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

type recordingAuditWriter struct {
	log    *callLog
	err    error
	mu     sync.Mutex
	events []auditx.Event
}

func (w *recordingAuditWriter) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	if w.log != nil {
		w.log.add("audit")
	}
	w.mu.Lock()
	w.events = append(w.events, ev)
	w.mu.Unlock()
	return w.err
}

func (w *recordingAuditWriter) got() []auditx.Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]auditx.Event(nil), w.events...)
}

var auditFailures = map[string]error{
	"upstream error": status.Error(codes.Internal, "audit db down"),
	"timeout":        context.DeadlineExceeded,
	"unavailable":    status.Error(codes.Unavailable, "team-audit down"),
	"plain error":    errors.New("dial tcp: refused"),
}

func assertAuditUnavailable(t *testing.T, err error) {
	t.Helper()
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != auditx.Message {
		t.Fatalf("err = %v, want Unavailable %q", err, auditx.Message)
	}
}

func newVoucherSvc(t *testing.T) (*Service, *storefake.InMemoryVoucherRepository, *eventLog) {
	t.Helper()
	vrepo := storefake.NewInMemoryVoucherRepository()
	rrepo := storefake.NewInMemoryReservationRepository()
	events := &eventLog{}
	svc := New(vrepo, rrepo, auditx.NewGate(&recordingAuditWriter{}, nil), events.build, nil)
	return svc, vrepo, events
}

func mustCreatePercentVoucher(t *testing.T, svc *Service, p CreateVoucherParams) store.Voucher {
	t.Helper()
	v, err := svc.CreateVoucher(t.Context(), p)
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	return v
}

func TestCreateVoucherEmitsEvent(t *testing.T) {
	svc, repo, events := newVoucherSvc(t)
	ctx := reqctx.WithRequestID(t.Context(), "req-42")
	v, err := svc.CreateVoucher(ctx, CreateVoucherParams{
		Code:          "SAVE10",
		DiscountType:  discountTypePercent,
		DiscountValue: 10,
	})
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	if v.ID == "" {
		t.Fatal("expected generated voucher id")
	}
	if events.count() != 1 {
		t.Fatalf("expected 1 outbox row built, got %d", events.count())
	}
	if got := events.vouchers[0]; got.ID != v.ID || got.Code != "SAVE10" {
		t.Fatalf("builder received %+v, want the stored voucher %+v", got, v)
	}
	if got := events.rows[0]; got.AggregateID != v.ID || got.AggregateType != "voucher" {
		t.Fatalf("row = %+v, want aggregate voucher/%s", got, v.ID)
	}
	if events.reqIDs[0] != "req-42" {
		t.Fatalf("builder context request id = %q, want req-42", events.reqIDs[0])
	}
	if _, err := repo.GetByCode(t.Context(), "SAVE10"); err != nil {
		t.Fatalf("voucher not stored: %v", err)
	}
}

func TestCreateVoucherEventBuilderErrorStoresNothing(t *testing.T) {
	svc, repo, events := newVoucherSvc(t)
	events.err = errors.New("marshal failed")
	_, err := svc.CreateVoucher(t.Context(), CreateVoucherParams{Code: "NOEVT", DiscountType: discountTypePercent, DiscountValue: 10})
	if err == nil {
		t.Fatal("want the builder error, got nil")
	}
	assertVoucherAbsent(t, repo, "NOEVT")
}

func TestValidateAndReserveIdempotent(t *testing.T) {
	svc, _, _ := newVoucherSvc(t)
	ctx := t.Context()
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

	second, err := svc.ValidateAndReserve(ctx, "resv-1", "PCT20", "buyer-1", 100000, "")
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if second.DiscountAmount != first.DiscountAmount || second.VoucherID != first.VoucherID {
		t.Fatalf("idempotency broken: first=%+v second=%+v", first, second)
	}

	v, _ := svc.GetVoucher(ctx, "PCT20")
	if v.Used != 0 {
		t.Fatalf("expected used=0 after reserve, got %d", v.Used)
	}
}

func TestValidateAndReserveRejections(t *testing.T) {
	ctx := t.Context()
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
	ctx := t.Context()
	mustCreatePercentVoucher(t, svc, CreateVoucherParams{
		Code:          "ONE",
		DiscountType:  discountTypeFixed,
		DiscountValue: 1000,
		Quota:         1,
	})

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
	ctx := t.Context()
	mustCreatePercentVoucher(t, svc, CreateVoucherParams{Code: "C", DiscountType: discountTypeFixed, DiscountValue: 100, Quota: 10})

	if _, err := svc.ValidateAndReserve(ctx, "r", "C", "b", 5000, ""); err != nil {
		t.Fatalf("reserve: %v", err)
	}
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
	ctx := t.Context()
	mustCreatePercentVoucher(t, svc, CreateVoucherParams{Code: "R", DiscountType: discountTypeFixed, DiscountValue: 100, Quota: 10})
	if _, err := svc.ValidateAndReserve(ctx, "r", "R", "b", 5000, ""); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	released, err := svc.ReleaseReservation(ctx, "r")
	if err != nil || !released {
		t.Fatalf("release: released=%v err=%v", released, err)
	}
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
		voucher  store.Voucher
		subtotal int64
		want     int64
	}{
		{"percent", store.Voucher{DiscountType: discountTypePercent, DiscountValue: 10}, 100000, 10000},
		{"percent capped", store.Voucher{DiscountType: discountTypePercent, DiscountValue: 50, MaxDiscount: 15000}, 100000, 15000},
		{"fixed", store.Voucher{DiscountType: discountTypeFixed, DiscountValue: 20000}, 100000, 20000},
		{"fixed over subtotal", store.Voucher{DiscountType: discountTypeFixed, DiscountValue: 20000}, 5000, 5000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeDiscount(tc.voucher, tc.subtotal); got != tc.want {
				t.Fatalf("computeDiscount = %d, want %d", got, tc.want)
			}
		})
	}
}

type orderedVoucherRepo struct {
	store.VoucherRepository
	log *callLog
}

func (r orderedVoucherRepo) Create(ctx context.Context, v store.Voucher, event func(store.Voucher) (outbox.Row, error)) (store.Voucher, error) {
	r.log.add("insert")
	return r.VoucherRepository.Create(ctx, v, event)
}

func newAuditedVoucherSvc(t *testing.T, w auditx.Writer, log *callLog) (*Service, *storefake.InMemoryVoucherRepository, *eventLog) {
	t.Helper()
	mem := storefake.NewInMemoryVoucherRepository()
	events := &eventLog{}
	svc := New(orderedVoucherRepo{mem, log}, storefake.NewInMemoryReservationRepository(), auditx.NewGate(w, nil), events.build, nil)
	return svc, mem, events
}

func platformParams(code string) CreateVoucherParams {
	return CreateVoucherParams{
		Code: code, Scope: 2, DiscountType: discountTypePercent, DiscountValue: 10, Quota: 5,
		AuditActorID: "admin-1",
	}
}

func assertVoucherAbsent(t *testing.T, mem *storefake.InMemoryVoucherRepository, code string) {
	t.Helper()
	if _, err := mem.GetByCode(t.Context(), code); !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("voucher %q must not exist, GetByCode err = %v", code, err)
	}
}

func TestCreateVoucherAuditBeforeInsert(t *testing.T) {
	for name, scope := range map[string]int32{"platform": 2, "unspecified": 0} {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			w := &recordingAuditWriter{log: log}
			svc, mem, events := newAuditedVoucherSvc(t, w, log)
			p := platformParams("PLAT10")
			p.Scope = scope
			if _, err := svc.CreateVoucher(t.Context(), p); err != nil {
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
			if _, err := mem.GetByCode(t.Context(), "PLAT10"); err != nil {
				t.Errorf("voucher not stored: %v", err)
			}
			if events.count() != 1 {
				t.Errorf("VoucherChanged emitted %d times, want 1", events.count())
			}
		})
	}
}

func TestCreateVoucherAuditTargetIsTrimmedCodeAndFixedType(t *testing.T) {
	w := &recordingAuditWriter{}
	svc, _, _ := newAuditedVoucherSvc(t, w, &callLog{})
	p := platformParams("  FIX5  ")
	p.DiscountType, p.DiscountValue = discountTypeFixed, 500
	if _, err := svc.CreateVoucher(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	evs := w.got()
	if len(evs) != 1 || evs[0].TargetID != "FIX5" || evs[0].Metadata["discount_type"] != "DISCOUNT_TYPE_FIXED" || evs[0].Metadata["value"] != "500" {
		t.Fatalf("events = %+v", evs)
	}
}

func TestCreateVoucherAuditDownUnavailable(t *testing.T) {
	cases := map[string]CreateVoucherParams{
		"platform":    platformParams("DOWN-PLAT"),
		"unspecified": func() CreateVoucherParams { p := platformParams("DOWN-UNSPEC"); p.Scope = 0; return p }(),
		"shop admin":  shopAdminParams("DOWN-SHOP"),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			w := &recordingAuditWriter{log: log, err: errors.New("audit down")}
			svc, mem, events := newAuditedVoucherSvc(t, w, log)
			_, err := svc.CreateVoucher(t.Context(), p)
			assertAuditUnavailable(t, err)
			if got := log.get(); len(got) != 1 || got[0] != "audit" {
				t.Fatalf("call order = %v, want [audit] (no insert)", got)
			}
			assertVoucherAbsent(t, mem, strings.TrimSpace(p.Code))
			if events.count() != 0 {
				t.Errorf("nothing may be emitted, got %d", events.count())
			}
		})
	}
}

func TestCreateVoucherAuditErrorFailsClosed(t *testing.T) {
	for name, cause := range auditFailures {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			svc, mem, events := newAuditedVoucherSvc(t, &recordingAuditWriter{log: log, err: cause}, log)
			_, err := svc.CreateVoucher(t.Context(), platformParams("PLAT-DOWN"))
			assertAuditUnavailable(t, err)
			if got := log.get(); len(got) != 1 || got[0] != "audit" {
				t.Fatalf("call order = %v, want [audit] (no insert)", got)
			}
			assertVoucherAbsent(t, mem, "PLAT-DOWN")
			if events.count() != 0 {
				t.Errorf("nothing may be emitted, got %d", events.count())
			}
		})
	}
}

func TestCreateVoucherShopScopeNotAudited(t *testing.T) {
	for name, w := range map[string]*recordingAuditWriter{"writer wired": {}, "writer failing": {err: errors.New("down")}} {
		t.Run(name, func(t *testing.T) {
			svc, mem, _ := newAuditedVoucherSvc(t, w, &callLog{})
			p := platformParams("SHOP1")
			p.Scope, p.SellerID = voucherScopeShop, "seller-1"
			if _, err := svc.CreateVoucher(t.Context(), p); err != nil {
				t.Fatalf("shop voucher: %v", err)
			}
			if n := len(w.got()); n != 0 {
				t.Fatalf("shop voucher wrote %d audit events, want 0", n)
			}
			if _, err := mem.GetByCode(t.Context(), "SHOP1"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreateVoucherInvalidCodeNotAudited(t *testing.T) {
	for _, code := range []string{"", "   "} {
		w := &recordingAuditWriter{}
		svc, _, _ := newAuditedVoucherSvc(t, w, &callLog{})
		if _, err := svc.CreateVoucher(t.Context(), platformParams(code)); !errors.Is(err, ErrInvalidVoucher) {
			t.Fatalf("code %q: err = %v, want ErrInvalidVoucher", code, err)
		}
		if n := len(w.got()); n != 0 {
			t.Fatalf("code %q: invalid voucher wrote %d events", code, n)
		}
	}
}

func TestCreateVoucherInsertFailureAfterAuditIsNotUnavailable(t *testing.T) {
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	svc, _, _ := newAuditedVoucherSvc(t, w, log)
	if _, err := svc.CreateVoucher(t.Context(), platformParams("DUP")); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateVoucher(t.Context(), platformParams("DUP"))
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

func TestCreateVoucherShopAdminAuditBeforeInsert(t *testing.T) {
	log := &callLog{}
	w := &recordingAuditWriter{log: log}
	svc, mem, events := newAuditedVoucherSvc(t, w, log)
	if _, err := svc.CreateVoucher(t.Context(), shopAdminParams("SHOP-ADM10")); err != nil {
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
	if _, err := mem.GetByCode(t.Context(), "SHOP-ADM10"); err != nil {
		t.Errorf("voucher not stored: %v", err)
	}
	if events.count() != 1 {
		t.Errorf("VoucherChanged emitted %d times, want 1", events.count())
	}
}

func TestCreateVoucherShopAdminAuditErrorFailsClosed(t *testing.T) {
	for name, cause := range auditFailures {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			svc, mem, events := newAuditedVoucherSvc(t, &recordingAuditWriter{log: log, err: cause}, log)
			_, err := svc.CreateVoucher(t.Context(), shopAdminParams("SHOP-DOWN"))
			assertAuditUnavailable(t, err)
			if got := log.get(); len(got) != 1 || got[0] != "audit" {
				t.Fatalf("call order = %v, want [audit] (no insert)", got)
			}
			assertVoucherAbsent(t, mem, "SHOP-DOWN")
			if events.count() != 0 {
				t.Errorf("nothing may be emitted, got %d", events.count())
			}
		})
	}
}

func TestCreateVoucherPlatformStillPlatformAction(t *testing.T) {
	w := &recordingAuditWriter{}
	svc, _, _ := newAuditedVoucherSvc(t, w, &callLog{})
	p := platformParams("PLAT-KEEP")
	p.AuditAdmin = true
	if _, err := svc.CreateVoucher(t.Context(), p); err != nil {
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
	store.VoucherRepository
	getByIDErr error
}

func (r scopeVoucherRepo) GetByID(ctx context.Context, id string) (store.Voucher, error) {
	if r.getByIDErr != nil {
		return store.Voucher{}, r.getByIDErr
	}
	return r.VoucherRepository.GetByID(ctx, id)
}

func newScopeSvc(t *testing.T, getByIDErr error) (*Service, *storefake.InMemoryReservationRepository) {
	t.Helper()
	vrepo := scopeVoucherRepo{VoucherRepository: storefake.NewInMemoryVoucherRepository(), getByIDErr: getByIDErr}
	rrepo := storefake.NewInMemoryReservationRepository()
	svc := New(vrepo, rrepo, auditx.NewGate(&recordingAuditWriter{}, nil), (&eventLog{}).build, nil)
	return svc, rrepo
}

func scopeShopVoucher(t *testing.T, svc *Service, code, seller string, mod func(*CreateVoucherParams)) store.Voucher {
	t.Helper()
	p := CreateVoucherParams{Code: code, Scope: scopeShopWant, SellerID: seller, DiscountType: discountTypeFixed, DiscountValue: 1000, Quota: 5}
	if mod != nil {
		mod(&p)
	}
	return mustCreatePercentVoucher(t, svc, p)
}

func scopePlatformVoucher(t *testing.T, svc *Service, code string, mod func(*CreateVoucherParams)) store.Voucher {
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

func assertNoHold(t *testing.T, rrepo *storefake.InMemoryReservationRepository, id string) {
	t.Helper()
	if _, err := rrepo.GetByReservationID(t.Context(), id); err == nil {
		t.Fatalf("hold %q exists, want none", id)
	}
}

func TestValidateAndReserveReportsShopScope(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopeShopVoucher(t, svc, "SHOP", "S1", func(p *CreateVoucherParams) { p.MinSpend = 1000 })
	res, err := svc.ValidateAndReserve(t.Context(), "r1", "SHOP", "b", 5000, "S1")
	if err != nil || !res.Valid {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	assertScope(t, res, scopeShopWant, "S1")
}

func TestValidateAndReservePlatformScopeNoSeller(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopePlatformVoucher(t, svc, "PLAT", nil)
	res, err := svc.ValidateAndReserve(t.Context(), "r1", "PLAT", "b", 5000, "")
	if err != nil || !res.Valid {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	assertScope(t, res, scopePlatformWant, "")
}

func TestValidateAndReservePlatformIgnoresRequestSeller(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopePlatformVoucher(t, svc, "PLAT", nil)
	res, err := svc.ValidateAndReserve(t.Context(), "r1", "PLAT", "b", 5000, "S9")
	if err != nil || !res.Valid {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	assertScope(t, res, scopePlatformWant, "")
}

func TestValidateAndReservePlatformMinSpendBoundary(t *testing.T) {
	svc, _ := newScopeSvc(t, nil)
	scopePlatformVoucher(t, svc, "PLAT", func(p *CreateVoucherParams) { p.MinSpend = 500000 })
	ctx := t.Context()
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
	res, err := svc.ValidateAndReserve(t.Context(), "r1", "SHOP", "b", 5000, "S2")
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
	res, err := svc.ValidateAndReserve(t.Context(), "r1", "SHOP", "b", 5000, "")
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
		create     func(*testing.T, *Service)
		code       string
		sellerID   string
		wantReason string
		wantScope  int32
		wantSeller string
	}{
		{"expired platform", func(t *testing.T, s *Service) {
			scopePlatformVoucher(t, s, "OLD", func(p *CreateVoucherParams) { p.EndsAt = now.Add(-time.Hour) })
		}, "OLD", "", "voucher expired", scopePlatformWant, ""},
		{"not yet active platform", func(t *testing.T, s *Service) {
			scopePlatformVoucher(t, s, "FUT", func(p *CreateVoucherParams) { p.StartsAt = now.Add(time.Hour) })
		}, "FUT", "", "voucher not yet active", scopePlatformWant, ""},
		{"below min spend shop", func(t *testing.T, s *Service) {
			scopeShopVoucher(t, s, "MIN", "S1", func(p *CreateVoucherParams) { p.MinSpend = 999999 })
		}, "MIN", "S1", "order subtotal below minimum spend", scopeShopWant, "S1"},
		{"quota exhausted shop", func(t *testing.T, s *Service) {
			v := scopeShopVoucher(t, s, "FULL", "S1", func(p *CreateVoucherParams) { p.Quota = 1 })
			if err := s.vouchers.IncrementUsed(t.Context(), v.ID, 1); err != nil {
				t.Fatal(err)
			}
		}, "FULL", "S1", "voucher quota exhausted", scopeShopWant, "S1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newScopeSvc(t, nil)
			tc.create(t, svc)
			res, err := svc.ValidateAndReserve(t.Context(), "r1", tc.code, "b", 5000, tc.sellerID)
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
		res, err := svc.ValidateAndReserve(t.Context(), "r-"+code, code, "b", 5000, "S1")
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
	ctx := t.Context()
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
	ctx := t.Context()
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
	_, _, err := rrepo.Create(t.Context(), store.Reservation{
		ReservationID: "r1", VoucherID: "ghost", BuyerID: "b", DiscountAmount: 777, Status: store.ReservationReserved,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ValidateAndReserve(t.Context(), "r1", "ANY", "b", 5000, "S1")
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
	_, _, err := rrepo.Create(t.Context(), store.Reservation{
		ReservationID: "r1", VoucherID: "v1", BuyerID: "b", DiscountAmount: 100, Status: store.ReservationReserved,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateAndReserve(t.Context(), "r1", "ANY", "b", 5000, ""); err == nil {
		t.Fatal("want store error, got nil")
	}
}

func seedListedVouchers(t *testing.T, seller string, n int) *Service {
	t.Helper()
	repo := storefake.NewInMemoryVoucherRepository()
	noEvent := func(store.Voucher) (outbox.Row, error) { return outbox.Row{}, nil }
	for i := 0; i < n; i++ {
		_, err := repo.Create(t.Context(), store.Voucher{
			Code: fmt.Sprintf("V%03d", i), Scope: voucherScopeShop, SellerID: seller,
			DiscountType: discountTypeFixed, DiscountValue: 100, Quota: 5,
		}, noEvent)
		if err != nil {
			t.Fatalf("seed voucher %d: %v", i, err)
		}
	}
	return New(repo, storefake.NewInMemoryReservationRepository(), auditx.NewGate(&recordingAuditWriter{}, nil), (&eventLog{}).build, nil)
}

func codesOf(vs []store.Voucher) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Code)
	}
	return out
}

func TestListVouchersCursorIsPlainOffset(t *testing.T) {
	svc := seedListedVouchers(t, "seller-1", 120)

	first, cursor, err := svc.ListVouchers(t.Context(), "seller-1", "", 50)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(first) != 50 || cursor != "50" {
		t.Fatalf("page 1 = %d items cursor %q, want 50 items cursor \"50\"", len(first), cursor)
	}
	if first[0].Code != "V000" || first[49].Code != "V049" {
		t.Fatalf("page 1 spans %s..%s, want V000..V049", first[0].Code, first[49].Code)
	}

	second, cursor, err := svc.ListVouchers(t.Context(), "seller-1", cursor, 50)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(second) != 50 || cursor != "100" {
		t.Fatalf("page 2 = %d items cursor %q, want 50 items cursor \"100\"", len(second), cursor)
	}
	if second[0].Code != "V050" || second[49].Code != "V099" {
		t.Fatalf("page 2 spans %s..%s, want V050..V099", second[0].Code, second[49].Code)
	}

	third, cursor, err := svc.ListVouchers(t.Context(), "seller-1", cursor, 50)
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(third) != 20 || cursor != "" {
		t.Fatalf("page 3 = %d items cursor %q, want 20 items and an empty cursor", len(third), cursor)
	}
	if third[0].Code != "V100" || third[19].Code != "V119" {
		t.Fatalf("page 3 spans %s..%s, want V100..V119", third[0].Code, third[19].Code)
	}
}

func TestListVouchersCursorIsPlainOffsetSmallPage(t *testing.T) {
	svc := seedListedVouchers(t, "seller-1", 20)
	items, cursor, err := svc.ListVouchers(t.Context(), "seller-1", "", 7)
	if err != nil || len(items) != 7 || cursor != "7" {
		t.Fatalf("items=%d cursor=%q err=%v, want 7 items cursor \"7\"", len(items), cursor, err)
	}
	items, cursor, err = svc.ListVouchers(t.Context(), "seller-1", "14", 7)
	if err != nil || len(items) != 6 || cursor != "" {
		t.Fatalf("last page items=%d cursor=%q err=%v, want 6 items and an empty cursor", len(items), cursor, err)
	}
}

func TestListVouchersOversizedPageUsesDefault(t *testing.T) {
	svc := seedListedVouchers(t, "seller-1", 120)
	for _, size := range []int32{201, 100000, 0, -1} {
		items, cursor, err := svc.ListVouchers(t.Context(), "seller-1", "", size)
		if err != nil {
			t.Fatalf("page_size %d: %v", size, err)
		}
		if len(items) != 50 || cursor != "50" {
			t.Fatalf("page_size %d: %d items cursor %q, want the default page of 50 with cursor \"50\"", size, len(items), cursor)
		}
	}
}

func TestListVouchersMalformedCursorIsOffsetZero(t *testing.T) {
	svc := seedListedVouchers(t, "seller-1", 120)
	want, _, err := svc.ListVouchers(t.Context(), "seller-1", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, cursor := range []string{"not-a-number", "-5", "1.5", "50abc"} {
		got, next, err := svc.ListVouchers(t.Context(), "seller-1", cursor, 50)
		if err != nil {
			t.Fatalf("cursor %q: %v", cursor, err)
		}
		if next != "50" || strings.Join(codesOf(got), ",") != strings.Join(codesOf(want), ",") {
			t.Fatalf("cursor %q: page starts %s next %q, want the first page (V000.. next \"50\")", cursor, got[0].Code, next)
		}
	}
}
