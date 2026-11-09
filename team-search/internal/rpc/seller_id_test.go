package rpc_test

import (
	"context"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

type sellerHitIndex struct{ mockIndex }

func (s *sellerHitIndex) Search(context.Context, string, map[string]string, string, int64, int64, int32, searchv1.SortBy, int, int) (index.SearchResult, error) {
	return index.SearchResult{Hits: []index.Hit{{ListingID: "l1", SellerID: "seller-9"}}, Total: 1}, nil
}

func TestSearchListings_C4_HitCarriesSellerID(t *testing.T) {
	h := newHandler(t, &sellerHitIndex{}, newFakeSaved())
	resp, err := h.SearchListings(principalCtx("anonymous", "anonymous", "search:read"), &searchv1.SearchListingsRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetHits()) != 1 || resp.GetHits()[0].GetSellerId() != "seller-9" {
		t.Fatalf("hits = %v, want seller_id seller-9", resp.GetHits())
	}
}
