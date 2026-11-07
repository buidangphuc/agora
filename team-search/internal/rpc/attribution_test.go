package rpc_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/rpc"
)

var requestIDShape = regexp.MustCompile(`^[0-9a-f]{32}$`)

type attrRerank struct{}

func (attrRerank) Rerank(_ context.Context, _ string, docs []retrieval.RerankDoc) ([]string, error) {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[len(docs)-1-i] = d.ID
	}
	return out, nil
}

type attrEmbedDown struct{}

func (attrEmbedDown) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embed down")
}

func (attrEmbedDown) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embed down")
}

type attrAllLegsDown struct {
	failingIndex
}

func (a attrAllLegsDown) SearchVector(context.Context, []float32, map[string]string, string, int64, int64, int32, searchv1.SortBy, int, int) (index.SearchResult, error) {
	return index.SearchResult{}, a.err
}

func attrEngine(embed retrieval.EmbedClient, rr retrieval.RerankClient, rerank bool) *retrieval.Engine {
	return retrieval.NewEngine(&mockIndex{}, embed, rr, config.Retrieval{
		EnableHybridSearch: true,
		EnableReranker:     rerank,
		HybridRRFK:         60,
		HybridFusionWindow: 200,
		LexicalWeight:      1,
		SemanticWeight:     1,
	})
}

func attrHandler(t *testing.T, eng *retrieval.Engine, opts ...rpc.Option) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(&mockIndex{}, eng, newFakeSaved(), discardLogger(), opts...)
	if err != nil {
		t.Fatalf("rpc.NewHandler: %v", err)
	}
	return h
}

func attrSearch(t *testing.T, h *rpc.Handler, req *searchv1.SearchListingsRequest) *searchv1.SearchListingsResponse {
	t.Helper()
	res, err := h.SearchListings(userCtx("buyer_1", "search:read"), req)
	if err != nil {
		t.Fatalf("SearchListings: %v", err)
	}
	return res
}

func TestSearchAttributionRequestIDShape(t *testing.T) {
	res := attrSearch(t, attrHandler(t, nil), &searchv1.SearchListingsRequest{Query: "laptop"})
	if !requestIDShape.MatchString(res.GetRequestId()) {
		t.Errorf("request_id = %q, want 32 lowercase hex", res.GetRequestId())
	}
}

func TestSearchAttributionPagingGetsNewRequestID(t *testing.T) {
	h := attrHandler(t, nil)
	first := attrSearch(t, h, &searchv1.SearchListingsRequest{Query: "laptop", Page: pageReq("", 1)})
	cursor := first.GetPage().GetNextCursor()
	if cursor == "" {
		t.Fatal("expected a next_cursor for page 1")
	}
	second := attrSearch(t, h, &searchv1.SearchListingsRequest{Query: "laptop", Page: pageReq(cursor, 1)})
	if !requestIDShape.MatchString(first.GetRequestId()) || !requestIDShape.MatchString(second.GetRequestId()) {
		t.Fatalf("request ids not well formed: %q %q", first.GetRequestId(), second.GetRequestId())
	}
	if first.GetRequestId() == second.GetRequestId() {
		t.Errorf("page 1 and page 2 share request_id %q", first.GetRequestId())
	}
}

