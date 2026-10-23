package rpc_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/store"
)

// ---- fakes -------------------------------------------------------------------------

// callLog is the shared, ordered record of audit writes and ledger applications.
type callLog struct {
	mu     sync.Mutex
	events []string
}

func (l *callLog) add(s string) { l.mu.Lock(); l.events = append(l.events, s); l.mu.Unlock() }
func (l *callLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

type auditCall struct {
	Actor, Payment, Order, Refund string
	Amount                        int64
}

// fakeAudit is the auditx.Writer at the true external boundary (team-audit).
type fakeAudit struct {
	log    *callLog
	mu     sync.Mutex
	events []auditx.Event
	err    error
	block  bool
}

func (f *fakeAudit) WriteAuditEvent(ctx context.Context, ev auditx.Event) error {
	f.log.add("audit")
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.mu.Unlock()
	return nil
}

func (f *fakeAudit) all() []auditx.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]auditx.Event(nil), f.events...)
}

func (f *fakeAudit) written() []auditCall {
	var out []auditCall
	for _, ev := range f.all() {
		amount, _ := strconv.ParseInt(ev.Metadata["amount"], 10, 64)
		out = append(out, auditCall{ev.ActorID, ev.TargetID, ev.Metadata["order_id"], ev.Metadata["refund_id"], amount})
	}
	return out
}

// flakyLedger fails ApplyRefund with a plain error while failN > 0 (-> Internal).
type flakyLedger struct {
	store.SettlementLedger
	log   *callLog
	failN int
}

func (l *flakyLedger) ApplyRefund(ctx context.Context, req store.RefundRequest) (store.RefundResult, error) {
	l.log.add("refund")
	if l.failN > 0 {
		l.failN--
		return store.RefundResult{}, errors.New("ledger update failed")
	}
	return l.SettlementLedger.ApplyRefund(ctx, req)
}

type auditFixture struct {
	ledgerFixture
	log    *callLog
	audit  *fakeAudit
	flaky  *flakyLedger
	txID   string
	orderI string
}

// newAuditFixture builds the ledger fixture (order-L, seller-L, buyer-L, 500000) with a
// failure-injectable ledger and the fake audit writer behind the SDK gate.
func newAuditFixture(t *testing.T, writer func(*fakeAudit)) auditFixture {
	t.Helper()
	log := &callLog{}
	st := newStores()
	flaky := &flakyLedger{SettlementLedger: st.settlement, log: log}
	st.settlement = flaky
	fa := &fakeAudit{log: log}
	if writer != nil {
		writer(fa)
	}
	f := newLedgerFixtureWith(t, st, 0)
	f.h = newHandler(t, f.svc, fa)
	tx := f.payAndCredit(t)
	log.events = nil // payment setup is not part of the refund call order
	return auditFixture{ledgerFixture: f, log: log, audit: fa, flaky: flaky, txID: tx.GetId(), orderI: "order-L"}
}

func withErr(err error) func(*fakeAudit) { return func(f *fakeAudit) { f.err = err } }

func admin(id string, extra ...string) context.Context {
	return principalCtx(id, auth.TypeUser, append([]string{"admin"}, extra...)...)
}

func (f auditFixture) read(t *testing.T) *paymentv1.PaymentTransaction {
	t.Helper()
	res, err := f.h.GetPayment(userCtx("buyer-L"), &paymentv1.GetPaymentRequest{Id: f.txID})
	if err != nil {
		t.Fatal(err)
	}
	return res.GetTransaction()
}

func (f auditFixture) deductions(t *testing.T) []store.LedgerEntry {
	t.Helper()
	return entriesOf(t, f.ledgerFixture, "seller-L")[store.LedgerTypeRefundDeduction]
}

func (f auditFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	p := f.read(t)
	if p.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID || p.GetRefundedAmount() != 0 || len(p.GetRefunds()) != 0 {
		t.Fatalf("payment changed: %v", p)
	}
	if d := f.deductions(t); len(d) != 0 {
		t.Fatalf("REFUND_DEDUCTION entries written: %+v", d)
	}
}

// ---- admin path --------------------------------------------------------------------

// Scenario: Admin refund writes one audit event and refunds
func TestAdminRefundWritesAuditEvent(t *testing.T) {
	f := newAuditFixture(t, nil)
	res, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if res.GetTransaction().GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_REFUNDED || res.GetTransaction().GetRefundedAmount() != 200000 {
		t.Fatalf("refund response: %v", res)
	}
	p := f.read(t)
	if p.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_REFUNDED || p.GetRefundedAmount() != 200000 {
		t.Fatalf("payment: %v", p)
	}
	want := []auditCall{{Actor: "adm-1", Payment: f.txID, Order: "order-L", Refund: "R-1", Amount: 200000}}
	if got := f.audit.written(); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit events = %+v, want %+v", got, want)
	}
}

