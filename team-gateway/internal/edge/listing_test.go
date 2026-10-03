package edge_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	listingv1 "github.com/buidangphuc/team-gateway/generated/platform/listing/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
)

// fakeListingClient records what reaches the upstream.
type fakeListingClient struct {
	listingv1.ListingServiceClient
	gotReq *listingv1.BatchGetStorefrontsRequest
	gotMD  metadata.MD
	calls  int
}

func (f *fakeListingClient) BatchGetStorefronts(
	ctx context.Context, in *listingv1.BatchGetStorefrontsRequest, _ ...grpc.CallOption,
) (*listingv1.BatchGetStorefrontsResponse, error) {
	f.calls++
	f.gotReq = in
	f.gotMD, _ = metadata.FromOutgoingContext(ctx)
	return &listingv1.BatchGetStorefrontsResponse{
		Shops: []*listingv1.ShopSummary{{SellerId: "a", DisplayName: "Shop Alpha", Slug: "a"}},
	}, nil
}

// The forwarder passes the request through unchanged, forwards the resolved
// (anonymous + public scopes) principal metadata and returns the upstream body.
func TestBatchGetStorefrontsForwardsUnchanged(t *testing.T) {
	up := &fakeListingClient{}
	e := edge.NewEdge(nil, []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000)
	f := edge.NewListingForwarder(up, e)

	req := connect.NewRequest(&listingv1.BatchGetStorefrontsRequest{SellerIds: []string{"a", "b", "a"}})
	res, err := f.BatchGetStorefronts(context.Background(), req)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if up.calls != 1 {
		t.Fatalf("want exactly one upstream call, got %d", up.calls)
	}
	if got := up.gotReq.GetSellerIds(); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "a" {
		t.Errorf("request must reach upstream unchanged (no dedupe/composition), got %v", got)
	}
	if v := up.gotMD.Get("x-principal-id"); len(v) != 1 || v[0] != "anonymous" {
		t.Errorf("principal id not forwarded: %v", v)
	}
	if v := up.gotMD.Get("x-principal-scopes"); len(v) != 1 || v[0] != "listing.read,search:read" {
		t.Errorf("principal scopes not forwarded: %v", v)
	}
	if len(res.Msg.GetShops()) != 1 || res.Msg.GetShops()[0].GetDisplayName() != "Shop Alpha" {
		t.Errorf("response not passed through: %v", res.Msg)
	}
}
