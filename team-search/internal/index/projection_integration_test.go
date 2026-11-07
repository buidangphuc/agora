package index_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

func updSales(t *testing.T, idx *index.OpenSearchIndex, id string, sold int64, v int64) {
	t.Helper()
	if err := idx.UpdateSales(context.Background(), id, sold, v); err != nil {
		t.Fatalf("UpdateSales %s=%d@%d: %v", id, sold, v, err)
	}
}

func updRating(t *testing.T, idx *index.OpenSearchIndex, id string, avg float64, reviews int64, v int64) {
	t.Helper()
	if err := idx.UpdateRating(context.Background(), id, avg, reviews, v); err != nil {
		t.Fatalf("UpdateRating %s=%v/%d@%d: %v", id, avg, reviews, v, err)
	}
}

func soldOf(t *testing.T, url, name, id string) (sold, version int64) {
	t.Helper()
	src := mustSource(t, url, name, id)
	sold, _ = num(src, "sold_count")
	version, _ = num(src, "sales_version")
	return sold, version
}

func ratingOf(t *testing.T, url, name, id string) (avg float64, reviews, version int64) {
	t.Helper()
	src := mustSource(t, url, name, id)
	avg, _ = src["rating_avg"].(float64)
	reviews, _ = num(src, "review_count")
	version, _ = num(src, "rating_version")
	return avg, reviews, version
}

func search(t *testing.T, idx *index.OpenSearchIndex, q, categoryID string, minRating int32, sortBy searchv1.SortBy) index.SearchResult {
	t.Helper()
	res, err := idx.Search(context.Background(), q, nil, categoryID, 0, 0, minRating, sortBy, 0, 50)
	if err != nil {
		t.Fatalf("Search %q: %v", q, err)
	}
	return res
}

func hitIDs(res index.SearchResult) []string {
	out := make([]string, 0, len(res.Hits))
	for _, h := range res.Hits {
		out = append(out, h.ListingID)
	}
	return out
}

func wantIDs(t *testing.T, res index.SearchResult, want ...string) {
	t.Helper()
	got := hitIDs(res)
	if len(got) != len(want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hits = %v, want %v", got, want)
		}
	}
}

func wantBuckets(t *testing.T, label string, got []index.FacetBucket, want ...index.FacetBucket) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %+v, want %+v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %+v, want %+v", label, got, want)
		}
	}
}

func TestIT_BR3_BR4_SalesLaterValueReplacesOlderAndEqualIsNoop(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "alpaca", 100))
	updSales(t, idx, "l1", 5, 200)
	if s, v := soldOf(t, url, name, "l1"); s != 5 || v != 200 {
		t.Fatalf("sold=%d@%d, want 5@200", s, v)
	}
	updSales(t, idx, "l1", 9, 100)
	updSales(t, idx, "l1", 9, 200)
	updSales(t, idx, "l1", 5, 200)
	if s, v := soldOf(t, url, name, "l1"); s != 5 || v != 200 {
		t.Errorf("older or equal event changed it: sold=%d@%d, want 5@200", s, v)
	}
	updSales(t, idx, "l1", 3, 300)
	if s, v := soldOf(t, url, name, "l1"); s != 3 || v != 300 {
		t.Errorf("sold=%d@%d, want the later lower value 3@300", s, v)
	}
	updSales(t, idx, "l1", 0, 400)
	if s, _ := soldOf(t, url, name, "l1"); s != 0 {
		t.Errorf("sold=%d, want 0", s)
	}
}

