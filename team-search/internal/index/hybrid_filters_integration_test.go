package index_test

import (
	"context"
	"math"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

// unitAt returns a unit vector whose cosine similarity to make384() is cos.
func unitAt(cos float64) []float32 {
	v := make([]float32, 384)
	v[0] = float32(cos)
	v[1] = float32(math.Sqrt(1 - cos*cos))
	return v
}

func embedded(id string, stock int32, price int64, cat, seller string, cos float64, tags []string, skus ...index.SkuDoc) index.ListingDoc {
	d := phone(id, 100, tags, skus...)
	d.Stock = i32(stock)
	d.Price = price
	d.CategoryID = cat
	d.SellerID = seller
	d.Embedding = unitAt(cos)
	return d
}

func vecIDs(t *testing.T, idx *index.OpenSearchIndex, f map[string]string, cat string, minP, maxP int64) []string {
	t.Helper()
	res, err := idx.SearchVector(context.Background(), make384(), f, cat, minP, maxP, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, h := range res.Hits {
		out = append(out, h.ListingID)
	}
	return out
}

// Against a real cluster: the filter narrows the k nearest neighbours themselves
// (k=2 still returns 2 matching listings although non-matching ones are closer),
// and knn scores are (1+cosine)/2.
func TestIT_Knn_FiltersConstrainNeighboursAndScoreScale(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, embedded("sold", 0, 100, "c1", "s1", 1.0, nil))
	upsert(t, idx, embedded("pricey", 5, 9000, "c2", "s2", 0.99, nil))
	upsert(t, idx, embedded("ok1", 5, 100, "c1", "s1", 0.5, []string{"connectivity:bluetooth-5-3"}, sku("v1", 1, "color:den")))
	upsert(t, idx, embedded("ok2", 5, 200, "c1", "s1", 0.0, nil))

	got := vecIDs(t, idx, map[string]string{"in_stock": "true"}, "", 0, 0)
	if len(got) != 2 || got[0] != "pricey" || got[1] != "ok1" {
		t.Errorf("in_stock: %v, want [pricey ok1] (sold-out excluded, k filled)", got)
	}
	if got := vecIDs(t, idx, map[string]string{"in_stock": "true"}, "c1", 0, 1000); len(got) != 2 || got[0] != "ok1" || got[1] != "ok2" {
		t.Errorf("category+price+stock: %v, want [ok1 ok2]", got)
	}
	if got := vecIDs(t, idx, map[string]string{"seller_id": "s2"}, "", 0, 0); len(got) != 1 || got[0] != "pricey" {
		t.Errorf("seller: %v", got)
	}
	if got := vecIDs(t, idx, map[string]string{"tag.connectivity": "bluetooth-5-3", "sku.color": "den"}, "", 0, 0); len(got) != 1 || got[0] != "ok1" {
		t.Errorf("tag+sku: %v", got)
	}

	res, _ := idx.SearchVector(context.Background(), make384(), nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	scores := map[string]float64{}
	for _, h := range res.Hits {
		scores[h.ListingID] = h.Score
	}
	for id, want := range map[string]float64{"sold": 1.0, "ok1": 0.75, "ok2": 0.5} {
		if math.Abs(scores[id]-want) > 0.01 {
			t.Errorf("score(%s)=%v, want (1+cos)/2=%v", id, scores[id], want)
		}
	}
}

func TestIT_FacetsForIDs(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, embedded("a", 5, 100, "c1", "s1", 1, nil))
	upsert(t, idx, embedded("b", 5, 100, "c2", "s1", 1, nil))
	upsert(t, idx, embedded("c", 5, 100, "c2", "s1", 1, nil))
	f, err := idx.FacetsForIDs(context.Background(), []string{"a", "b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, b := range f.Categories {
		got[b.Key] = b.Count
	}
	if got["c1"] != 1 || got["c2"] != 1 || len(got) != 2 {
		t.Errorf("categories over {a,b} = %v", got)
	}
}