func TestSearchAttributionRepeatedSearchGets1000DistinctRequestIDs(t *testing.T) {
	h := attrHandler(t, nil)
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := attrSearch(t, h, &searchv1.SearchListingsRequest{Query: "laptop"}).GetRequestId()
		if !requestIDShape.MatchString(id) {
			t.Fatalf("call %d: request_id = %q, want 32 lowercase hex", i, id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 1000 {
		t.Errorf("distinct request_id = %d of 1000", len(seen))
	}
}

func TestSearchAttributionRequestIDIsUUIDv4(t *testing.T) {
	id := attrSearch(t, attrHandler(t, nil), &searchv1.SearchListingsRequest{Query: "laptop"}).GetRequestId()
	if len(id) != 32 || id[12] != '4' || !regexp.MustCompile(`^[89ab]$`).MatchString(id[16:17]) {
		t.Errorf("request_id = %q is not a UUID v4 rendered as hex", id)
	}
}

func TestSearchAttributionAnonymousCallerGetsBothFields(t *testing.T) {
	h := attrHandler(t, nil)
	res, err := h.SearchListings(principalCtx("anonymous", "anonymous", "search:read"), &searchv1.SearchListingsRequest{Query: "laptop"})
	if err != nil {
		t.Fatalf("SearchListings: %v", err)
	}
	if !requestIDShape.MatchString(res.GetRequestId()) {
		t.Errorf("request_id = %q, want 32 lowercase hex", res.GetRequestId())
	}
	if res.GetModelVersion() != "lexical@v1" {
		t.Errorf("model_version = %q, want lexical@v1", res.GetModelVersion())
	}
}

func TestSearchAttributionFailedSearchReturnsNoResponse(t *testing.T) {
	idx := attrAllLegsDown{failingIndex{mockIndex: &mockIndex{}, err: errors.New("index unavailable")}}
	for name, eng := range map[string]*retrieval.Engine{
		"without engine": nil,
		"with engine":    retrieval.NewEngine(idx, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{EnableHybridSearch: true, HybridFusionWindow: 200}),
	} {
		t.Run(name, func(t *testing.T) {
			h, err := rpc.NewHandler(idx, eng, newFakeSaved(), discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			res, err := h.SearchListings(userCtx("buyer_1", "search:read"), &searchv1.SearchListingsRequest{Query: "laptop"})
			if err == nil {
				t.Fatal("expected an error status")
			}
			if status.Code(err) == codes.OK {
				t.Fatalf("code = OK with err %v", err)
			}
			if res != nil {
				t.Errorf("response = %v, want nil (no request_id)", res)
			}
		})
	}
}

func TestSearchAttributionModelVersionPerPath(t *testing.T) {
	hybrid := searchv1.SearchMode_SEARCH_MODE_HYBRID
	real := &retrieval.MockEmbedClient{Dim: 4}
	cases := []struct {
		name string
		eng  *retrieval.Engine
		opts []rpc.Option
		req  *searchv1.SearchListingsRequest
		want string
	}{
		{"The engine is not configured", nil, nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: hybrid}, "lexical@v1"},
		{"A hybrid search reports hybrid", attrEngine(real, nil, false), nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: hybrid}, "hybrid@v1"},
		{"The reranker is reported", attrEngine(real, attrRerank{}, true), nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: hybrid}, "hybrid+rerank@v1"},
		{"A fallback page never carries rerank", attrEngine(attrEmbedDown{}, attrRerank{}, true), nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: hybrid}, "lexical@v1"},
		{"A lexical request reports lexical", attrEngine(real, attrRerank{}, true), nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL}, "lexical@v1"},
		{"A semantic request reports semantic", attrEngine(real, nil, false), nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: searchv1.SearchMode_SEARCH_MODE_SEMANTIC}, "semantic@v1"},
		{"Semantic fallback reports lexical", attrEngine(attrEmbedDown{}, nil, false), nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: searchv1.SearchMode_SEARCH_MODE_SEMANTIC}, "lexical@v1"},
		{"A key sort in hybrid mode reports lexical", attrEngine(real, attrRerank{}, true), nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: hybrid, SortBy: searchv1.SortBy_SORT_BY_PRICE_ASC}, "lexical@v1"},
		{"Deep paging beyond the fusion window reports lexical", attrEngine(real, attrRerank{}, true), nil,
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: hybrid, Page: pageReq("200", 10)}, "lexical@v1"},
		{"An empty query reports lexical", attrEngine(real, attrRerank{}, true), nil,
			&searchv1.SearchListingsRequest{Query: "", SearchMode: hybrid}, "lexical@v1"},
		{"The unspecified mode resolves to the mode that ran", attrEngine(real, nil, false), nil,
			&searchv1.SearchListingsRequest{Query: "laptop"}, "hybrid@v1"},
		{"The ranker version comes from the environment", attrEngine(real, nil, false), []rpc.Option{rpc.WithRankerVersion("v2")},
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL}, "lexical@v2"},
		{"The ranker version applies to rerank", attrEngine(real, attrRerank{}, true), []rpc.Option{rpc.WithRankerVersion("v2")},
			&searchv1.SearchListingsRequest{Query: "laptop", SearchMode: hybrid}, "hybrid+rerank@v2"},
		{"The ranker version applies without an engine", nil, []rpc.Option{rpc.WithRankerVersion("v2")},
			&searchv1.SearchListingsRequest{Query: "laptop"}, "lexical@v2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := attrSearch(t, attrHandler(t, c.eng, c.opts...), c.req)
			if res.GetModelVersion() != c.want {
				t.Errorf("model_version = %q, want %q", res.GetModelVersion(), c.want)
			}
		})
	}
}