func TestIT_BR2_BR4a_RatingGroupMovesTogetherAndEqualIsNoop(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "alpaca", 100))
	updRating(t, idx, "l1", 4.5, 2, 200)
	if a, r, v := ratingOf(t, url, name, "l1"); a != 4.5 || r != 2 || v != 200 {
		t.Fatalf("rating=%v/%d@%d, want 4.5/2@200", a, r, v)
	}
	updRating(t, idx, "l1", 1, 9, 200)
	updRating(t, idx, "l1", 1, 9, 150)
	if a, r, v := ratingOf(t, url, name, "l1"); a != 4.5 || r != 2 || v != 200 {
		t.Errorf("equal or older event changed it: %v/%d@%d", a, r, v)
	}
	updRating(t, idx, "l1", 3.25, 4, 300)
	if a, r, v := ratingOf(t, url, name, "l1"); a != 3.25 || r != 4 || v != 300 {
		t.Errorf("rating=%v/%d@%d, want 3.25/4@300", a, r, v)
	}
}

func TestIT_BR3_SalesAndRatingVersionsAreIndependent(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "alpaca", 100))
	updSales(t, idx, "l1", 5, 900)
	updRating(t, idx, "l1", 4, 1, 200)
	if a, r, _ := ratingOf(t, url, name, "l1"); a != 4 || r != 1 {
		t.Errorf("a newer sales version blocked the rating: %v/%d", a, r)
	}
	if s, _ := soldOf(t, url, name, "l1"); s != 5 {
		t.Errorf("sold=%d, want 5", s)
	}
}

func TestIT_BR5_PendingDocIsStoredWithoutStatusAndWithPendingAt(t *testing.T) {
	idx, url, name := readyIndex(t)
	before := time.Now().Add(-time.Minute).UnixMilli()
	updSales(t, idx, "ghost", 3, 100)
	updRating(t, idx, "ghost", 4, 1, 100)
	src := mustSource(t, url, name, "ghost")
	if _, has := src["status"]; has {
		t.Errorf("pending doc must carry no status: %v", src)
	}
	if s, v := soldOf(t, url, name, "ghost"); s != 3 || v != 100 {
		t.Errorf("sold=%d@%d", s, v)
	}
	if a, r, v := ratingOf(t, url, name, "ghost"); a != 4 || r != 1 || v != 100 {
		t.Errorf("rating=%v/%d@%d", a, r, v)
	}
	if p, ok := num(src, "pending_at"); !ok || p < before || p > time.Now().Add(time.Minute).UnixMilli() {
		t.Errorf("pending_at = %d (%v), want the indexer clock in epoch millis", p, ok)
	}
}

func TestIT_BR5_PendingDocIsInvisibleInResultsTotalsAndFacets(t *testing.T) {
	idx, _, _ := readyIndex(t)
	live := liveDoc("live", "dromedary live", 100)
	upsert(t, idx, live)
	updSales(t, idx, "ghost", 99, 100)
	updRating(t, idx, "ghost", 5, 10, 100)
	updSales(t, idx, "live", 1, 100)
	updRating(t, idx, "live", 4, 1, 100)
	for _, sortBy := range []searchv1.SortBy{searchv1.SortBy_SORT_BY_UNSPECIFIED, searchv1.SortBy_SORT_BY_BEST_SELLING} {
		for _, minRating := range []int32{0, 3} {
			res := search(t, idx, "", "", minRating, sortBy)
			if res.Total != 1 {
				t.Errorf("sort=%v min_rating=%d total=%d, want 1", sortBy, minRating, res.Total)
			}
			wantIDs(t, res, "live")
			if len(res.Facets.Categories) != 1 || res.Facets.Categories[0].Count != 1 {
				t.Errorf("pending doc counted in categories: %+v", res.Facets.Categories)
			}
			if len(res.Facets.Sellers) != 1 || res.Facets.Sellers[0].Count != 1 {
				t.Errorf("pending doc counted in sellers: %+v", res.Facets.Sellers)
			}
			wantBuckets(t, "ratings", res.Facets.Ratings, index.FacetBucket{Key: "4", Count: 1}, index.FacetBucket{Key: "3", Count: 1})
		}
	}
	if _, err := idx.Suggest(context.Background(), "dromedary", 10); err != nil {
		t.Fatal(err)
	}
}

