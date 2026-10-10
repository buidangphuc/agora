package index_test

import (
	"context"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

func i64(v int64) *int64 { return &v }

func createdDoc(id, title string, v, createdAt int64) index.ListingDoc {
	d := liveDoc(id, title, v)
	d.CreatedAt = i64(createdAt)
	return d
}

func createdAtOf(t *testing.T, url, name, id string) (int64, bool) {
	t.Helper()
	return num(mustSource(t, url, name, id), "created_at")
}

func TestIT_CreatedAt_UpdatedDoesNotChangeIt(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, createdDoc("l1", "a", 100, 1000))
	upsert(t, idx, liveDoc("l1", "b", 200))
	if c, ok := createdAtOf(t, url, name, "l1"); !ok || c != 1000 {
		t.Errorf("created_at = %d (%v), want 1000 carried forward", c, ok)
	}
}

func TestIT_CreatedAt_LateCreatedAfterNewerUpdatedStillRecords(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, liveDoc("l1", "updated title", 200))
	upsert(t, idx, createdDoc("l1", "created title", 100, 1000))
	src := mustSource(t, url, name, "l1")
	if src["title"] != "updated title" {
		t.Errorf("stale CREATED base fields applied: %v", src)
	}
	if c, ok := num(src, "created_at"); !ok || c != 1000 {
		t.Errorf("created_at = %d (%v), want 1000", c, ok)
	}
	// A redelivered (or later-stamped) CREATED never moves it forward.
	upsert(t, idx, createdDoc("l1", "x", 100, 5000))
	if c, _ := createdAtOf(t, url, name, "l1"); c != 1000 {
		t.Errorf("created_at moved to %d", c)
	}
}

func TestIT_CreatedAt_NeverOnTombstone(t *testing.T) {
	idx, url, name := readyIndex(t)
	del(t, idx, "l1", 200)
	upsert(t, idx, createdDoc("l1", "x", 100, 1000))
	wantTombstone(t, url, name, "l1", 200)
}

func TestIT_SortNewest_OrdersByCreationTimeMissingLast(t *testing.T) {
	idx, _, _ := readyIndex(t)
	// ids chosen so _id order would differ from creation order.
	upsert(t, idx, createdDoc("a-old", "quokka a", 100, 1000))
	upsert(t, idx, createdDoc("b-new", "quokka b", 100, 3000))
	upsert(t, idx, liveDoc("0-none", "quokka c", 100))
	upsert(t, idx, liveDoc("z-none", "quokka d", 100))
	upsert(t, idx, createdDoc("c-mid", "quokka e", 100, 2000))
	for _, leg := range []string{"lexical", "vector"} {
		var (
			res index.SearchResult
			err error
		)
		if leg == "lexical" {
			res, err = idx.Search(context.Background(), "quokka", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_NEWEST, 0, 10)
		} else {
			// No embeddings are indexed; this only proves the k-NN body with the
			// sort is accepted by a real cluster.
			res, err = idx.SearchVector(context.Background(), make384(), nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_NEWEST, 0, 10)
		}
		if err != nil {
			t.Fatalf("%s: %v", leg, err)
		}
		if leg == "vector" {
			continue
		}
		got := make([]string, 0, len(res.Hits))
		for _, h := range res.Hits {
			got = append(got, h.ListingID)
		}
		want := []string{"b-new", "c-mid", "a-old", "0-none", "z-none"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	}
}

func make384() []float32 {
	v := make([]float32, 384)
	v[0] = 1
	return v
}
