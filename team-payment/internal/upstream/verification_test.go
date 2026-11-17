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

	verificationv1 "github.com/buidangphuc/team-payment/generated/platform/verification/v1"
)

type recordingVerification struct {
	verificationv1.UnimplementedVerificationServiceServer
	mu     sync.Mutex
	reqs   []*verificationv1.GetVerificationStatusRequest
	mds    []metadata.MD
	status verificationv1.VerificationStatus
	err    error
	block  bool
}

func (r *recordingVerification) GetVerificationStatus(ctx context.Context, req *verificationv1.GetVerificationStatusRequest) (*verificationv1.GetVerificationStatusResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mds = append(r.mds, md)
	r.mu.Unlock()
	if r.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if r.err != nil {
		return nil, r.err
	}
	return &verificationv1.GetVerificationStatusResponse{Status: r.status}, nil
}

func (r *recordingVerification) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func startVerification(t *testing.T, svc *recordingVerification) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	verificationv1.RegisterVerificationServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func dialGate(t *testing.T, addr string) *PayoutGate {
	t.Helper()
	client, conn, err := DialVerificationService(addr)
	if err != nil {
		t.Fatalf("DialVerificationService: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewPayoutGate(client)
}

func wantCode(t *testing.T, err error, code codes.Code, msg string) {
	t.Helper()
	if status.Code(err) != code {
		t.Fatalf("code = %v (%v), want %v", status.Code(err), err, code)
	}
	if got := status.Convert(err).Message(); got != msg {
		t.Errorf("message = %q, want %q", got, msg)
	}
}

// Scenario: Verified seller requests a bank payout (gate passes for VERIFIED)
func TestPayoutGateVerifiedPasses(t *testing.T) {
	rec := &recordingVerification{status: verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED}
	g := dialGate(t, startVerification(t, rec))
	if err := g.Require(context.Background(), "seller-1"); err != nil {
		t.Fatalf("Require: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 || rec.reqs[0].GetUserId() != "seller-1" {
		t.Errorf("want one status call for seller-1, got %v", rec.reqs)
	}
}

// Scenario: Seller who never submitted is refused / Seller with a rejected submission is refused
func TestPayoutGateNonVerifiedStatusesRefused(t *testing.T) {
	cases := map[string]verificationv1.VerificationStatus{
		"never submitted (Seller who never submitted is refused)":        verificationv1.VerificationStatus_VERIFICATION_STATUS_NOT_SUBMITTED,
		"rejected (Seller with a rejected submission is refused)":        verificationv1.VerificationStatus_VERIFICATION_STATUS_REJECTED,
		"pending (Seller with a pending submission is refused)":          verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING,
		"unspecified (Seller who never submitted is refused, fail safe)": verificationv1.VerificationStatus_VERIFICATION_STATUS_UNSPECIFIED,
	}
	for name, st := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &recordingVerification{status: st}
			g := dialGate(t, startVerification(t, rec))
			wantCode(t, g.Require(context.Background(), "seller-1"), codes.FailedPrecondition, "seller verification required")
		})
	}
}

// Scenario: Verification service unreachable
func TestPayoutGateTransportErrorIsUnavailable(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	g := dialGate(t, addr)
	start := time.Now()
	wantCode(t, g.Require(context.Background(), "seller-1"), codes.Unavailable, "verification unavailable")
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v, want under 3s", d)
	}
}

// Scenario: Verification rejects the service principal
func TestPayoutGatePermissionDeniedIsUnavailable(t *testing.T) {
	rec := &recordingVerification{err: status.Error(codes.PermissionDenied, "no scope")}
	g := dialGate(t, startVerification(t, rec))
	wantCode(t, g.Require(context.Background(), "seller-1"), codes.Unavailable, "verification unavailable")
}

// Scenario: Verification rejects the service principal (NotFound and internal errors also fail closed)
func TestPayoutGateOtherErrorsAreUnavailable(t *testing.T) {
	for _, c := range []codes.Code{codes.NotFound, codes.Internal, codes.Unauthenticated} {
		t.Run(c.String(), func(t *testing.T) {
			rec := &recordingVerification{err: status.Error(c, "x")}
			g := dialGate(t, startVerification(t, rec))
			wantCode(t, g.Require(context.Background(), "seller-1"), codes.Unavailable, "verification unavailable")
		})
	}
}

// Scenario: Verification call exceeds the deadline
func TestPayoutGateSlowServerUnavailableOneAttempt(t *testing.T) {
	rec := &recordingVerification{block: true}
	g := dialGate(t, startVerification(t, rec))
	start := time.Now()
	err := g.Require(context.Background(), "seller-1")
	elapsed := time.Since(start)
	wantCode(t, err, codes.Unavailable, "verification unavailable")
	if elapsed > 2500*time.Millisecond {
		t.Errorf("took %v, want at most 2.5s", elapsed)
	}
	if n := rec.calls(); n != 1 {
		t.Errorf("attempts = %d, want exactly 1", n)
	}
}

// Scenario: Verification rejects the service principal (call is made as service-team-payment with verification.read)
func TestPayoutGateUsesServicePrincipal(t *testing.T) {
	rec := &recordingVerification{status: verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED}
	g := dialGate(t, startVerification(t, rec))
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "seller-1", "x-principal-type", "user", "x-principal-scopes", "listing.write"))
	if err := g.Require(ctx, "seller-1"); err != nil {
		t.Fatalf("Require: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	want := map[string]string{
		"x-principal-id":     "service-team-payment",
		"x-principal-type":   "service",
		"x-principal-scopes": "verification.read",
	}
	for k, v := range want {
		if vals := rec.mds[0].Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}
