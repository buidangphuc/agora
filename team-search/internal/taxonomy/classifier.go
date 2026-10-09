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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

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

// Service-principal convention for internal gRPC calls (same wire shape as
// team-order's and team-notification's upstream clients): the caller sets x-principal-*
// itself because an indexer has no user request to forward. The scope is the least this
// service needs: ai.classify, which only service principals may use (team-ai refuses it
// for a user principal). team-gateway never routes ClassifyTags.
const (
	servicePrincipalID     = "service-team-search"
	servicePrincipalType   = "service"
	servicePrincipalScopes = "ai.classify"

	// callTimeout bounds one classification; a slow team-ai must not stall indexing.
	callTimeout = 2 * time.Second
)

// GRPCClassifier calls team-ai's AIService.ClassifyTags.
type GRPCClassifier struct {
	client  aiv1.AIServiceClient
	timeout time.Duration
}

// NewGRPCClassifier wraps an AIService client (tests inject one over a real listener).
func NewGRPCClassifier(client aiv1.AIServiceClient, timeout time.Duration) *GRPCClassifier {
	if timeout <= 0 {
		timeout = callTimeout
	}
	return &GRPCClassifier{client: client, timeout: timeout}
}

// DialGRPCClassifier connects lazily to team-ai's gRPC address (host:port).
func DialGRPCClassifier(addr string) (*GRPCClassifier, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial team-ai %q: %w", addr, err)
	}
	return NewGRPCClassifier(aiv1.NewAIServiceClient(conn), callTimeout), conn, nil
}

// Classify calls ClassifyTags with the service principal and a 2 s deadline.
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
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", servicePrincipalID,
		"x-principal-type", servicePrincipalType,
		"x-principal-scopes", servicePrincipalScopes,
	))
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
