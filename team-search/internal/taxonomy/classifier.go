// Package taxonomy is team-search's client for team-ai's tag classifier. The
// indexer calls it while projecting a listing event so the read-model carries
// canonical SPU tags and per-variant (SKU) attributes (add-tag-classifier-filter-
// enrichment). team-ai owns the taxonomy; team-search only stores what it returns
// and holds no taxonomy of its own (AGENTS.md rule 3: a call, never a shared DB).
package taxonomy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/buidangphuc/team-search/internal/index"
)

// Variant is the listing variant fields the classifier reads.
type Variant struct {
	ID      string
	Name    string
	SkuCode string
	Price   int64
	Stock   int32
}

// Listing is what is classified: the SPU text plus its variants (may be empty).
type Listing struct {
	Title       string
	Description string
	CategoryID  string
	Variants    []Variant
}

// Classification is ready to be stored on the document.
type Classification struct {
	FacetTags []string
	SKUs      []index.SkuDoc
}

// Classifier classifies one listing. An error means "could not classify" (the
// indexer then marks the document tags_pending); it is never a "no tags" answer.
type Classifier interface {
	Classify(ctx context.Context, l Listing) (Classification, error)
}

// HTTPClassifier calls team-ai's REST endpoints.
type HTTPClassifier struct {
	baseURL string
	client  *http.Client
}

// NewHTTPClassifier builds a classifier for team-ai's base URL (e.g. http://team-ai-svc:8000).
func NewHTTPClassifier(baseURL string, timeout time.Duration) *HTTPClassifier {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &HTTPClassifier{baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: timeout}}
}

type wireTag struct {
	Slug       string `json:"slug"`
	FacetGroup string `json:"facet_group"`
}

type wireSku struct {
	VariantFacets map[string]string `json:"variant_facets"`
}

type wireResponse struct {
	CanonicalTags    []wireTag `json:"canonical_tags"`
	SpuCanonicalTags []wireTag `json:"spu_canonical_tags"`
	SkuResults       []wireSku `json:"sku_results"`
}

// Classify uses the hierarchy endpoint when the listing has variants and the
// plain SPU endpoint otherwise.
func (c *HTTPClassifier) Classify(ctx context.Context, l Listing) (Classification, error) {
	title := strings.TrimSpace(l.Title)
	if len([]rune(title)) < 2 {
		return Classification{}, nil // team-ai requires a 2+ character title; nothing to classify.
	}
	var (
		path string
		body map[string]any
	)
	if len(l.Variants) > 0 {
		vs := make([]map[string]any, 0, len(l.Variants))
		for _, v := range l.Variants {
			name := strings.TrimSpace(v.Name)
			if name == "" {
				name = v.SkuCode
			}
			if name == "" {
				name = v.ID
			}
			vs = append(vs, map[string]any{
				"variant_id": v.ID, "name": name, "sku_code": v.SkuCode,
				"price": max(v.Price, 0), "stock": max(v.Stock, 0),
			})
		}
		path = "/api/v1/ai/tags/classify-sku-hierarchy"
		body = map[string]any{"spu_title": title, "spu_description": l.Description, "category_id": l.CategoryID, "variants": vs}
	} else {
		path = "/api/v1/ai/tags/classify"
		body = map[string]any{"title": title, "description": l.Description, "category_id": l.CategoryID, "top_k": 30, "include_candidates": false}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Classification{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return Classification{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return Classification{}, fmt.Errorf("tag classifier call: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Classification{}, fmt.Errorf("read tag classifier response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Classification{}, fmt.Errorf("tag classifier status %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	var parsed wireResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Classification{}, fmt.Errorf("decode tag classifier response: %w", err)
	}
	return build(l, parsed), nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// build maps the wire answer to document fields, dropping malformed entries.
func build(l Listing, r wireResponse) Classification {
	tags := r.SpuCanonicalTags
	if len(tags) == 0 {
		tags = r.CanonicalTags
	}
	seen := map[string]bool{}
	out := Classification{FacetTags: []string{}}
	for _, t := range tags {
		if !index.ValidAttr(t.FacetGroup, t.Slug) {
			continue
		}
		k := index.AttrKey(t.FacetGroup, t.Slug)
		if !seen[k] {
			seen[k] = true
			out.FacetTags = append(out.FacetTags, k)
		}
	}
	sort.Strings(out.FacetTags)

	if len(l.Variants) == 0 || len(r.SkuResults) != len(l.Variants) {
		return out
	}
	for i, v := range l.Variants {
		attrs := []string{}
		for g, s := range r.SkuResults[i].VariantFacets {
			if index.ValidAttr(g, s) {
				attrs = append(attrs, index.AttrKey(g, s))
			}
		}
		sort.Strings(attrs)
		out.SKUs = append(out.SKUs, index.SkuDoc{
			VariantID: v.ID, SkuCode: v.SkuCode, Name: v.Name, Price: v.Price,
			Stock: v.Stock, InStock: v.Stock > 0, Attrs: attrs,
		})
	}
	return out
}

// MockClassifier is a deterministic test double.
type MockClassifier struct {
	Result Classification
	Err    error
	Calls  []Listing
}

// Classify records the call and returns the canned answer.
func (m *MockClassifier) Classify(_ context.Context, l Listing) (Classification, error) {
	m.Calls = append(m.Calls, l)
	return m.Result, m.Err
}
