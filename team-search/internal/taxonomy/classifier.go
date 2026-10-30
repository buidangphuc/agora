// Package taxonomy is team-search's client for team-ai's tag classifier over
// gRPC (AIService.ClassifyTags). The
// indexer calls it while projecting a listing event so the read-model carries
// canonical SPU tags and per-variant (SKU) attributes (add-tag-classifier-filter-
// enrichment). team-ai owns the taxonomy; team-search only stores what it returns
// and holds no taxonomy of its own (AGENTS.md rule 3: a call, never a shared DB).
package taxonomy

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/grpc"

	aiv1 "github.com/buidangphuc/team-search/generated/platform/ai/v1"
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

// GRPCClassifier calls team-ai's AIService.ClassifyTags over a connection built
// with upstream.NewClient (service principal ai.classify and the call deadline are
// the connection's options).
type GRPCClassifier struct {
	client aiv1.AIServiceClient
}

// NewGRPCClassifier wraps a connection to team-ai.
func NewGRPCClassifier(conn grpc.ClientConnInterface) *GRPCClassifier {
	return &GRPCClassifier{client: aiv1.NewAIServiceClient(conn)}
}

// Classify calls ClassifyTags.
func (c *GRPCClassifier) Classify(ctx context.Context, l Listing) (Classification, error) {
	title := strings.TrimSpace(l.Title)
	if len([]rune(title)) < 2 {
		return Classification{}, nil // team-ai requires a 2+ character title; nothing to classify.
	}
	req := &aiv1.ClassifyTagsRequest{Title: title, Description: l.Description, CategoryId: l.CategoryID}
	for _, v := range l.Variants {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			name = v.SkuCode
		}
		req.Variants = append(req.Variants, &aiv1.ClassifyVariant{
			VariantId: v.ID, Name: name, SkuCode: v.SkuCode, Price: max(v.Price, 0), Stock: max(v.Stock, 0),
		})
	}
	res, err := c.client.ClassifyTags(ctx, req)
	if err != nil {
		return Classification{}, fmt.Errorf("team-ai ClassifyTags: %w", err)
	}
	return build(l, res), nil
}

// build maps the response to document fields, dropping malformed entries.
func build(l Listing, r *aiv1.ClassifyTagsResponse) Classification {
	seen := map[string]bool{}
	out := Classification{FacetTags: []string{}}
	for _, t := range r.GetTags() {
		if !index.ValidAttr(t.GetFacetGroup(), t.GetSlug()) {
			continue
		}
		k := index.AttrKey(t.GetFacetGroup(), t.GetSlug())
		if !seen[k] {
			seen[k] = true
			out.FacetTags = append(out.FacetTags, k)
		}
	}
	sort.Strings(out.FacetTags)

	if len(l.Variants) == 0 || len(r.GetSkus()) != len(l.Variants) {
		return out
	}
	for i, v := range l.Variants {
		attrs := []string{}
		for _, t := range r.GetSkus()[i].GetTags() {
			if index.ValidAttr(t.GetFacetGroup(), t.GetSlug()) {
				attrs = append(attrs, index.AttrKey(t.GetFacetGroup(), t.GetSlug()))
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
