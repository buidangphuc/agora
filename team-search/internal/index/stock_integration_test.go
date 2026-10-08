package index_test

import (
	"context"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

func i32(v int32) *int32 { return &v }

func stockOf(t *testing.T, url, name, id string) (stock, stockVersion int64) {
	t.Helper()
	src := mustSource(t, url, name, id)
	s, ok := num(src, "stock")
	if !ok {
		t.Fatalf("doc %s has no stock: %v", id, src)
	}
	sv, _ := num(src, "stock_version")
	return s, sv
}

func updStock(t *testing.T, idx *index.OpenSearchIndex, id string, stock int32, v int64) {
	t.Helper()
	if err := idx.UpdateStock(context.Background(), id, stock, v); err != nil {
		t.Fatalf("UpdateStock %s=%d@%d: %v", id, stock, v, err)
	}
}

func stockDoc(id, title string, stock int32, v int64) index.ListingDoc {
	d := liveDoc(id, title, v)
	d.Stock = i32(stock)
	return d
}

func TestIT_Stock_InOrderAndReversedEndAtNewest(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, stockDoc("in", "a", 10, 100))
	upsert(t, idx, stockDoc("rev", "b", 10, 100))
	updStock(t, idx, "in", 8, 200)
	updStock(t, idx, "in", 5, 300)
	updStock(t, idx, "rev", 5, 300)
	updStock(t, idx, "rev", 8, 200)
	updStock(t, idx, "rev", 8, 300) // equal version: noop
	for _, id := range []string{"in", "rev"} {
		if s, sv := stockOf(t, url, name, id); s != 5 || sv != 300 {
			t.Errorf("%s: stock=%d stock_version=%d, want 5@300", id, s, sv)
		}
	}
}

func TestIT_Stock_EventForMissingDocCreatesNothing(t *testing.T) {
	idx, url, name := readyIndex(t)
	updStock(t, idx, "ghost", 5, 100)
	if src, ok := getSource(t, url, name, "ghost"); ok {
		t.Fatalf("a stock event created a document: %v", src)
	}
}

func TestIT_Stock_EventOnTombstoneIsNoop(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, stockDoc("l1", "a", 10, 100))
	del(t, idx, "l1", 200)
	updStock(t, idx, "l1", 5, 900)
	wantTombstone(t, url, name, "l1", 200)
}

func TestIT_Stock_AppliedEvenIfOlderThanBaseVersion(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, stockDoc("l1", "a", 10, 100))
	if err := idx.PartialUpdate(context.Background(), "l1", map[string]any{"price": int64(5), "version": int64(900)}); err != nil {
		t.Fatal(err)
	}
	updStock(t, idx, "l1", 3, 500)
	src := mustSource(t, url, name, "l1")
	if s, _ := num(src, "stock"); s != 3 {
		t.Errorf("stock = %d, want 3", s)
	}
	if v, _ := num(src, "version"); v != 900 {
		t.Errorf("a stock event changed the base version: %d", v)
	}
}

func TestIT_Stock_OlderListingChangedKeepsNewerStockButAppliesTitle(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, stockDoc("l1", "orig", 10, 100))
	updStock(t, idx, "l1", 8, 300) // checkout
	upsert(t, idx, stockDoc("l1", "renamed", 10, 200))
	src := mustSource(t, url, name, "l1")
	if src["title"] != "renamed" {
		t.Errorf("title not applied: %v", src)
	}
	if s, sv := stockOf(t, url, name, "l1"); s != 8 || sv != 300 {
		t.Errorf("stock=%d@%d, want the newer 8@300 kept", s, sv)
	}
}

func TestIT_Stock_NewerListingChangedReplacesStock(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, stockDoc("l1", "orig", 10, 100))
	updStock(t, idx, "l1", 8, 300)
	upsert(t, idx, stockDoc("l1", "edit", 50, 400))
	if s, sv := stockOf(t, url, name, "l1"); s != 50 || sv != 400 {
		t.Errorf("stock=%d@%d, want 50@400", s, sv)
	}
}

func TestIT_Stock_StaleBaseUpsertStillAppliesNewerStock(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, stockDoc("l1", "orig", 10, 100))
	if err := idx.PartialUpdate(context.Background(), "l1", map[string]any{"price": int64(5), "version": int64(900)}); err != nil {
		t.Fatal(err)
	}
	upsert(t, idx, stockDoc("l1", "stale", 7, 500))
	src := mustSource(t, url, name, "l1")
	if src["title"] != "orig" {
		t.Errorf("stale base fields applied: %v", src)
	}
	if s, sv := stockOf(t, url, name, "l1"); s != 7 || sv != 500 {
		t.Errorf("stock=%d@%d, want 7@500 (stock is ordered independently)", s, sv)
	}
}

func TestIT_Stock_NewerUpsertOverTombstoneTakesStock(t *testing.T) {
	idx, url, name := readyIndex(t)
	del(t, idx, "l1", 200)
	upsert(t, idx, stockDoc("l1", "back", 4, 300))
	if s, sv := stockOf(t, url, name, "l1"); s != 4 || sv != 300 {
		t.Errorf("stock=%d@%d, want 4@300", s, sv)
	}
}

func TestIT_InStock_OnlyPositiveStockMatches(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, stockDoc("five", "wombat five", 5, 100))
	upsert(t, idx, stockDoc("zero", "wombat zero", 0, 100))
	upsert(t, idx, liveDoc("unknown", "wombat unknown", 100)) // no stock projected
	ctx := context.Background()
	res, err := idx.Search(ctx, "wombat", map[string]string{"in_stock": "true"}, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ListingID != "five" {
		t.Errorf("in_stock: total=%d hits=%+v, want only five", res.Total, res.Hits)
	}
	if len(res.Facets.Sellers) != 1 || res.Facets.Sellers[0].Count != 1 {
		t.Errorf("facets not restricted: %+v", res.Facets.Sellers)
	}
	all, err := idx.Search(ctx, "wombat", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 3 {
		t.Errorf("without in_stock stock must not affect matching, total=%d", all.Total)
	}
	if _, err := idx.SearchVector(ctx, make384(), map[string]string{"in_stock": "true"}, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10); err != nil {
		t.Errorf("k-NN leg with in_stock rejected by the cluster: %v", err)
	}
}
