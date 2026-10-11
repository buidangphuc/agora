package retrieval_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/buidangphuc/team-search/internal/retrieval"
)

func TestHTTPEmbedClient_EmbedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings": [[0.1, 0.2, 0.3]]}`))
	}))
	defer srv.Close()

	client := retrieval.NewHTTPEmbedClient(srv.URL, 1*time.Second)
	vec, err := client.Embed(context.Background(), "giày chạy bộ")
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vec))
	}
	if vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Errorf("unexpected vector values: %+v", vec)
	}
}

func TestHTTPEmbedClient_OpenAIFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"embedding": [0.5, 0.6]}]}`))
	}))
	defer srv.Close()

	client := retrieval.NewHTTPEmbedClient(srv.URL, 1*time.Second)
	vec, err := client.Embed(context.Background(), "áo thun")
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(vec) != 2 || vec[0] != 0.5 || vec[1] != 0.6 {
		t.Errorf("unexpected vector values: %+v", vec)
	}
}

func TestHTTPRerankClient_RerankSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rerank" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Item at index 1 scored highest (0.95), index 0 scored lower (0.2)
		_, _ = w.Write([]byte(`{"results": [{"index": 1, "score": 0.95}, {"index": 0, "score": 0.2}]}`))
	}))
	defer srv.Close()

	client := retrieval.NewHTTPRerankClient(srv.URL, 1*time.Second)
	reordered, err := client.Rerank(context.Background(), "laptop", []retrieval.RerankDoc{{ID: "id-1", Text: "a"}, {ID: "id-2", Text: "b"}})
	if err != nil {
		t.Fatalf("Rerank error: %v", err)
	}
	if len(reordered) != 2 {
		t.Fatalf("expected 2 reordered items, got %d", len(reordered))
	}
	if reordered[0] != "id-2" || reordered[1] != "id-1" {
		t.Errorf("expected id-2 then id-1, got %+v", reordered)
	}
}

// TEI and the modelserve router return a bare list, and the request must carry
// candidate text, not listing IDs.
func TestHTTPRerankClient_BareListAndTexts(t *testing.T) {
	var gotTexts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string   `json:"query"`
			Texts []string `json:"texts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotTexts = req.Texts
		_, _ = w.Write([]byte(`[{"index":2,"score":0.9},{"index":0,"score":0.5},{"index":1,"score":0.1}]`))
	}))
	defer srv.Close()

	client := retrieval.NewHTTPRerankClient(srv.URL, time.Second)
	got, err := client.Rerank(context.Background(), "laptop", []retrieval.RerankDoc{
		{ID: "id-a", Text: "Old phone\nscratched"},
		{ID: "id-b", Text: "Mouse\nwired"},
		{ID: "id-c", Text: "Gaming laptop\n16GB"},
	})
	if err != nil {
		t.Fatalf("Rerank error: %v", err)
	}
	if want := []string{"id-c", "id-a", "id-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if want := []string{"Old phone\nscratched", "Mouse\nwired", "Gaming laptop\n16GB"}; !reflect.DeepEqual(gotTexts, want) {
		t.Errorf("texts sent = %v, want candidate text %v", gotTexts, want)
	}
}
