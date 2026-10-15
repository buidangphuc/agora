package handler_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	auditv1 "github.com/buidangphuc/team-payment/generated/platform/audit/v1"
	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/handler"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
	"github.com/buidangphuc/team-payment/internal/upstream"
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
	Actor, Payment, Order string
	Amount                int64
}

// fakeAudit is the AuditWriter at the true external boundary (team-audit).
type fakeAudit struct {
	log   *callLog
	mu    sync.Mutex
	calls []auditCall
	err   error
}

func (f *fakeAudit) WriteAdminRefund(_ context.Context, actorID, paymentID, orderID string, amount int64) error {
	f.log.add("audit")
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	f.calls = append(f.calls, auditCall{actorID, paymentID, orderID, amount})
	f.mu.Unlock()
	return nil
}

func (f *fakeAudit) written() []auditCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]auditCall(nil), f.calls...)
}

// flakyLedger fails ApplyRefund with a plain error while failN > 0 (-> Internal).
type flakyLedger struct {
	repository.SettlementLedger
	log   *callLog
	failN int
}

func (l *flakyLedger) ApplyRefund(ctx context.Context, req repository.RefundRequest) (repository.RefundResult, error) {
	l.log.add("refund")
	if l.failN > 0 {
		l.failN--
		return repository.RefundResult{}, errors.New("ledger update failed")
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
// failure-injectable ledger and, when withWriter, the fake audit writer.
func newAuditFixture(t *testing.T, writer func(*callLog) handler.AuditWriter) auditFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := &callLog{}
	f := ledgerFixture{
		payments: repository.NewInMemoryPaymentRepository(),
		ledger:   repository.NewInMemoryLedgerRepository(),
		wallets:  repository.NewInMemoryWalletRepository(),
	}
	orders := &mockOrderClient{orders: map[string]*orderv1.Order{
		"order-L": {Id: "order-L", BuyerId: "buyer-L", SellerId: "seller-L", TotalAmount: 500000, Status: orderv1.OrderStatus_ORDER_STATUS_PENDING},
	}}
	flaky := &flakyLedger{SettlementLedger: repository.NewInMemorySettlementLedger(f.payments, f.ledger), log: log}
	f.svc = service.NewPaymentService(f.payments, f.wallets, orders, logger,
		service.WithLedgerRepo(f.ledger), service.WithSettlementLedger(flaky), service.WithPayoutHold(0))
	opts := []handler.Option{handler.WithMockPayments(true)}
	var fa *fakeAudit
	if writer != nil {
		w := writer(log)
		opts = append(opts, handler.WithAuditWriter(w))
		fa, _ = w.(*fakeAudit)
	}
	f.h = handler.NewPaymentHandler(f.svc, logger, opts...)
	tx := f.payAndCredit(t)
	log.events = nil // payment setup is not part of the refund call order
	return auditFixture{ledgerFixture: f, log: log, audit: fa, flaky: flaky, txID: tx.GetId(), orderI: "order-L"}
}

func withFake(err error) func(*callLog) handler.AuditWriter {
	return func(l *callLog) handler.AuditWriter { return &fakeAudit{log: l, err: err} }
}

func admin(id string, extra ...string) context.Context {
	return principalCtx(id, commonv1.PrincipalType_PRINCIPAL_TYPE_USER, append([]string{"admin"}, extra...)...)
}

func (f auditFixture) read(t *testing.T) *paymentv1.PaymentTransaction {
	t.Helper()
	res, err := f.h.GetPayment(userCtx("buyer-L"), &paymentv1.GetPaymentRequest{Id: f.txID})
	if err != nil {
		t.Fatal(err)
	}
	return res.GetTransaction()
}

func (f auditFixture) deductions(t *testing.T) []repository.LedgerEntry {
	t.Helper()
	return entriesOf(t, f.ledgerFixture, "seller-L")[repository.LedgerTypeRefundDeduction]
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
	f := newAuditFixture(t, withFake(nil))
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
	want := []auditCall{{Actor: "adm-1", Payment: f.txID, Order: "order-L", Amount: 200000}}
	if got := f.audit.written(); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit events = %+v, want %+v", got, want)
	}
}

// Scenario: Audit is written before the refund
func TestAdminRefundAuditBeforeRefund(t *testing.T) {
	f := newAuditFixture(t, withFake(nil))
	if _, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000)); err != nil {
		t.Fatal(err)
	}
	if got, want := f.log.all(), []string{"audit", "refund"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
}

// ---- metadata, through the real upstream client ------------------------------------

type captureAudit struct {
	auditv1.UnimplementedAuditServiceServer
	mu    sync.Mutex
	reqs  []*auditv1.WriteAuditEventRequest
	block bool
}