func TestIT_BR5_StockEventStillCreatesNoDocumentNextToPendingValues(t *testing.T) {
	idx, url, name := readyIndex(t)
	updSales(t, idx, "ghost", 3, 100)
	updStock(t, idx, "other", 5, 100)
	if _, ok := getSource(t, url, name, "other"); ok {
		t.Error("a stock event created a document")
	}
}

func TestIT_BR5_MergeOnUpsertKeepsPendingValuesAndClearsPendingAt(t *testing.T) {
	idx, url, name := readyIndex(t)
	updSales(t, idx, "l1", 4, 100)
	updRating(t, idx, "l1", 4, 1, 100)
	upsert(t, idx, liveDoc("l1", "lemming", 50))
	src := mustSource(t, url, name, "l1")
	if src["status"] != "published" || src["title"] != "lemming" {
		t.Fatalf("listing fields not applied: %v", src)
	}
	if s, v := soldOf(t, url, name, "l1"); s != 4 || v != 100 {
		t.Errorf("sold=%d@%d, want 4@100 merged", s, v)
	}
	if a, r, v := ratingOf(t, url, name, "l1"); a != 4 || r != 1 || v != 100 {
		t.Errorf("rating=%v/%d@%d, want 4/1@100 merged", a, r, v)
	}
	if p, has := src["pending_at"]; has && p != nil {
		t.Errorf("pending_at not cleared by the listing upsert: %v", p)
	}
	res := search(t, idx, "lemming", "", 0, searchv1.SortBy_SORT_BY_UNSPECIFIED)
	wantIDs(t, res, "l1")
	h := res.Hits[0]
	if h.SoldCount != 4 || h.RatingAvg != 4 || h.ReviewCount != 1 {
		t.Errorf("hit = sold %d rating %v reviews %d, want 4/4/1", h.SoldCount, h.RatingAvg, h.ReviewCount)
	}
}

func TestIT_BR6_ListingUpdateKeepsProjectedValues(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "orig", 100))
	updSales(t, idx, "l1", 5, 200)
	updRating(t, idx, "l1", 4.5, 2, 200)
	upsert(t, idx, liveDoc("l1", "renamed", 300))
	src := mustSource(t, url, name, "l1")
	if src["title"] != "renamed" {
		t.Fatalf("title not applied: %v", src)
	}
	if s, v := soldOf(t, url, name, "l1"); s != 5 || v != 200 {
		t.Errorf("sold=%d@%d, want 5@200", s, v)
	}
	if a, r, v := ratingOf(t, url, name, "l1"); a != 4.5 || r != 2 || v != 200 {
		t.Errorf("rating=%v/%d@%d, want 4.5/2@200", a, r, v)
	}
	upsert(t, idx, liveDoc("l1", "stale", 150))
	if s, _ := soldOf(t, url, name, "l1"); s != 5 {
		t.Errorf("a stale upsert reset sold to %d", s)
	}
}

func TestIT_BR6_SalesAndRatingAreAppliedEvenIfOlderThanBaseVersion(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "orig", 900))
	updSales(t, idx, "l1", 2, 100)
	updRating(t, idx, "l1", 3, 1, 100)
	if s, _ := soldOf(t, url, name, "l1"); s != 2 {
		t.Errorf("sold=%d, want 2", s)
	}
	if v, _ := num(mustSource(t, url, name, "l1"), "version"); v != 900 {
		t.Errorf("a projection event changed the base version: %d", v)
	}
}

