package edge_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	paymentv1 "github.com/buidangphuc/team-gateway/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const (
	getMySubscriptionPath    = "/platform.promotion.v1.SubscriptionService/GetMySubscription"
	cancelSubscriptionPath   = "/platform.promotion.v1.SubscriptionService/CancelSubscription"
	chargeSellerFeeEdgePath  = "/platform.payment.v1.PaymentService/ChargeSellerFee"
	chargeSellerFeeEdgeBody  = `{"sellerId":"seller-1","amount":{"currency":"VND","units":"1"}}`
	subscriptionPrincipalKey = "x-principal-id"
)

type subscriptionNewRPCUpstream struct {
	promotionv1.SubscriptionServiceClient
	getCalls    atomic.Int64
	cancelCalls atomic.Int64
	gotPID      atomic.Value
}

func (s *subscriptionNewRPCUpstream) record(ctx context.Context) {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if p := md.Get(subscriptionPrincipalKey); len(p) > 0 {
			s.gotPID.Store(p[0])
		}
	}
}

func (s *subscriptionNewRPCUpstream) GetMySubscription(ctx context.Context, _ *promotionv1.GetMySubscriptionRequest, _ ...grpc.CallOption) (*promotionv1.GetMySubscriptionResponse, error) {
	s.getCalls.Add(1)
	s.record(ctx)
	return &promotionv1.GetMySubscriptionResponse{}, nil
}

func (s *subscriptionNewRPCUpstream) CancelSubscription(ctx context.Context, _ *promotionv1.CancelSubscriptionRequest, _ ...grpc.CallOption) (*promotionv1.CancelSubscriptionResponse, error) {
	s.cancelCalls.Add(1)
	s.record(ctx)
	return &promotionv1.CancelSubscriptionResponse{}, nil
}

type paymentChargeSpy struct {
	paymentv1.PaymentServiceClient
	chargeCalls atomic.Int64
}

func (p *paymentChargeSpy) ChargeSellerFee(context.Context, *paymentv1.ChargeSellerFeeRequest, ...grpc.CallOption) (*paymentv1.ChargeSellerFeeResponse, error) {
	p.chargeCalls.Add(1)
	return &paymentv1.ChargeSellerFeeResponse{}, nil
}

// Scenario: Unauthenticated call is denied at the edge.
func TestSubscriptionRoutesRequireAuth(t *testing.T) {
	for _, path := range []string{getMySubscriptionPath, cancelSubscriptionPath} {
		up := &subscriptionNewRPCUpstream{}
		code, body := callWith(t, &upstream.Clients{Subscription: up}, path, `{}`, nil)
		if code != http.StatusUnauthorized {
			t.Errorf("%s without token = %d %s, want 401 unauthenticated", path, code, body)
		}
		if n := up.getCalls.Load() + up.cancelCalls.Load(); n != 0 {
			t.Errorf("%s without token reached team-promotion %d time(s)", path, n)
		}
	}
}

// Scenario: Gateway forwards the new RPCs and never ChargeSellerFee.
func TestSubscriptionForwardsNewRPCsOnly(t *testing.T) {
	up := &subscriptionNewRPCUpstream{}
	pay := &paymentChargeSpy{}
	clients := &upstream.Clients{Subscription: up, Payment: pay}

	code, body := callWith(t, clients, getMySubscriptionPath, `{}`, []string{"seller"})
	if code != http.StatusOK {
		t.Fatalf("GetMySubscription = %d %s, want 200", code, body)
	}
	code, body = callWith(t, clients, cancelSubscriptionPath, `{}`, []string{"seller"})
	if code != http.StatusOK {
		t.Fatalf("CancelSubscription = %d %s, want 200", code, body)
	}
	if up.getCalls.Load() != 1 || up.cancelCalls.Load() != 1 {
		t.Fatalf("upstream calls get=%d cancel=%d, want 1 each", up.getCalls.Load(), up.cancelCalls.Load())
	}
	if pid, _ := up.gotPID.Load().(string); pid != "user-1" {
		t.Errorf("principal id forwarded = %q, want user-1", pid)
	}

	for _, scopes := range [][]string{{"seller"}, {"admin"}, nil} {
		code, body = callWith(t, clients, chargeSellerFeeEdgePath, chargeSellerFeeEdgeBody, scopes)
		if code < 400 {
			t.Errorf("ChargeSellerFee via gateway (scopes %v) = %d %s, want an error status", scopes, code, body)
		}
	}
	if pay.chargeCalls.Load() != 0 {
		t.Fatalf("ChargeSellerFee reached team-payment %d time(s), want 0", pay.chargeCalls.Load())
	}
}
