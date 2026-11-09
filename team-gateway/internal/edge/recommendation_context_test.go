package edge_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"

	recommendationv1 "github.com/buidangphuc/team-gateway/generated/platform/recommendation/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
)

type fakeRecommendationClient struct {
	recommendationv1.RecommendationServiceClient
	gotReq *recommendationv1.RecommendRequest
	calls  int
}

func (f *fakeRecommendationClient) Recommend(
	_ context.Context, in *recommendationv1.RecommendRequest, _ ...grpc.CallOption,
) (*recommendationv1.RecommendResponse, error) {
	f.calls++
	f.gotReq = in
	return &recommendationv1.RecommendResponse{}, nil
}

func contextIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("listing-%d", i)
	}
	return ids
}

func newRecommendationForwarderUnderTest(up *fakeRecommendationClient) *edge.RecommendationForwarder {
	e := edge.NewEdge(nil, []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000)
	return edge.NewRecommendationForwarder(up, e)
}

func TestRecommendRejects51ContextIdsWithoutUpstreamCall_GatewayRejects51Ids(t *testing.T) {
	up := &fakeRecommendationClient{}
	f := newRecommendationForwarderUnderTest(up)

	req := connect.NewRequest(&recommendationv1.RecommendRequest{
		ContextListingIds: contextIDs(51),
	})
	_, err := f.Recommend(context.Background(), req)

	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		t.Fatalf("want connect error INVALID_ARGUMENT, got %v", err)
	}
	if cerr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want invalid_argument", cerr.Code())
	}
	if up.calls != 0 {
		t.Errorf("upstream must not be called for 51 ids, got %d calls", up.calls)
	}
}

func TestRecommendForwards50ContextIdsUnchanged_GatewayForwardsContextIds(t *testing.T) {
	up := &fakeRecommendationClient{}
	f := newRecommendationForwarderUnderTest(up)
	want := contextIDs(50)

	req := connect.NewRequest(&recommendationv1.RecommendRequest{
		ContextListingIds: want,
	})
	if _, err := f.Recommend(context.Background(), req); err != nil {
		t.Fatalf("50 ids must be accepted: %v", err)
	}
	if up.calls != 1 {
		t.Fatalf("want exactly one upstream call, got %d", up.calls)
	}
	got := up.gotReq.GetContextListingIds()
	if len(got) != 50 {
		t.Fatalf("upstream got %d ids, want 50 (no truncation)", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("id %d = %q, want %q (order and values unchanged)", i, got[i], want[i])
		}
	}
}

func TestRecommendForwardsContextIdsToUpstreamRequest_GatewayForwardsContextIds(t *testing.T) {
	up := &fakeRecommendationClient{}
	f := newRecommendationForwarderUnderTest(up)

	req := connect.NewRequest(&recommendationv1.RecommendRequest{
		ContextListingIds: []string{"A", "B"},
	})
	if _, err := f.Recommend(context.Background(), req); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if up.calls != 1 {
		t.Fatalf("want one upstream call, got %d", up.calls)
	}
	got := up.gotReq.GetContextListingIds()
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Errorf("context ids must reach upstream, got %v", got)
	}
}
