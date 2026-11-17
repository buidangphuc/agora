package upstream_test

// Task 1.3 (identity-account-deletion): clients for OrderService.GetUserObligations and
// PaymentService.GetPayoutObligations.
//
// API assumed by these tests:
//   - upstream.NewObligations(orderConn, paymentConn *grpc.ClientConn) *upstream.Obligations
//   - (*Obligations).UserObligations(ctx, userID) (upstream.UserObligations, error)
//   - (*Obligations).PayoutObligations(ctx, userID) (upstream.PayoutObligations, error)
//   - connections are built by the caller with the SDK client (service principal, 2 s default
//     timeout); the clients themselves add no retry and return the upstream error unchanged.

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sdkup "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"
	orderv1 "github.com/buidangphuc/team-identity/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-identity/generated/platform/payment/v1"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

type obRec struct {
	mu        sync.Mutex
	ids       []string
	mds       []metadata.MD
	remaining []time.Duration
	err       error
	block     bool
}

func (r *obRec) record(ctx context.Context, id string) error {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.ids = append(r.ids, id)
	r.mds = append(r.mds, md)
	if dl, ok := ctx.Deadline(); ok {
		r.remaining = append(r.remaining, time.Until(dl))
	}
	block, err := r.block, r.err
	r.mu.Unlock()
	if block {
		<-ctx.Done()
		return status.FromContextError(ctx.Err()).Err()
	}
	return err
}

func (r *obRec) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ids)
}

type fakeOrder struct {
	orderv1.UnimplementedOrderServiceServer
	obRec
	resp *orderv1.GetUserObligationsResponse
}

func (f *fakeOrder) GetUserObligations(ctx context.Context, req *orderv1.GetUserObligationsRequest) (*orderv1.GetUserObligationsResponse, error) {
	if err := f.record(ctx, req.GetUserId()); err != nil {
		return nil, err
	}
	return f.resp, nil
}

type fakePayment struct {
	paymentv1.UnimplementedPaymentServiceServer
	obRec
	resp *paymentv1.GetPayoutObligationsResponse
}

func (f *fakePayment) GetPayoutObligations(ctx context.Context, req *paymentv1.GetPayoutObligationsRequest) (*paymentv1.GetPayoutObligationsResponse, error) {
	if err := f.record(ctx, req.GetUserId()); err != nil {
		return nil, err
	}
	return f.resp, nil
}