func TestIT_BR8_EventNotLaterThanTombstoneIsIgnored(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "gone", 100))
	del(t, idx, "l1", 200)
	updSales(t, idx, "l1", 2, 100)
	updSales(t, idx, "l1", 2, 200)
	updRating(t, idx, "l1", 4, 1, 100)
	updRating(t, idx, "l1", 4, 1, 200)
	wantTombstone(t, url, name, "l1", 200)
	src := mustSource(t, url, name, "l1")
	for _, k := range []string{"sold_count", "rating_avg", "review_count", "pending_at"} {
		if v, has := src[k]; has && v != nil {
			t.Errorf("ignored event left %s=%v on the tombstone", k, v)
		}
	}
	res := search(t, idx, "", "", 0, searchv1.SortBy_SORT_BY_BEST_SELLING)
	if res.Total != 0 || len(res.Hits) != 0 {
		t.Errorf("tombstone visible: %+v", res)
	}
	upsert(t, idx, liveDoc("l1", "back", 300))
	if s, _ := soldOf(t, url, name, "l1"); s != 0 {
		t.Errorf("sold=%d after recreate, want 0", s)
	}
	if _, r, _ := ratingOf(t, url, name, "l1"); r != 0 {
		t.Errorf("review_count=%d after recreate, want 0", r)
	}
}

func TestIT_BR8a_RecreateAfterDeleteMergesPendingValues(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "gone", 100))
	del(t, idx, "l1", 200)
	updSales(t, idx, "l1", 6, 300)
	updRating(t, idx, "l1", 5, 1, 300)
	src := mustSource(t, url, name, "l1")
	if src["status"] != "deleted" {
		t.Fatalf("pending values on a tombstone must keep status deleted: %v", src)
	}
	if _, ok := num(src, "tombstoned_at"); !ok {
		t.Errorf("tombstoned_at lost: %v", src)
	}
	if p, ok := num(src, "pending_at"); !ok || p == 0 {
		t.Errorf("pending write on a tombstone must set pending_at: %v", src)
	}
	if res := search(t, idx, "", "", 0, searchv1.SortBy_SORT_BY_BEST_SELLING); res.Total != 0 {
		t.Errorf("tombstone with pending values is visible: %+v", res)
	}
	upsert(t, idx, liveDoc("l1", "reborn", 400))
	src = mustSource(t, url, name, "l1")
	if src["status"] != "published" || src["title"] != "reborn" {
		t.Fatalf("recreate not applied: %v", src)
	}
	if s, v := soldOf(t, url, name, "l1"); s != 6 || v != 300 {
		t.Errorf("sold=%d@%d, want 6@300", s, v)
	}
	if a, r, _ := ratingOf(t, url, name, "l1"); a != 5 || r != 1 {
		t.Errorf("rating=%v/%d, want 5/1", a, r)
	}
	if _, has := src["tombstoned_at"]; has {
		t.Errorf("tombstoned_at survived: %v", src)
	}
}

func TestIT_BR8a_StaleUpsertOverTombstoneKeepsPendingValuesUnmerged(t *testing.T) {
	idx, url, name := readyIndex(t)
	del(t, idx, "l1", 200)
	updSales(t, idx, "l1", 6, 300)
	upsert(t, idx, liveDoc("l1", "stale", 150))
	if src := mustSource(t, url, name, "l1"); src["status"] != "deleted" {
		t.Fatalf("a stale upsert revived the tombstone: %v", src)
	}
	wantTombstone(t, url, name, "l1", 200)
}

func sortedFixture(t *testing.T, idx *index.OpenSearchIndex) {
	t.Helper()
	upsert(t, idx, liveDoc("a", "narwhal a", 100))
	upsert(t, idx, liveDoc("b", "narwhal b", 100))
	upsert(t, idx, liveDoc("c", "narwhal c", 100))
	updSales(t, idx, "a", 7, 100)
	updSales(t, idx, "c", 3, 100)
}

