package index_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/buidangphuc/team-search/internal/index"
)

type listingStockReader interface {
	ListingStocks(ctx context.Context, ids []string) (map[string]int32, error)
}

func TestOpenSearchGetListingStockBatch(t *testing.T) {
	seed, osURL, name := readyIndex(t)
	upsert(t, seed, stockDoc("five", "a", 5, 100))
	upsert(t, seed, stockDoc("zero", "b", 0, 100))
	upsert(t, seed, liveDoc("legacy", "c", 100))
	upsert(t, seed, stockDoc("dead", "d", 7, 100))
	del(t, seed, "dead", 200)

	target, err := url.Parse(osURL)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
	}))
	defer proxy.Close()

	idx, err := index.New(proxy.URL, name)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := any(idx).(listingStockReader)
	if !ok {
		t.Fatal("OpenSearchIndex lacks ListingStocks(ctx, ids) (map[string]int32, error)")
	}
	before := calls.Load()
	got, err := r.ListingStocks(context.Background(), []string{"five", "zero", "legacy", "dead", "never", "five"})
	if err != nil {
		t.Fatal(err)
	}
	if n := calls.Load() - before; n != 1 {
		t.Errorf("%d HTTP round trips, want 1", n)
	}
	if v, ok := got["five"]; !ok || v != 5 {
		t.Errorf("five: %v %v", v, ok)
	}
	if v, ok := got["zero"]; !ok || v != 0 {
		t.Errorf("zero must be present as 0: %v %v", v, ok)
	}
	for _, id := range []string{"legacy", "dead", "never"} {
		if v, ok := got[id]; ok {
			t.Errorf("%s must be absent, got %d", id, v)
		}
	}
}