func serve(t *testing.T, reg func(*grpc.Server), opts ...sdkup.Option) *grpc.ClientConn {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	reg(srv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	opts = append([]sdkup.Option{sdkup.AsService("service-team-identity", "obligations.read")}, opts...)
	conn, err := sdkup.NewClient(lis.Addr().String(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func newObligations(t *testing.T, o *fakeOrder, p *fakePayment, opts ...sdkup.Option) *upstream.Obligations {
	t.Helper()
	oc := serve(t, func(s *grpc.Server) { orderv1.RegisterOrderServiceServer(s, o) }, opts...)
	pc := serve(t, func(s *grpc.Server) { paymentv1.RegisterPaymentServiceServer(s, p) }, opts...)
	c := upstream.NewObligations(oc, pc)
	if c == nil {
		t.Fatal("NewObligations returned nil")
	}
	return c
}

func userCtx() context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "u-1", "x-principal-type", "user"))
}

// Scenario: Zero obligations on every counter allows deletion (counters are mapped, request carries the user id)
func TestObligationsClientMapsCountersAndUserID(t *testing.T) {
	o := &fakeOrder{resp: &orderv1.GetUserObligationsResponse{OpenBuyerOrders: 1, OpenSellerOrders: 2, OpenReturns: 3}}
	p := &fakePayment{resp: &paymentv1.GetPayoutObligationsResponse{AvailableBalance: 50000, HeldBalance: 700, PendingPayouts: 4}}
	c := newObligations(t, o, p)

	u, err := c.UserObligations(userCtx(), "u-1")
	if err != nil {
		t.Fatalf("UserObligations: %v", err)
	}
	if u != (upstream.UserObligations{OpenBuyerOrders: 1, OpenSellerOrders: 2, OpenReturns: 3}) {
		t.Fatalf("user obligations = %+v", u)
	}
	pay, err := c.PayoutObligations(userCtx(), "u-1")
	if err != nil {
		t.Fatalf("PayoutObligations: %v", err)
	}
	if pay != (upstream.PayoutObligations{AvailableBalance: 50000, HeldBalance: 700, PendingPayouts: 4}) {
		t.Fatalf("payout obligations = %+v", pay)
	}
	if len(o.ids) != 1 || o.ids[0] != "u-1" || len(p.ids) != 1 || p.ids[0] != "u-1" {
		t.Fatalf("user ids sent: order=%v payment=%v", o.ids, p.ids)
	}
}

// Scenario: Obligations are checked for the caller only (calls use the service principal, never the user's)
func TestObligationsClientUsesServicePrincipal(t *testing.T) {
	o := &fakeOrder{resp: &orderv1.GetUserObligationsResponse{}}
	p := &fakePayment{resp: &paymentv1.GetPayoutObligationsResponse{}}
	c := newObligations(t, o, p)
	if _, err := c.UserObligations(userCtx(), "u-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PayoutObligations(userCtx(), "u-1"); err != nil {
		t.Fatal(err)
	}
	for name, md := range map[string]metadata.MD{"order": o.mds[0], "payment": p.mds[0]} {
		if got := md.Get("x-principal-type"); len(got) != 1 || got[0] != "service" {
			t.Errorf("%s x-principal-type = %v, want [service]", name, got)
		}
		if got := md.Get("x-principal-id"); len(got) != 1 || got[0] != "service-team-identity" {
			t.Errorf("%s x-principal-id = %v, want the service id, never the user", name, got)
		}
	}
}

// Scenario: Order service unreachable fails closed (single attempt, error surfaced)
func TestObligationsClientOrderErrorSingleAttempt(t *testing.T) {
	o := &fakeOrder{obRec: obRec{err: status.Error(codes.Unavailable, "down")}}
	p := &fakePayment{resp: &paymentv1.GetPayoutObligationsResponse{}}
	c := newObligations(t, o, p)
	_, err := c.UserObligations(userCtx(), "u-1")
	if err == nil {
		t.Fatal("want the upstream error, got nil")
	}
	if o.calls() != 1 {
		t.Fatalf("order called %d times, want exactly 1 (no retry)", o.calls())
	}
}

// Scenario: Payment service unreachable fails closed (call is bounded by a timeout)
func TestObligationsClientPaymentTimeoutSingleAttempt(t *testing.T) {
	o := &fakeOrder{resp: &orderv1.GetUserObligationsResponse{}}
	p := &fakePayment{obRec: obRec{block: true}}
	c := newObligations(t, o, p, sdkup.WithTimeout(50*time.Millisecond))
	_, err := c.PayoutObligations(userCtx(), "u-1")
	if err == nil {
		t.Fatal("want a deadline error, got nil")
	}
	if code := status.Code(err); code != codes.DeadlineExceeded {
		t.Fatalf("code = %v, want DeadlineExceeded", code)
	}
	if p.calls() != 1 {
		t.Fatalf("payment called %d times, want exactly 1 (no retry)", p.calls())
	}
}

// Scenario: Payment service unreachable fails closed (default bound is 2 s)
func TestObligationsClientDefaultTimeoutIsTwoSeconds(t *testing.T) {
	o := &fakeOrder{resp: &orderv1.GetUserObligationsResponse{}}
	p := &fakePayment{resp: &paymentv1.GetPayoutObligationsResponse{}}
	c := newObligations(t, o, p)
	if _, err := c.UserObligations(userCtx(), "u-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PayoutObligations(userCtx(), "u-1"); err != nil {
		t.Fatal(err)
	}
	for name, r := range map[string][]time.Duration{"order": o.remaining, "payment": p.remaining} {
		if len(r) != 1 || r[0] <= time.Second || r[0] > 2*time.Second {
			t.Errorf("%s deadline remaining = %v, want within (1s, 2s]", name, r)
		}
	}
}