func TestSearchAttributionSemanticRequestReportsSemanticWithItsHits(t *testing.T) {
	h := attrHandler(t, attrEngine(&retrieval.MockEmbedClient{Dim: 4}, nil, false))
	res := attrSearch(t, h, &searchv1.SearchListingsRequest{Query: "laptop", SearchMode: searchv1.SearchMode_SEARCH_MODE_SEMANTIC})
	if res.GetModelVersion() != "semantic@v1" {
		t.Fatalf("model_version = %q, want semantic@v1", res.GetModelVersion())
	}
	if len(res.GetHits()) != 1 || res.GetHits()[0].GetListingId() != "vec_listing_1" {
		t.Errorf("hits = %v, want the semantic leg's hit", res.GetHits())
	}
}

func TestSearchAttributionSavedSearchRunReportsLexicalAndFreshRequestID(t *testing.T) {
	h := attrHandler(t, attrEngine(&retrieval.MockEmbedClient{Dim: 4}, attrRerank{}, true))
	ctx := userCtx("buyer_1", "search:read", "search:write")
	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "iPhone"})
	if err != nil {
		t.Fatalf("SaveSearch: %v", err)
	}
	id := saved.GetSavedSearch().GetId()
	first, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: id})
	if err != nil {
		t.Fatalf("RunSavedSearch: %v", err)
	}
	second, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: id})
	if err != nil {
		t.Fatalf("RunSavedSearch: %v", err)
	}
	for _, r := range []*searchv1.RunSavedSearchResponse{first, second} {
		if !requestIDShape.MatchString(r.GetRequestId()) {
			t.Errorf("request_id = %q, want 32 lowercase hex", r.GetRequestId())
		}
		if r.GetModelVersion() != "lexical@v1" {
			t.Errorf("model_version = %q, want lexical@v1", r.GetModelVersion())
		}
	}
	if first.GetRequestId() == second.GetRequestId() {
		t.Errorf("two runs share request_id %q", first.GetRequestId())
	}
}

func TestSearchAttributionSavedSearchRunUsesConfiguredRankerVersion(t *testing.T) {
	h := attrHandler(t, nil, rpc.WithRankerVersion("v2"))
	ctx := userCtx("buyer_1", "search:read", "search:write")
	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "iPhone"})
	if err != nil {
		t.Fatalf("SaveSearch: %v", err)
	}
	res, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()})
	if err != nil {
		t.Fatalf("RunSavedSearch: %v", err)
	}
	if res.GetModelVersion() != "lexical@v2" {
		t.Errorf("model_version = %q, want lexical@v2", res.GetModelVersion())
	}
}

func TestSearchAttributionSavedSearchOfAnotherUserReturnsNoRequestID(t *testing.T) {
	h := attrHandler(t, nil)
	owner := userCtx("buyer_1", "search:read", "search:write")
	saved, err := h.SaveSearch(owner, &searchv1.SaveSearchRequest{Query: "iPhone"})
	if err != nil {
		t.Fatalf("SaveSearch: %v", err)
	}
	res, err := h.RunSavedSearch(userCtx("buyer_2", "search:read", "search:write"), &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v (%v), want NotFound", status.Code(err), err)
	}
	if res != nil {
		t.Errorf("response = %v, want nil", res)
	}
}
