package index_test

import (
	"context"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

func liveDoc(id, title string, version int64) index.ListingDoc {
	return index.ListingDoc{ID: id, Title: title, Status: "published", SellerID: "s1", CategoryID: "c1", Price: 100, Version: version}
}

func upsert(t *testing.T, idx *index.OpenSearchIndex, doc index.ListingDoc) {
	t.Helper()
	if err := idx.Upsert(context.Background(), doc); err != nil {
		t.Fatalf("Upsert %s@%d: %v", doc.ID, doc.Version, err)
	}
}

func del(t *testing.T, idx *index.OpenSearchIndex, id string, version int64) {
	t.Helper()
	if err := idx.Delete(context.Background(), id, version); err != nil {
		t.Fatalf("Delete %s@%d: %v", id, version, err)
	}
}

func wantTombstone(t *testing.T, url, name, id string, version int64) {
	t.Helper()
	src := mustSource(t, url, name, id)
	if src["status"] != "deleted" {
		t.Fatalf("doc %s is not a tombstone: %v", id, src)
	}
	if v, _ := num(src, "version"); v != version {
		t.Errorf("tombstone version = %d, want %d", v, version)
	}
	if _, ok := num(src, "tombstoned_at"); !ok {
		t.Errorf("tombstone has no tombstoned_at: %v", src)
	}
	for _, k := range []string{"title", "seller_id", "price", "stock", "created_at"} {
		if _, has := src[k]; has {
			t.Errorf("tombstone kept field %q: %v", k, src)
		}
	}
}

func TestIT_Tombstone_DeleteWritesVersionedTombstone(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "phone", 100))
	del(t, idx, "l1", 200)
	wantTombstone(t, url, name, "l1", 200)

	// A delete for a document the read-model never held still leaves a tombstone.
	del(t, idx, "never", 300)
	wantTombstone(t, url, name, "never", 300)
}

func TestIT_Tombstone_StaleEqualOrUnversionedUpsertIsNoop(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "phone", 100))
	del(t, idx, "l1", 200)
	for _, v := range []int64{150, 200, 0} {
		upsert(t, idx, liveDoc("l1", "revived", v))
		wantTombstone(t, url, name, "l1", 200)
	}
}

func TestIT_Tombstone_NewerUpsertReplacesIt(t *testing.T) {
	idx, url, name := readyIndex(t)
	del(t, idx, "l1", 200)
	upsert(t, idx, liveDoc("l1", "reapproved", 300))
	src := mustSource(t, url, name, "l1")
	if src["status"] != "published" || src["title"] != "reapproved" {
		t.Fatalf("newer upsert did not replace the tombstone: %v", src)
	}
	if v, _ := num(src, "version"); v != 300 {
		t.Errorf("version = %d, want 300", v)
	}
	if _, has := src["tombstoned_at"]; has {
		t.Errorf("tombstoned_at survived the replacement: %v", src)
	}
}

func TestIT_Tombstone_PartialUpdateIsNoopAtAnyVersion(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "phone", 100))
	del(t, idx, "l1", 200)
	for _, v := range []int64{150, 200, 999, 0} {
		if err := idx.PartialUpdate(context.Background(), "l1", map[string]any{"status": "published", "title": "x", "version": v}); err != nil {
			t.Fatalf("PartialUpdate@%d: %v", v, err)
		}
		wantTombstone(t, url, name, "l1", 200)
	}
}

func TestIT_Tombstone_DeleteOverNewerTombstoneKeepsNewerVersion(t *testing.T) {
	idx, url, name := readyIndex(t)
	del(t, idx, "l1", 500)
	del(t, idx, "l1", 300)
	wantTombstone(t, url, name, "l1", 500)
	del(t, idx, "l1", 0)
	wantTombstone(t, url, name, "l1", 500)
	del(t, idx, "l1", 700)
	wantTombstone(t, url, name, "l1", 700)
}

func TestIT_Tombstone_StaleDeleteOnNewerLiveDocIsNoop(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "reapproved", 900))
	del(t, idx, "l1", 400)
	if src := mustSource(t, url, name, "l1"); src["status"] != "published" {
		t.Fatalf("a delete older than the live document removed it: %v", src)
	}
}

func TestIT_Upsert_PlainStaleUpsertOnLiveDocIsNoop(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "new", 200))
	upsert(t, idx, liveDoc("l1", "old", 100))
	upsert(t, idx, liveDoc("l1", "same", 200))
	if src := mustSource(t, url, name, "l1"); src["title"] != "new" {
		t.Fatalf("stale upsert overwrote the doc: %v", src)
	}
	upsert(t, idx, liveDoc("l1", "newer", 300))
	if src := mustSource(t, url, name, "l1"); src["title"] != "newer" {
		t.Fatalf("newer upsert not applied: %v", src)
	}
}

func TestIT_Tombstone_InvisibleToSearchAndSuggest(t *testing.T) {
	idx, _, _ := readyIndex(t)
	ctx := context.Background()
	upsert(t, idx, liveDoc("gone", "zebraphone gone", 100))
	upsert(t, idx, liveDoc("kept", "zebraphone kept", 100))
	del(t, idx, "gone", 200)
	res, err := idx.Search(ctx, "zebraphone", map[string]string{"seller_id": "s1"}, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ListingID != "kept" {
		t.Errorf("tombstone leaked into search: total=%d hits=%+v", res.Total, res.Hits)
	}
	if len(res.Facets.Sellers) != 1 || res.Facets.Sellers[0].Count != 1 {
		t.Errorf("tombstone counted in sellers facet: %+v", res.Facets.Sellers)
	}
	sugg, err := idx.Suggest(ctx, "zebra", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sugg) != 1 || sugg[0] != "zebraphone kept" {
		t.Errorf("tombstone leaked into suggest: %v", sugg)
	}
}