func (c *captureAudit) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	c.mu.Lock()
	c.reqs = append(c.reqs, req)
	c.mu.Unlock()
	if c.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &auditv1.WriteAuditEventResponse{}, nil
}

func startCapture(t *testing.T, c *captureAudit) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	auditv1.RegisterAuditServiceServer(srv, c)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func realClient(t *testing.T, addr string, timeout time.Duration) *upstream.AuditClient {
	t.Helper()
	c, err := upstream.NewAuditClient(addr, timeout)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Scenario: Reason is not copied into the audit event
func TestAdminRefundAuditMetadataKeys(t *testing.T) {
	cap := &captureAudit{}
	client := realClient(t, startCapture(t, cap), time.Second)
	f := newAuditFixture(t, func(*callLog) handler.AuditWriter { return client })
	_, err := f.h.RefundPayment(admin("adm-1"), &paymentv1.RefundPaymentRequest{
		PaymentId: f.txID, RefundId: "R-1", Amount: 200000, Reason: "buyer complaint 123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cap.reqs) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(cap.reqs))
	}
	req := cap.reqs[0]
	keys := make([]string, 0, 2)
	for k := range req.GetMetadata() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"amount", "order_id"}) {
		t.Fatalf("metadata keys = %v, want exactly amount, order_id", keys)
	}
	if req.GetMetadata()["amount"] != strconv.Itoa(200000) || req.GetMetadata()["order_id"] != "order-L" ||
		req.GetAction() != "payment.admin_refund" || req.GetTargetType() != "payment" ||
		req.GetTargetId() != f.txID || req.GetActorId() != "adm-1" {
		t.Fatalf("event = %+v", req)
	}
}

// Scenario: Audit timeout is bounded and refuses the refund (handler half, real client)
func TestAdminRefundAuditTimeoutFailsClosed(t *testing.T) {
	cap := &captureAudit{block: true}
	client := realClient(t, startCapture(t, cap), 200*time.Millisecond)
	f := newAuditFixture(t, func(*callLog) handler.AuditWriter { return client })
	start := time.Now()
	_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("not bounded: %v", el)
	}
	f.assertUnchanged(t)
}

// ---- refusals before the audit write -----------------------------------------------

// Scenario: Refusal before the audit write writes no event
func TestAdminRefundRefusedBeforeAuditWritesNothing(t *testing.T) {
	f := newAuditFixture(t, withFake(nil))
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
	f := newAuditFixture(t, withFake(nil))
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
	f := newAuditFixture(t, withFake(status.Error(codes.Internal, "audit db down")))
	_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	if got := f.log.all(); !reflect.DeepEqual(got, []string{"audit"}) {
		t.Fatalf("calls = %v; the refund must not run after an audit failure", got)
	}
	f.assertUnchanged(t)
}

// Scenario: Payment audit: no audit upstream configured
func TestAdminRefundNilAuditWriterUnavailable(t *testing.T) {
	f := newAuditFixture(t, nil)
	_, err := f.h.RefundPayment(admin("adm-1"), refundReq(f.txID, "R-1", 200000))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	if got := f.log.all(); len(got) != 0 {
		t.Fatalf("refund ran without an audit writer: %v", got)
	}
	f.assertUnchanged(t)
}

// ---- idempotency -------------------------------------------------------------------

// Scenario: Retry after audit success and refund failure refunds once
func TestAdminRefundRetryAfterFailureRefundsOnce(t *testing.T) {
	f := newAuditFixture(t, withFake(nil))
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
	f := newAuditFixture(t, withFake(nil))
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
	f := newAuditFixture(t, withFake(nil))
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
	f := newAuditFixture(t, withFake(nil))
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
	f := newAuditFixture(t, withFake(nil))
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
	for name, mk := range map[string]func(*callLog) handler.AuditWriter{
		"audit errors":    withFake(status.Error(codes.Unavailable, "audit down")),
		"no audit writer": nil,
	} {
		f := newAuditFixture(t, mk)
		if _, err := f.h.RefundPayment(userCtx("seller-L"), refundReq(f.txID, "R-1", 200000)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if p := f.read(t); p.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_REFUNDED || p.GetRefundedAmount() != 200000 {
			t.Fatalf("%s: payment %v", name, p)
		}
	}
}

// Scenario: Non-seller non-admin is denied without an event
func TestRefundPaymentNonSellerDeniedNoAudit(t *testing.T) {
	f := newAuditFixture(t, withFake(nil))
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

var _ grpc.CallOption // keep grpc import tied to fakes above
