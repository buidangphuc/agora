package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	listingv1 "github.com/buidangphuc/team-gateway/generated/platform/listing/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type planCapListingUpstream struct {
	listingv1.ListingServiceClient
	err error
}

func (u *planCapListingUpstream) CreateListing(context.Context, *listingv1.CreateListingRequest, ...grpc.CallOption) (*listingv1.CreateListingResponse, error) {
	return nil, u.err
}

func TestCreateListingKeepsPlanListingLimitReason(t *testing.T) {
	f := newCockpitFixture(t)
	up := &planCapListingUpstream{err: errWithInfo(t, codes.ResourceExhausted, "listing limit of plan reached", "team-domain", "PLAN_LISTING_LIMIT")}
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Listing: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform.listing.v1.ListingService/CreateListing", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.token(t, "seller"))
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	body := string(b)
	if res.StatusCode != http.StatusTooManyRequests || !strings.Contains(body, "resource_exhausted") {
		t.Fatalf("CreateListing at cap = %d %s, want 429 resource_exhausted", res.StatusCode, body)
	}
	if !strings.Contains(body, "PLAN_LISTING_LIMIT") || !strings.Contains(body, "team-domain") {
		t.Fatalf("CreateListing at cap lost ErrorInfo: %s", body)
	}
}