func TestIT_BR9_BestSellingOrdersBySoldCountMissingCountsAsZero(t *testing.T) {
	idx, _, _ := readyIndex(t)
	sortedFixture(t, idx)
	res := search(t, idx, "narwhal", "", 0, searchv1.SortBy_SORT_BY_BEST_SELLING)
	wantIDs(t, res, "a", "c", "b")
	if res.Total != 3 {
		t.Errorf("total = %d, want 3", res.Total)
	}
	if _, err := idx.SearchVector(context.Background(), make384(), nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_BEST_SELLING, 0, 10); err != nil {
		t.Errorf("k-NN leg with best-selling rejected by the cluster: %v", err)
	}
}

func TestIT_BR9_BestSellingPaginates(t *testing.T) {
	idx, _, _ := readyIndex(t)
	sortedFixture(t, idx)
	res, err := idx.Search(context.Background(), "narwhal", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_BEST_SELLING, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, res, "c")
}

func TestIT_BR10_BestSellingTiesBreakByCreatedAtThenId(t *testing.T) {
	idx, _, _ := readyIndex(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	upsert(t, idx, createdDoc("d-id", "okapi d", 100, base))
	upsert(t, idx, createdDoc("f-id", "okapi f", 100, base+5*60*1000))
	upsert(t, idx, createdDoc("e-id", "okapi e", 100, base+5*60*1000))
	for _, id := range []string{"d-id", "e-id", "f-id"} {
		updSales(t, idx, id, 2, 100)
	}
	wantIDs(t, search(t, idx, "okapi", "", 0, searchv1.SortBy_SORT_BY_BEST_SELLING), "e-id", "f-id", "d-id")
}

func TestIT_BR10_BestSellingMissingCreatedAtSortsLastWithinATie(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, liveDoc("a-none", "tapir a", 100))
	upsert(t, idx, createdDoc("z-has", "tapir z", 100, 1000))
	updSales(t, idx, "a-none", 2, 100)
	updSales(t, idx, "z-has", 2, 100)
	wantIDs(t, search(t, idx, "tapir", "", 0, searchv1.SortBy_SORT_BY_BEST_SELLING), "z-has", "a-none")
}

func ratingFixture(t *testing.T) *index.OpenSearchIndex {
	t.Helper()
	idx, _, _ := readyIndex(t)
	hi := liveDoc("hi", "quokka hi", 100)
	hi.CategoryID = "c1"
	mid := liveDoc("mid", "quokka mid", 100)
	mid.CategoryID = "c2"
	lo := liveDoc("lo", "quokka lo", 100)
	lo.CategoryID = "c2"
	none := liveDoc("none", "quokka none", 100)
	none.CategoryID = "c2"
	for _, d := range []index.ListingDoc{hi, mid, lo, none} {
		upsert(t, idx, d)
	}
	updRating(t, idx, "hi", 4.5, 2, 100)
	updRating(t, idx, "mid", 3.5, 3, 100)
	updRating(t, idx, "lo", 2, 1, 100)
	return idx
}

func TestIT_BR13_BR15_MinRatingKeepsOnlyListingsAtOrAboveIt(t *testing.T) {
	idx := ratingFixture(t)
	sortBy := searchv1.SortBy_SORT_BY_BEST_SELLING
	for _, tc := range []struct {
		min  int32
		want []string
	}{
		{4, []string{"hi"}},
		{3, []string{"hi", "mid"}},
		{5, nil},
		{1, []string{"hi", "lo", "mid"}},
		{0, []string{"hi", "lo", "mid", "none"}},
	} {
		res := search(t, idx, "quokka", "", tc.min, sortBy)
		if res.Total != int64(len(tc.want)) {
			t.Errorf("min_rating %d: total=%d, want %d", tc.min, res.Total, len(tc.want))
		}
		got := map[string]bool{}
		for _, id := range hitIDs(res) {
			got[id] = true
		}
		if len(got) != len(tc.want) {
			t.Errorf("min_rating %d: hits %v, want %v", tc.min, hitIDs(res), tc.want)
		}
		for _, id := range tc.want {
			if !got[id] {
				t.Errorf("min_rating %d: %s missing from %v", tc.min, id, hitIDs(res))
			}
		}
	}
	if _, err := idx.SearchVector(context.Background(), make384(), nil, "", 0, 0, 4, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10); err != nil {
		t.Errorf("k-NN leg with min_rating rejected by the cluster: %v", err)
	}
}

