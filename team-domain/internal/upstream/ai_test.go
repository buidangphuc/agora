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

	aiv1 "github.com/buidangphuc/team-domain/generated/platform/ai/v1"
)

// fakeAI is the team-ai boundary: it records what it receives.
type fakeAI struct {
	aiv1.UnimplementedAIServiceServer
	mu    sync.Mutex
	reqs  []*aiv1.ScreenListingRequest
	mds   []metadata.MD
	resp  *aiv1.ScreenListingResponse
	err   error
	block bool
}

func (f *fakeAI) ScreenListing(ctx context.Context, req *aiv1.ScreenListingRequest) (*aiv1.ScreenListingResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mds = append(f.mds, md)
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func startAI(t *testing.T, svc *fakeAI) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	aiv1.RegisterAIServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

// Scenario: Auto-screen: Unconfigured team-ai fails safe.
func TestNewAIClientEmptyAddrIsNil(t *testing.T) {
	c, err := NewAIClient("", time.Second)
	if err != nil || c != nil {
		t.Fatalf("NewAIClient(\"\") = %v, %v; want nil, nil", c, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
	// The nil client must report an error (never a verdict), so the caller fails safe.
	res, err := c.ScreenListing(context.Background(), ScreenInput{Title: "t"})
	if err == nil || res != nil {
		t.Fatalf("nil client ScreenListing = %v, %v; want nil result and an error", res, err)
	}
}

// Scenario: Auto-screen: Screen call uses the service principal.
func TestScreenListingUsesServicePrincipal(t *testing.T) {
	f := &fakeAI{resp: &aiv1.ScreenListingResponse{Verdict: aiv1.ScreenVerdict_SCREEN_VERDICT_ALLOW, Backend: "mock"}}
	c, err := NewAIClient(startAI(t, f), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAIClient: %v, %v", c, err)
	}
	defer c.Close()

	// The inbound request carries the seller's principal; it must not be forwarded.
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "seller-1",
		"x-principal-type", "user",
		"x-principal-scopes", "seller",
	))
	if _, err := c.ScreenListing(ctx, ScreenInput{ListingID: "L-1", Title: "Ao thun", Description: "d", CategoryID: "c-1"}); err != nil {
		t.Fatalf("ScreenListing: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.mds) != 1 {
		t.Fatalf("team-ai got %d calls, want 1", len(f.mds))
	}
	md := f.mds[0]
	want := map[string]string{
		"x-principal-id":     "service-team-domain",
		"x-principal-type":   "service",
		"x-principal-scopes": "ai.classify",
	}
	for k, v := range want {
		got := md.Get(k)
		if len(got) != 1 || got[0] != v {
			t.Errorf("metadata %s = %v, want exactly [%s]", k, got, v)
		}
	}
	r := f.reqs[0]
	if r.GetListingId() != "L-1" || r.GetTitle() != "Ao thun" || r.GetDescription() != "d" || r.GetCategoryId() != "c-1" {
		t.Errorf("request fields not forwarded: %+v", r)
	}
}

// Scenario: Auto-screen: Screen call uses the service principal (verdict and labels pass through).
func TestScreenListingReturnsVerdictAndLabels(t *testing.T) {
	f := &fakeAI{resp: &aiv1.ScreenListingResponse{
		Verdict: aiv1.ScreenVerdict_SCREEN_VERDICT_REVIEW, Labels: []string{"keyword:replica"}, Backend: "mock",
	}}
	c, err := NewAIClient(startAI(t, f), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAIClient: %v, %v", c, err)
	}
	defer c.Close()
	res, err := c.ScreenListing(context.Background(), ScreenInput{Title: "replica bag"})
	if err != nil {
		t.Fatalf("ScreenListing: %v", err)
	}
	if res.GetVerdict() != aiv1.ScreenVerdict_SCREEN_VERDICT_REVIEW {
		t.Errorf("verdict = %v, want REVIEW", res.GetVerdict())
	}
	if l := res.GetLabels(); len(l) != 1 || l[0] != "keyword:replica" {
		t.Errorf("labels = %v, want [keyword:replica]", l)
	}
}

// Scenario: Auto-screen: AI timeout fails safe within the bound.
func TestScreenListingTimeoutBounded(t *testing.T) {
	f := &fakeAI{block: true}
	c, err := NewAIClient(startAI(t, f), 300*time.Millisecond)
	if err != nil || c == nil {
		t.Fatalf("NewAIClient: %v, %v", c, err)
	}
	defer c.Close()
	start := time.Now()
	res, err := c.ScreenListing(context.Background(), ScreenInput{Title: "t"})
	elapsed := time.Since(start)
	if err == nil || res != nil {
		t.Fatalf("hung team-ai: got %v, %v; want nil result and an error", res, err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("call took %v; must return near the 300ms bound", elapsed)
	}
	if got := status.Code(unwrapStatus(err)); got != codes.DeadlineExceeded {
		t.Errorf("error code = %v, want DeadlineExceeded", got)
	}
}

// Scenario: Auto-screen: AI timeout fails safe within the bound (inbound deadline caps the call).
func TestScreenListingRespectsInboundDeadline(t *testing.T) {
	f := &fakeAI{block: true}
	c, err := NewAIClient(startAI(t, f), 30*time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAIClient: %v, %v", c, err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = c.ScreenListing(ctx, ScreenInput{Title: "t"})
	if err == nil {
		t.Fatal("want an error from a hung team-ai")
	}
	if got := status.Code(unwrapStatus(err)); got != codes.DeadlineExceeded {
		t.Errorf("error code = %v, want DeadlineExceeded", got)
	}
	// boundedCtx leaves a margin of the inbound deadline for the caller's own work.
	if el := time.Since(start); el > 1200*time.Millisecond-deadlineMargin+400*time.Millisecond {
		t.Fatalf("call took %v; must stop before the inbound deadline minus the margin", el)
	}
}

// Scenario: Auto-screen: Unconfigured team-ai fails safe (an upstream error is an error, never ALLOW).
func TestScreenListingUpstreamErrorIsError(t *testing.T) {
	f := &fakeAI{err: status.Error(codes.PermissionDenied, "no scope")}
	c, err := NewAIClient(startAI(t, f), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAIClient: %v, %v", c, err)
	}
	defer c.Close()
	res, err := c.ScreenListing(context.Background(), ScreenInput{Title: "t"})
	if err == nil || res != nil {
		t.Fatalf("got %v, %v; want nil result and an error", res, err)
	}
}

func unwrapStatus(err error) error {
	for err != nil {
		if _, ok := status.FromError(err); ok {
			if s, _ := status.FromError(err); s.Code() != codes.Unknown {
				return err
			}
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
	return err
}
