package edge_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
)

// stockSearch is an upstream SearchService whose SearchListings returns a fixed
// response, so the test sees exactly what the edge does to SearchHit.stock.
type stockSearch struct {
	searchv1.SearchServiceClient
	resp *searchv1.SearchListingsResponse
}

func (s stockSearch) SearchListings(context.Context, *searchv1.SearchListingsRequest, ...grpc.CallOption) (*searchv1.SearchListingsResponse, error) {
	return s.resp, nil
}

// SearchHit.stock (optional int32 = 3) must cross the edge in Connect JSON with
// presence intact: a projected stock of 0 is the key "stock": 0, an unknown
// stock is no key at all.
func TestSearchListingsStockRoundTripsInConnectJSON(t *testing.T) {
	up := stockSearch{resp: &searchv1.SearchListingsResponse{Hits: []*searchv1.SearchHit{
		{ListingId: "zero", Score: 1, Stock: proto.Int32(0)},
		{ListingId: "unknown", Score: 1},
		{ListingId: "eight", Score: 1, Stock: proto.Int32(8)},
	}}}
	e := edge.NewEdge(nil, []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.NewTextHandler(io.Discard, nil)))...)
	mux := http.NewServeMux()
	p, h := searchv1connect.NewSearchServiceHandler(edge.NewSearchForwarder(up, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := http.Post(srv.URL+searchv1connect.SearchServiceSearchListingsProcedure, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", res.StatusCode, raw)
	}
	var body struct {
		Hits []map[string]json.RawMessage `json:"hits"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	if len(body.Hits) != 3 {
		t.Fatalf("want 3 hits, got %s", raw)
	}
	if v, ok := body.Hits[0]["stock"]; !ok || string(v) != "0" {
		t.Errorf(`hit with stock 0 must carry "stock": 0, got %s`, raw)
	}
	if _, ok := body.Hits[1]["stock"]; ok {
		t.Errorf("hit with unknown stock must have no stock key, got %s", raw)
	}
	if v := string(body.Hits[2]["stock"]); v != "8" {
		t.Errorf(`hit with stock 8 must carry "stock": 8, got %s`, raw)
	}
}