// Scenario: Audit is written before the refund
func TestAdminRefundAuditBeforeRefund(t *testing.T) {
	f := newAuditFixture(t, nil)
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000)); err != nil {
		t.Fatal(err)
	}
	if got, want := f.log.all(), []string{"audit", "refund"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
}

// ---- metadata and bound ------------------------------------------------------------

// Scenario: An admin refund writes one event with the SDK event shape
func TestAdminRefundAuditMetadataKeys(t *testing.T) {
	f := newAuditFixture(t, nil)
	_, err := f.h.RefundPayment(admin("adm-1"), &paymentv1.RefundPaymentRequest{
		PaymentId: f.txID, RefundId: "R-1", Amount: 200000, Reason: "buyer complaint 123"})
	if err != nil {
		t.Fatal(err)
	}
	evs := f.audit.all()
	if len(evs) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(evs))
	}
	ev := evs[0]
	keys := make([]string, 0, 3)
	for k := range ev.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"amount", "order_id", "refund_id"}) {
		t.Fatalf("metadata keys = %v, want exactly amount, order_id, refund_id", keys)
	}
	if ev.Metadata["amount"] != strconv.Itoa(200000) || ev.Metadata["order_id"] != "order-L" || ev.Metadata["refund_id"] != "R-1" ||
		ev.Action != "payment.admin_refund" || ev.TargetType != "payment" ||
		ev.TargetID != f.txID || ev.ActorID != "adm-1" {
		t.Fatalf("event = %+v", ev)
	}
	for _, v := range ev.Metadata {
		if strings.Contains(v, "complaint") {
			t.Fatalf("the reason leaked into metadata: %+v", ev.Metadata)
		}
	}
}

// Scenario: A slow audit service is cut off
func TestAdminRefundAuditTimeoutFailsClosed(t *testing.T) {
	f := newAuditFixture(t, func(a *fakeAudit) { a.block = true })
	start := time.Now()
	_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != "audit trail unavailable" {
		t.Fatalf("want Unavailable \"audit trail unavailable\", got %v", err)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("not bounded: %v", el)
	}
	if got := f.log.all(); !reflect.DeepEqual(got, []string{"audit"}) {
		t.Fatalf("calls = %v; the refund must not run after an audit timeout", got)
	}
	f.assertUnchanged(t)
}

// ---- refusals before the audit write -----------------------------------------------

// Scenario: Refusal before the audit write writes no event
func TestAdminRefundRefusedBeforeAuditWritesNothing(t *testing.T) {
	f := newAuditFixture(t, nil)
	cases := []struct {
		name string
		req  *paymentv1.RefundPaymentRequest
		code codes.Code
	}{
		{"empty payment id", refundReq("", "R-1", 100), codes.InvalidArgument},
		{"empty refund id", refundReq(f.txID, "", 100), codes.InvalidArgument},
		{"refund id with space", refundReq(f.txID, "has space", 100), codes.InvalidArgument},
		{"zero amount", refundReq(f.txID, "R-1", 0), codes.InvalidArgument},
		{"negative amount", refundReq(f.txID, "R-1", -5), codes.InvalidArgument},
		{"unknown payment", refundReq("no-such-payment", "R-1", 100), codes.NotFound},
	}
	for _, c := range cases {
		_, err := f.h.RefundPayment(admin("adm-1"), c.req)
		if status.Code(err) != c.code {
			t.Errorf("%s: want %v, got %v", c.name, c.code, err)
		}
	}
	if got := f.audit.written(); len(got) != 0 {
		t.Fatalf("audit events written on refusal: %+v", got)
	}
	if got := f.log.all(); len(got) != 0 {
		t.Fatalf("audit writer was invoked on refusal: %v", got)
	}
	f.assertUnchanged(t)
}

// Scenario: Unauthenticated caller writes no event
func TestRefundPaymentAnonymousNoAudit(t *testing.T) {
	f := newAuditFixture(t, nil)
	_, err := f.h.RefundPayment(context.Background(), refundReq(f.txID, "R-1", 100000))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
	if got := f.log.all(); len(got) != 0 {
		t.Fatalf("calls made for an anonymous caller: %v", got)
	}
	f.assertUnchanged(t)
}

// ---- fail closed -------------------------------------------------------------------

