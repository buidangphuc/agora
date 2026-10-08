package index_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/team-search/internal/index"
)

// Real-OpenSearch integration tests. They run only when OPENSEARCH_TEST_URL
// points at a disposable cluster (a throwaway container, never a shared stack);
// otherwise they skip, so `go test ./...` stays hermetic. Each test uses its own
// uniquely named index and deletes it afterwards.

var itCounter atomic.Int64

func itURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("OPENSEARCH_TEST_URL")
	if u == "" {
		t.Skip("OPENSEARCH_TEST_URL not set; skipping real-OpenSearch test")
	}
	return strings.TrimRight(u, "/")
}

// itIndex returns a handle on a fresh, not yet created index, with cleanup.
func itIndex(t *testing.T) (*index.OpenSearchIndex, string, string) {
	t.Helper()
	url := itURL(t)
	name := fmt.Sprintf("it-srm-%d-%d", time.Now().UnixNano(), itCounter.Add(1))
	idx, err := index.New(url, name)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, url+"/"+name, nil)
		if res, err := http.DefaultClient.Do(req); err == nil {
			res.Body.Close()
		}
	})
	return idx, url, name
}

// readyIndex is itIndex plus EnsureIndex.
func readyIndex(t *testing.T) (*index.OpenSearchIndex, string, string) {
	t.Helper()
	idx, url, name := itIndex(t)
	if err := idx.EnsureIndex(context.Background()); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	return idx, url, name
}

func osDo(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// mappedType returns the mapped type of a top-level field, "" when unmapped.
func mappedType(t *testing.T, url, name, field string) string {
	t.Helper()
	code, body := osDo(t, http.MethodGet, url+"/"+name+"/_mapping", "")
	if code != 200 {
		t.Fatalf("get mapping: %d %s", code, body)
	}
	var m map[string]struct {
		Mappings struct {
			Properties map[string]struct {
				Type string `json:"type"`
			} `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	return m[name].Mappings.Properties[field].Type
}

// getSource returns the stored _source of a document and whether it exists.
func getSource(t *testing.T, url, name, id string) (map[string]any, bool) {
	t.Helper()
	code, body := osDo(t, http.MethodGet, url+"/"+name+"/_doc/"+id, "")
	if code == 404 {
		return nil, false
	}
	if code != 200 {
		t.Fatalf("get doc %s: %d %s", id, code, body)
	}
	var out struct {
		Source map[string]any `json:"_source"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out.Source, true
}

// num reads an integer field from a decoded _source.
func num(m map[string]any, k string) (int64, bool) {
	v, ok := m[k].(float64)
	return int64(v), ok
}

func mustSource(t *testing.T, url, name, id string) map[string]any {
	t.Helper()
	src, ok := getSource(t, url, name, id)
	if !ok {
		t.Fatalf("doc %s missing", id)
	}
	return src
}

var srmFields = map[string]string{
	"stock":         "integer",
	"stock_version": "long",
	"created_at":    "date",
	"tombstoned_at": "date",
}

func TestIT_EnsureIndex_FreshCreateHasReadModelFields(t *testing.T) {
	_, url, name := readyIndex(t)
	for f, want := range srmFields {
		if got := mappedType(t, url, name, f); got != want {
			t.Errorf("%s type = %q, want %q", f, got, want)
		}
	}
}

func TestIT_EnsureIndex_ExistingIndexGainsFieldsAndSecondCallIsNoop(t *testing.T) {
	idx, url, name := itIndex(t)
	// An index created by the previous release: none of the four fields.
	if code, body := osDo(t, http.MethodPut, url+"/"+name,
		`{"mappings":{"properties":{"id":{"type":"keyword"},"version":{"type":"long"}}}}`); code != 200 {
		t.Fatalf("create old index: %d %s", code, body)
	}
	for f := range srmFields {
		if got := mappedType(t, url, name, f); got != "" {
			t.Fatalf("precondition: %s already mapped as %q", f, got)
		}
	}
	for i := 0; i < 2; i++ {
		if err := idx.EnsureIndex(context.Background()); err != nil {
			t.Fatalf("EnsureIndex #%d: %v", i+1, err)
		}
		for f, want := range srmFields {
			if got := mappedType(t, url, name, f); got != want {
				t.Errorf("after EnsureIndex #%d: %s type = %q, want %q", i+1, f, got, want)
			}
		}
	}
}

func TestIT_EnsureIndex_ConcurrentCallsBothSucceed(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%v", existing), func(t *testing.T) {
			url := itURL(t)
			a, _, name := itIndex(t)
			if existing {
				if code, body := osDo(t, http.MethodPut, url+"/"+name, `{}`); code != 200 {
					t.Fatalf("create old index: %d %s", code, body)
				}
			}
			b, err := index.New(url, name)
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			errs := make([]error, 2)
			for i, idx := range []*index.OpenSearchIndex{a, b} {
				wg.Add(1)
				go func(i int, idx *index.OpenSearchIndex) {
					defer wg.Done()
					errs[i] = idx.EnsureIndex(context.Background())
				}(i, idx)
			}
			wg.Wait()
			for i, err := range errs {
				if err != nil {
					t.Errorf("EnsureIndex #%d: %v", i+1, err)
				}
			}
			if got := mappedType(t, url, name, "stock"); got != "integer" {
				t.Errorf("stock type = %q", got)
			}
		})
	}
}

func TestIT_EnsureIndex_ConflictingMappingFailsLoudly(t *testing.T) {
	idx, url, name := itIndex(t)
	if code, body := osDo(t, http.MethodPut, url+"/"+name,
		`{"mappings":{"properties":{"stock":{"type":"keyword"}}}}`); code != 200 {
		t.Fatalf("create conflicting index: %d %s", code, body)
	}
	err := idx.EnsureIndex(context.Background())
	if err == nil || !strings.Contains(err.Error(), "stock") {
		t.Fatalf("expected EnsureIndex to fail naming the stock mapping, got %v", err)
	}
}
