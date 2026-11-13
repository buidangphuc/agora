package upstream

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

	paymentv1 "github.com/buidangphuc/team-promotion/generated/platform/payment/v1"
)

type feeServer struct {
	paymentv1.UnimplementedPaymentServiceServer
	mu      sync.Mutex
	reqs    []*paymentv1.ChargeSellerFeeRequest
	mds     []metadata.MD
	created bool
	err     error
	block   bool
}

func (s *feeServer) ChargeSellerFee(ctx context.Context, req *paymentv1.ChargeSellerFeeRequest) (*paymentv1.ChargeSellerFeeResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.mu.Lock()
	s.reqs = append(s.reqs, req)
	s.mds = append(s.mds, md)
	s.mu.Unlock()
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	return &paymentv1.ChargeSellerFeeResponse{EntryId: "entry-1", Created: s.created}, nil
}

func startPayments(t *testing.T, svc *feeServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	paymentv1.RegisterPaymentServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func newPaymentClient(t *testing.T, addr string, timeout time.Duration) *PaymentClient {
	t.Helper()
	c, err := NewPaymentClient(addr, timeout)
	if err != nil || c == nil {
		t.Fatalf("NewPaymentClient: %v, %v", c, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestChargeSellerFeeSendsSubscriptionCharge(t *testing.T) {
	// Scenario: FREE seller subscribes to PRO
	svc := &feeServer{created: true}
	c := newPaymentClient(t, startPayments(t, svc), time.Second)
	created, err := c.ChargeSellerFee(t.Context(), "seller-1", 199000, "sub:abc:1:plan_pro")
	if err != nil || !created {
		t.Fatalf("ChargeSellerFee = %v, %v, want true nil", created, err)
	}
	if len(svc.reqs) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(svc.reqs))
	}
	r := svc.reqs[0]
	if r.GetSellerId() != "seller-1" || r.GetAmount() != 199000 || r.GetReferenceId() != "sub:abc:1:plan_pro" ||
		r.GetKind() != paymentv1.SellerFeeKind_SELLER_FEE_KIND_SUBSCRIPTION {
		t.Fatalf("request = %+v, want seller-1 199000 SUBSCRIPTION sub:abc:1:plan_pro", r)
	}
}

func TestChargeSellerFeeCarriesServicePrincipal(t *testing.T) {
	// Scenario: FREE seller subscribes to PRO
	svc := &feeServer{created: true}
	c := newPaymentClient(t, startPayments(t, svc), time.Second)
	caller := metadata.NewOutgoingContext(t.Context(), metadata.Pairs(
		"x-principal-id", "seller-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if _, err := c.ChargeSellerFee(caller, "seller-1", 199000, "sub:abc:1:plan_pro"); err != nil {
		t.Fatal(err)
	}
	if len(svc.mds) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(svc.mds))
	}
	for k, v := range map[string]string{
		"x-principal-id":     "service-team-promotion",
		"x-principal-type":   "service",
		"x-principal-scopes": "payment:write",
	} {
		if got := svc.mds[0].Get(k); len(got) != 1 || got[0] != v {
			t.Errorf("%s = %v, want [%s]", k, got, v)
		}
	}
}

func TestChargeSellerFeeReplayReportsNotCreated(t *testing.T) {
	// Scenario: Retry after a lost response is not charged twice
	svc := &feeServer{created: false}
	c := newPaymentClient(t, startPayments(t, svc), time.Second)
	created, err := c.ChargeSellerFee(t.Context(), "seller-1", 199000, "sub:abc:1:plan_pro")
	if err != nil || created {
		t.Fatalf("ChargeSellerFee = %v, %v, want false nil on a replay", created, err)
	}
}

func TestChargeSellerFeeKeepsFailedPrecondition(t *testing.T) {
	// Scenario: Insufficient withdrawable balance is rejected
	svc := &feeServer{err: status.Error(codes.FailedPrecondition, "insufficient withdrawable balance")}
	c := newPaymentClient(t, startPayments(t, svc), time.Second)
	created, err := c.ChargeSellerFee(t.Context(), "seller-1", 199000, "sub:abc:1:plan_pro")
	if status.Code(err) != codes.FailedPrecondition || created {
		t.Fatalf("ChargeSellerFee = %v, %v, want false and FailedPrecondition", created, err)
	}
}

func TestChargeSellerFeeOtherErrorsAreUnavailable(t *testing.T) {
	// Scenario: Payment service unavailable fails without state change
	for name, serverErr := range map[string]error{
		"internal":         status.Error(codes.Internal, "boom"),
		"unavailable":      status.Error(codes.Unavailable, "down"),
		"permission":       status.Error(codes.PermissionDenied, "missing payment:write"),
		"invalid argument": status.Error(codes.InvalidArgument, "amount"),
	} {
		t.Run(name, func(t *testing.T) {
			c := newPaymentClient(t, startPayments(t, &feeServer{err: serverErr}), time.Second)
			created, err := c.ChargeSellerFee(t.Context(), "seller-1", 199000, "sub:abc:1:plan_pro")
			if status.Code(err) != codes.Unavailable || created {
				t.Fatalf("ChargeSellerFee = %v, %v, want false and Unavailable", created, err)
			}
		})
	}
}

func TestChargeSellerFeeTimeoutIsUnavailable(t *testing.T) {
	// Scenario: Payment service unavailable fails without state change
	c := newPaymentClient(t, startPayments(t, &feeServer{block: true}), 50*time.Millisecond)
	start := time.Now()
	created, err := c.ChargeSellerFee(t.Context(), "seller-1", 199000, "sub:abc:1:plan_pro")
	if status.Code(err) != codes.Unavailable || created {
		t.Fatalf("ChargeSellerFee = %v, %v, want false and Unavailable", created, err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("call took %v, the client timeout was not applied", elapsed)
	}
}

func TestChargeSellerFeeNilClientIsUnavailable(t *testing.T) {
	// Scenario: Payment service unavailable fails without state change
	var c *PaymentClient
	if _, err := c.ChargeSellerFee(t.Context(), "seller-1", 199000, "k"); status.Code(err) != codes.Unavailable {
		t.Fatalf("nil client err = %v, want Unavailable", err)
	}
}