// Scenario: Audit error leaves the ledger unchanged
func TestAdminRefundAuditErrorFailsClosed(t *testing.T) {
	f := newAuditFixture(t, withErr(status.Error(codes.Internal, "audit db down")))
	_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != "audit trail unavailable" {
		t.Fatalf("want Unavailable \"audit trail unavailable\", got %v", err)
	}
	if got := f.log.all(); !reflect.DeepEqual(got, []string{"audit"}) {
		t.Fatalf("calls = %v; the refund must not run after an audit failure", got)
	}
	f.assertUnchanged(t)
}

// ---- idempotency -------------------------------------------------------------------

// Scenario: Retry after audit success and refund failure refunds once
func TestAdminRefundRetryAfterFailureRefundsOnce(t *testing.T) {
	f := newAuditFixture(t, nil)
	f.flaky.failN = 1
	_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if status.Code(err) != codes.Internal {
		t.Fatalf("first call: want Internal, got %v", err)
	}
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000)); err != nil {
		t.Fatalf("retry: %v", err)
	}
	p := f.read(t)
	if p.GetRefundedAmount() != 200000 || len(p.GetRefunds()) != 1 {
		t.Fatalf("payment: %v", p)
	}
	if d := f.deductions(t); len(d) != 1 || d[0].Amount != -200000 {
		t.Fatalf("deductions: %+v", d)
	}
	if got := len(f.audit.written()); got != 2 {
		t.Fatalf("audit events = %d, want 2 (at-least-once)", got)
	}
}

// Scenario: Replay of an applied refund id does not move money again
func TestAdminRefundReplayIsIdempotent(t *testing.T) {
	f := newAuditFixture(t, nil)
	first, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first.GetTransaction().GetRefunds()[0].GetId() != second.GetTransaction().GetRefunds()[0].GetId() {
		t.Fatalf("replay returned a different refund: %v vs %v", first, second)
	}
	p := f.read(t)
	if p.GetRefundedAmount() != 200000 || len(p.GetRefunds()) != 1 {
		t.Fatalf("payment: %v", p)
	}
	if d := f.deductions(t); len(d) != 1 || d[0].Amount != -200000 {
		t.Fatalf("deductions: %+v", d)
	}
	if got := len(f.audit.written()); got != 2 {
		t.Fatalf("audit events = %d, want 2 (replay is audited again)", got)
	}
}

// Scenario: Refund above the remainder is refused on the admin path
func TestAdminRefundExceedsRemainderRefused(t *testing.T) {
	f := newAuditFixture(t, nil)
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000)); err != nil {
		t.Fatal(err)
	}
	_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-2", 300001))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	if p := f.read(t); p.GetRefundedAmount() != 200000 || len(p.GetRefunds()) != 1 {
		t.Fatalf("payment changed: %v", p)
	}
}

// Scenario: Admin who is the order's seller is audited
func TestAdminWhoIsSellerRefundIsAudited(t *testing.T) {
	f := newAuditFixture(t, nil)
	if _, err := f.h.RefundPayment(admin("seller-L", "listing.write"), refundReq(f.txID, "R-1", 200000)); err != nil {
		t.Fatal(err)
	}
	got := f.audit.written()
	if len(got) != 1 || got[0].Actor != "seller-L" {
		t.Fatalf("audit events = %+v, want one with actor seller-L", got)
	}
}

// ---- seller / non-admin ------------------------------------------------------------

// Scenario: Seller refund writes no audit event
func TestSellerRefundWritesNoAudit(t *testing.T) {
	f := newAuditFixture(t, nil)
	if _, err := f.h.RefundPayment(userCtx("seller-L"), refundReq(f.txID, "R-1", 200000)); err != nil {
		t.Fatal(err)
	}
	if p := f.read(t); p.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_REFUNDED {
		t.Fatalf("payment: %v", p)
	}
	if got := f.log.all(); !reflect.DeepEqual(got, []string{"refund"}) {
		t.Fatalf("calls = %v; a seller refund must make no audit call", got)
	}
}

// Scenario: Seller refund succeeds while audit is down
func TestSellerRefundIgnoresAuditOutage(t *testing.T) {
	f := newAuditFixture(t, withErr(status.Error(codes.Unavailable, "audit down")))
	if _, err := f.h.RefundPayment(userCtx("seller-L"), refundReq(f.txID, "R-1", 200000)); err != nil {
		t.Fatal(err)
	}
	if p := f.read(t); p.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_REFUNDED || p.GetRefundedAmount() != 200000 {
		t.Fatalf("payment %v", p)
	}
	if got := f.log.all(); !reflect.DeepEqual(got, []string{"refund"}) {
		t.Fatalf("calls = %v; a seller refund must make no audit call", got)
	}
}