func TestIT_BR14_RatingExactlyAtTheBoundaryIsIncluded(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, liveDoc("four", "capybara four", 100))
	updRating(t, idx, "four", 4, 1, 100)
	wantIDs(t, search(t, idx, "capybara", "", 4, searchv1.SortBy_SORT_BY_UNSPECIFIED), "four")
}

func TestIT_BR15_UnreviewedListingIsExcludedByAnyFloorAndNotCounted(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, liveDoc("plain", "wombat plain", 100))
	upsert(t, idx, liveDoc("zero", "wombat zero", 100))
	updRating(t, idx, "zero", 0, 0, 100)
	for _, min := range []int32{1, 3} {
		res := search(t, idx, "wombat", "", min, searchv1.SortBy_SORT_BY_UNSPECIFIED)
		if res.Total != 0 || len(res.Hits) != 0 {
			t.Errorf("min_rating %d: unreviewed listings returned: total=%d hits=%v", min, res.Total, hitIDs(res))
		}
	}
	if res := search(t, idx, "wombat", "", 0, searchv1.SortBy_SORT_BY_UNSPECIFIED); res.Total != 2 {
		t.Errorf("min_rating 0 must apply no filter, total=%d", res.Total)
	}
}

func TestIT_BR18_RatingsFacetCountsCumulativeFloors(t *testing.T) {
	idx := ratingFixture(t)
	res := search(t, idx, "quokka", "", 0, searchv1.SortBy_SORT_BY_UNSPECIFIED)
	wantBuckets(t, "ratings", res.Facets.Ratings, index.FacetBucket{Key: "4", Count: 1}, index.FacetBucket{Key: "3", Count: 2})
}

func TestIT_BR18a_RatingsFacetIgnoresItsOwnFilterOthersStillApply(t *testing.T) {
	idx := ratingFixture(t)
	res := search(t, idx, "quokka", "", 4, searchv1.SortBy_SORT_BY_UNSPECIFIED)
	wantIDs(t, res, "hi")
	wantBuckets(t, "ratings", res.Facets.Ratings, index.FacetBucket{Key: "4", Count: 1}, index.FacetBucket{Key: "3", Count: 2})
	wantBuckets(t, "categories", res.Facets.Categories, index.FacetBucket{Key: "c1", Count: 1})

	res = search(t, idx, "quokka", "c2", 4, searchv1.SortBy_SORT_BY_UNSPECIFIED)
	if res.Total != 0 {
		t.Errorf("category c2 with min_rating 4: total=%d, want 0", res.Total)
	}
	wantBuckets(t, "ratings under another filter", res.Facets.Ratings, index.FacetBucket{Key: "4", Count: 0}, index.FacetBucket{Key: "3", Count: 1})
}

func TestIT_BR19_BR20_HitCarriesProjectedValuesAndZeros(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, liveDoc("rated", "ocelot rated", 100))
	upsert(t, idx, liveDoc("plain", "ocelot plain", 100))
	updSales(t, idx, "rated", 5, 100)
	updRating(t, idx, "rated", 4.5, 2, 100)
	res := search(t, idx, "ocelot", "", 0, searchv1.SortBy_SORT_BY_BEST_SELLING)
	wantIDs(t, res, "rated", "plain")
	if h := res.Hits[0]; h.SoldCount != 5 || h.RatingAvg != 4.5 || h.ReviewCount != 2 {
		t.Errorf("rated hit = %d/%v/%d, want 5/4.5/2", h.SoldCount, h.RatingAvg, h.ReviewCount)
	}
	if h := res.Hits[1]; h.SoldCount != 0 || h.RatingAvg != 0 || h.ReviewCount != 0 {
		t.Errorf("untouched hit = %d/%v/%d, want zeros", h.SoldCount, h.RatingAvg, h.ReviewCount)
	}
}

