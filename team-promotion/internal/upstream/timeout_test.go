package upstream

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
)

type hangingListings struct{}

func (hangingListings) GetListing(ctx context.Context, _ *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	<-ctx.Done()
	return nil, status.FromContextError(ctx.Err()).Err()
}

func TestWithTimeoutBoundsHangingLookup(t *testing.T) {
	g := WithTimeout(hangingListings{}, 50*time.Millisecond)
	start := time.Now()
	_, err := g.GetListing(context.Background(), &listingv1.GetListingRequest{Id: "l1"})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("lookup not bounded: %v", time.Since(start))
	}
}

func TestBoundedCtxRespectsInboundDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	ctx, c := boundedCtx(parent, 10*time.Second)
	defer c()
	dl, _ := ctx.Deadline()
	if rem := time.Until(dl); rem > 200*time.Millisecond {
		t.Fatalf("budget %v exceeds inbound deadline minus margin", rem)
	}

	expired, c2 := boundedCtx(func() context.Context {
		p, cc := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_ = cc
		return p
	}(), time.Second)
	defer c2()
	if expired.Err() == nil {
		select {
		case <-expired.Done():
		default:
			t.Fatal("context inside the margin must be already expired")
		}
	}
}

func TestWithTimeoutNilStaysNil(t *testing.T) {
	if WithTimeout(nil, time.Second) != nil {
		t.Fatal("nil getter must stay nil so callers fail closed")
	}
}