// Scenario: Non-seller non-admin is denied without an event
func TestRefundPaymentNonSellerDeniedNoAudit(t *testing.T) {
	f := newAuditFixture(t, nil)
	for _, ctx := range []context.Context{userCtx("someone-else"), userCtx("buyer-L")} {
		_, err := f.h.RefundPayment(ctx, refundReq(f.txID, "R-1", 100000))
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %v", err)
		}
	}
	if got := f.log.all(); len(got) != 0 {
		t.Fatalf("calls made for a denied caller: %v", got)
	}
	f.assertUnchanged(t)
}

// Scenario: Refund audit id: Admin refund event carries refund_id
func TestAdminRefundAuditCarriesRefundID(t *testing.T) {
	f := newAuditFixture(t, nil)
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "rf-1", 200000)); err != nil {
		t.Fatal(err)
	}
	evs := f.audit.all()
	if len(evs) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(evs))
	}
	r := evs[0]
	m := r.Metadata
	if len(m) != 3 || m["refund_id"] != "rf-1" || m["order_id"] != "order-L" || m["amount"] != "200000" ||
		r.Action != "payment.admin_refund" || r.TargetType != "payment" ||
		r.TargetID != f.txID || r.ActorID != "adm-1" {
		t.Fatalf("event = %+v", r)
	}
}

// Scenario: Refund audit id: Retry with the same refund_id is distinguishable
func TestAdminRefundRetryKeepsRefundID(t *testing.T) {
	f := newAuditFixture(t, nil)
	f.flaky.failN = 1
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "rf-1", 200000)); status.Code(err) != codes.Internal {
		t.Fatalf("first call: want Internal, got %v", err)
	}
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "rf-1", 200000)); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "rf-2", 100000)); err != nil {
		t.Fatalf("second refund: %v", err)
	}
	var ids []string
	for _, c := range f.audit.written() {
		ids = append(ids, c.Refund)
	}
	if want := []string{"rf-1", "rf-1", "rf-2"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("audited refund ids = %v, want %v", ids, want)
	}
	if p := f.read(t); p.GetRefundedAmount() != 300000 || len(p.GetRefunds()) != 2 {
		t.Fatalf("money moved wrongly: %v", p)
	}
}

// Scenario: Refund audit id: Invalid refund_id writes no event
func TestAdminRefundInvalidRefundIDWritesNoEvent(t *testing.T) {
	f := newAuditFixture(t, nil)
	for _, id := range []string{"", strings.Repeat("x", 65), "has space"} {
		_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, id, 100))
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("refund_id %q: want InvalidArgument, got %v", id, err)
		}
	}
	if got := f.audit.written(); len(got) != 0 {
		t.Fatalf("audit events written: %+v", got)
	}
	if got := f.log.all(); len(got) != 0 {
		t.Fatalf("audit writer invoked: %v", got)
	}
	f.assertUnchanged(t)
}

// Scenario: Refund audit id: Boundary 64-character refund_id is recorded intact
func TestAdminRefundAuditRefundIDMaxLength(t *testing.T) {
	f := newAuditFixture(t, nil)
	id := strings.Repeat("a", 64)
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, id, 200000)); err != nil {
		t.Fatal(err)
	}
	if evs := f.audit.all(); len(evs) != 1 || evs[0].Metadata["refund_id"] != id {
		t.Fatalf("events = %+v, want refund_id intact", evs)
	}
}

// Scenario: Refund audit id: Audit failure still refuses the refund
func TestAdminRefundAuditErrorLeavesLedgerUnchanged(t *testing.T) {
	f := newAuditFixture(t, withErr(status.Error(codes.Internal, "audit db down")))
	_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "rf-1", 200000))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	if got := f.log.all(); !reflect.DeepEqual(got, []string{"audit"}) {
		t.Fatalf("calls = %v", got)
	}
	f.assertUnchanged(t)
}

// Scenario: Refund audit id: Seller refund writes no audit event
func TestSellerRefundWritesNoAuditEvent(t *testing.T) {
	f := newAuditFixture(t, nil)
	if _, err := f.h.RefundPayment(userCtx("seller-L"), refundReq(f.txID, "rf-1", 200000)); err != nil {
		t.Fatal(err)
	}
	if got := f.audit.written(); len(got) != 0 {
		t.Fatalf("seller refund wrote audit events: %+v", got)
	}
	_, err := f.h.RefundPayment(userCtx("someone-else"), refundReq(f.txID, "rf-2", 100000))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	if got := f.audit.written(); len(got) != 0 {
		t.Fatalf("denied caller wrote audit events: %+v", got)
	}
}