func seedRaw(t *testing.T, url, name, id, body string) {
	t.Helper()
	if code, b := osDo(t, http.MethodPut, url+"/"+name+"/_doc/"+id+"?refresh=true", body); code >= 300 {
		t.Fatalf("seed %s: %d %s", id, code, b)
	}
}

func TestIT_Purge_ExpiredPendingDocsAreRemovedFreshOnesAndLiveKept(t *testing.T) {
	idx, url, name := readyIndex(t)
	now := time.Now()
	old := now.Add(-2 * time.Hour).UnixMilli()
	seedRaw(t, url, name, "expired", `{"id":"expired","sold_count":3,"sales_version":100,"pending_at":`+strconv.FormatInt(old, 10)+`}`)
	updSales(t, idx, "fresh", 2, 100)
	upsert(t, idx, liveDoc("live", "kept", 100))
	updSales(t, idx, "live", 9, 100)
	seedRaw(t, url, name, "odd-live", `{"id":"odd-live","status":"published","version":1,"sold_count":4,"pending_at":`+strconv.FormatInt(old, 10)+`}`)

	if _, err := idx.PurgeTombstones(context.Background(), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok := getSource(t, url, name, "expired"); ok {
		t.Error("expired pending doc survived")
	}
	for _, id := range []string{"fresh", "live", "odd-live"} {
		if _, ok := getSource(t, url, name, id); !ok {
			t.Errorf("purge removed %s", id)
		}
	}
	if s, _ := soldOf(t, url, name, "live"); s != 9 {
		t.Errorf("live sold_count = %d, want 9", s)
	}
}

func TestIT_Purge_KeepsLiveDocumentsWithProjectedValuesEvenWhenEverythingIsOld(t *testing.T) {
	idx, url, name := readyIndex(t)
	updSales(t, idx, "l1", 5, 100)
	updRating(t, idx, "l1", 4, 1, 100)
	upsert(t, idx, liveDoc("l1", "durian", 50))
	updSales(t, idx, "pend", 1, 100)
	n, err := idx.PurgeTombstones(context.Background(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Errorf("purged %d, want the pending doc removed", n)
	}
	if _, ok := getSource(t, url, name, "pend"); ok {
		t.Error("pending doc survived a cutoff in its future")
	}
	if s, _ := soldOf(t, url, name, "l1"); s != 5 {
		t.Errorf("live sold_count = %d, want 5", s)
	}
	res := search(t, idx, "durian", "", 0, searchv1.SortBy_SORT_BY_UNSPECIFIED)
	wantIDs(t, res, "l1")
	if res.Hits[0].SoldCount != 5 {
		t.Errorf("hit sold_count = %d, want 5", res.Hits[0].SoldCount)
	}
}

func TestIT_Purge_PendingValuesOnAnExpiredTombstoneGoWithIt(t *testing.T) {
	idx, url, name := readyIndex(t)
	del(t, idx, "l1", 200)
	updSales(t, idx, "l1", 6, 300)
	if _, err := idx.PurgeTombstones(context.Background(), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if s, _ := soldOf(t, url, name, "l1"); s != 6 {
		t.Fatalf("a tombstone inside the TTL lost its pending values: sold=%d", s)
	}
	if _, err := idx.PurgeTombstones(context.Background(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok := getSource(t, url, name, "l1"); ok {
		t.Fatal("expired tombstone survived")
	}
	upsert(t, idx, liveDoc("l1", "reborn", 400))
	if s, _ := soldOf(t, url, name, "l1"); s != 0 {
		t.Errorf("sold=%d after recreate, want 0 (pending values purged with the tombstone)", s)
	}
}
