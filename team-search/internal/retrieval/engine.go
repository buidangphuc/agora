package retrieval

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
)

// SearchParams contains all search query criteria and mode instructions.
type SearchParams struct {
	Query      string
	Filters    map[string]string
	CategoryID string
	MinPrice   int64
	MaxPrice   int64
	MinRating  int32
	SortBy     searchv1.SortBy
	SearchMode searchv1.SearchMode
	From       int
	Size       int
}

// Engine coordinates multi-strategy retrieval, fail-open resiliency, and rank fusion.
type Engine struct {
	idx          index.Index
	embedClient  EmbedClient
	rerankClient RerankClient
	cfg          config.Retrieval
}

// NewEngine creates a new retrieval engine instance.
func NewEngine(idx index.Index, embedClient EmbedClient, rerankClient RerankClient, cfg config.Retrieval) *Engine {
	if cfg.HybridRRFK <= 0 {
		cfg.HybridRRFK = 60
	}
	if cfg.HybridFusionWindow <= 0 {
		cfg.HybridFusionWindow = 200
	}
	if cfg.LexicalWeight <= 0 {
		cfg.LexicalWeight = 1.0
	}
	if cfg.SemanticWeight <= 0 {
		cfg.SemanticWeight = 1.0
	}
	return &Engine{
		idx:          idx,
		embedClient:  embedClient,
		rerankClient: rerankClient,
		cfg:          cfg,
	}
}

// Execute runs the appropriate retrieval pipeline based on SearchMode and parameters.
func (e *Engine) Execute(ctx context.Context, params SearchParams) (index.SearchResult, bool, error) {
	// Determine effective search mode
	mode := params.SearchMode
	if mode == searchv1.SearchMode_SEARCH_MODE_UNSPECIFIED {
		if e.cfg.EnableHybridSearch && e.embedClient != nil && strings.TrimSpace(params.Query) != "" {
			mode = searchv1.SearchMode_SEARCH_MODE_HYBRID
		} else {
			mode = searchv1.SearchMode_SEARCH_MODE_LEXICAL
		}
	}

	// 1. Pure Lexical Mode
	if mode == searchv1.SearchMode_SEARCH_MODE_LEXICAL || strings.TrimSpace(params.Query) == "" {
		res, err := e.idx.Search(ctx, params.Query, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, params.From, params.Size)
		return res, false, err
	}

	// 2. Pure Semantic Mode
	if mode == searchv1.SearchMode_SEARCH_MODE_SEMANTIC {
		if e.embedClient == nil {
			// Fail open to lexical
			res, err := e.idx.Search(ctx, params.Query, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, params.From, params.Size)
			return res, true, err
		}
		vec, err := e.embedClient.Embed(ctx, params.Query)
		if err != nil {
			// Fail open to lexical
			log.Printf("[retrieval] semantic embed error: %v, falling back to lexical", err)
			res, fallbackErr := e.idx.Search(ctx, params.Query, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, params.From, params.Size)
			return res, true, fallbackErr
		}
		res, err := e.idx.SearchVector(ctx, vec, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, params.From, params.Size)
		if err != nil {
			log.Printf("[retrieval] search vector error: %v, falling back to lexical", err)
			res, fallbackErr := e.idx.Search(ctx, params.Query, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, params.From, params.Size)
			return res, true, fallbackErr
		}
		kept, dropped := e.applySemanticFloor(res, params.From)
		if dropped {
			kept.Facets = e.facetsFor(ctx, hitIDs(kept.Hits), params, kept.Facets)
		}
		return kept, false, nil
	}

	// 3. Hybrid Mode (Multi-Strategy Fusion)
	// D8: RRF ranks by relevance and would destroy a key order, so an explicit
	// newest/price sort is served by the lexical leg with that sort and page.
	if isKeySort(params.SortBy) {
		res, err := e.idx.Search(ctx, params.Query, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, params.From, params.Size)
		return res, false, err
	}

	// D8: Deep paging beyond fusion window falls back to BM25
	if params.From >= e.cfg.HybridFusionWindow {
		res, err := e.idx.Search(ctx, params.Query, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, params.From, params.Size)
		return res, false, err
	}

	// Fetch up to fusion window candidates from each strategy
	fusionPoolSize := params.From + params.Size
	if fusionPoolSize < 50 {
		fusionPoolSize = 50
	}
	if fusionPoolSize > e.cfg.HybridFusionWindow {
		fusionPoolSize = e.cfg.HybridFusionWindow
	}

	var (
		wg         sync.WaitGroup
		lexResult  index.SearchResult
		lexErr     error
		semResult  index.SearchResult
		semErr     error
		semFloored bool
		isDegraded bool
	)

	// Execute Lexical Strategy
	wg.Add(1)
	go func() {
		defer wg.Done()
		lexResult, lexErr = e.idx.Search(ctx, params.Query, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, 0, fusionPoolSize)
	}()

	// Execute Semantic Strategy
	wg.Add(1)
	go func() {
		defer wg.Done()
		if e.embedClient == nil {
			semErr = context.Canceled
			return
		}
		// Bound semantic embedding to max 1.5s timeout for fast fail-open
		embedCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()

		vec, err := e.embedClient.Embed(embedCtx, params.Query)
		if err != nil {
			semErr = err
			return
		}
		semResult, semErr = e.idx.SearchVector(ctx, vec, params.Filters, params.CategoryID, params.MinPrice, params.MaxPrice, params.MinRating, params.SortBy, 0, fusionPoolSize)
		if semErr == nil {
			semResult, semFloored = e.applySemanticFloor(semResult, 0)
		}
	}()

	wg.Wait()

	// Handle strategy outcomes (D2: Fail-open)
	if lexErr != nil && semErr != nil {
		return index.SearchResult{}, false, lexErr
	}

	// Semantic failed: fail open cleanly to lexical.
	if semErr != nil {
		isDegraded = true
		if lexErr != nil {
			return index.SearchResult{}, false, lexErr
		}
		return index.SearchResult{
			Hits:   paginateHits(lexResult.Hits, params.From, params.Size),
			Total:  lexResult.Total,
			Facets: lexResult.Facets,
		}, isDegraded, nil
	}
	// Semantic succeeded but nothing is above the similarity floor: the lexical
	// leg alone answers, and it is not a degradation.
	if lexErr == nil && len(semResult.Hits) == 0 {
		return index.SearchResult{
			Hits:   paginateHits(lexResult.Hits, params.From, params.Size),
			Total:  lexResult.Total,
			Facets: lexResult.Facets,
		}, false, nil
	}

	// Lexical failed or matched nothing: the semantic candidates (already above
	// the floor) answer. Zero candidates means zero hits.
	if lexErr != nil || len(lexResult.Hits) == 0 {
		isDegraded = lexErr != nil
		facets := semResult.Facets
		switch {
		case len(semResult.Hits) == 0 && lexErr == nil:
			facets = lexResult.Facets // lexical matched nothing either: empty buckets
		case semFloored && len(semResult.Hits) > 0:
			facets = e.facetsFor(ctx, hitIDs(semResult.Hits), params, semResult.Facets)
		}
		return index.SearchResult{
			Hits:   paginateHits(semResult.Hits, params.From, params.Size),
			Total:  semResult.Total,
			Facets: facets,
		}, isDegraded, nil
	}

	// Perform in-process RRF Fusion
	candidates := map[string][]Candidate{
		"lexical":  toCandidates(lexResult.Hits, "lexical"),
		"semantic": toCandidates(semResult.Hits, "semantic"),
	}
	weights := map[string]float64{
		"lexical":  e.cfg.LexicalWeight,
		"semantic": e.cfg.SemanticWeight,
	}

	fused := RRF(candidates, e.cfg.HybridRRFK, weights)

	// Optional Cross-Encoder Reranker
	if e.cfg.EnableReranker && e.rerankClient != nil && len(fused) > 0 {
		rerankLimit := 20
		if len(fused) < rerankLimit {
			rerankLimit = len(fused)
		}
		docs := make([]RerankDoc, rerankLimit)
		for i := 0; i < rerankLimit; i++ {
			docs[i] = RerankDoc{ID: fused[i].ListingID, Text: fused[i].Text}
		}
		reorderedIDs, err := e.rerankClient.Rerank(ctx, params.Query, docs)
		if err != nil {
			log.Printf("[retrieval] rerank error: %v, keeping RRF order", err)
		}
		if err == nil && len(reorderedIDs) == len(docs) {
			idToCand := make(map[string]Candidate, len(fused))
			for _, c := range fused {
				idToCand[c.ListingID] = c
			}
			reranked := make([]Candidate, 0, len(fused))
			for _, id := range reorderedIDs {
				if c, ok := idToCand[id]; ok {
					c.Strategy = "reranked"
					reranked = append(reranked, c)
				}
			}
			for i := rerankLimit; i < len(fused); i++ {
				reranked = append(reranked, fused[i])
			}
			fused = reranked
		}
	}

	// Paginate fused candidates
	pagedCandidates := paginateCandidates(fused, params.From, params.Size)
	hits := make([]index.Hit, 0, len(pagedCandidates))
	for _, c := range pagedCandidates {
		hits = append(hits, index.Hit{ListingID: c.ListingID, Score: c.Score, Stock: c.Stock})
	}

	// Total and facets describe the fused candidate set the hits come from: the
	// semantic leg's own total is k (always a full page of neighbours), so it must
	// not inflate the total. When the fused set is exactly the lexical set the
	// lexical facets are already right; otherwise they are re-aggregated over the
	// fused ids under the same filters.
	total := lexResult.Total
	if int64(len(fused)) > total {
		total = int64(len(fused))
	}
	facets := lexResult.Facets
	if !sameSet(fused, lexResult.Hits) || lexResult.Total > int64(len(lexResult.Hits)) {
		facets = e.facetsFor(ctx, candidateIDs(fused), params, lexResult.Facets)
	}

	return index.SearchResult{
		Hits:   hits,
		Total:  total,
		Facets: facets,
	}, false, nil
}

// semanticMinKNNScore converts the configured cosine-similarity floor to the
// score OpenSearch reports for a cosinesimil knn query (Lucene engine):
// score = (1 + cosine) / 2, in [0, 1]. A floor at or below -1 disables it.
func (e *Engine) semanticMinKNNScore() (float64, bool) {
	if e.cfg.SemanticMinScore <= -1 {
		return 0, false
	}
	return (1 + e.cfg.SemanticMinScore) / 2, true
}

// applySemanticFloor drops semantic hits whose cosine similarity is below the
// configured floor. Hits arrive sorted by score, so everything after the first
// miss is dropped too; total shrinks to what is left (offset + kept).
func (e *Engine) applySemanticFloor(res index.SearchResult, from int) (index.SearchResult, bool) {
	floor, on := e.semanticMinKNNScore()
	if !on {
		return res, false
	}
	kept := make([]index.Hit, 0, len(res.Hits))
	for _, h := range res.Hits {
		if h.Score >= floor {
			kept = append(kept, h)
		}
	}
	if len(kept) == len(res.Hits) {
		return res, false
	}
	res.Hits = kept
	res.Total = int64(from + len(kept))
	return res, true
}

// facetsFor aggregates facets over exactly the given candidate ids under the
// request's filters. An index without that capability, or a failed aggregation,
// keeps the fallback facets rather than failing the search.
func (e *Engine) facetsFor(ctx context.Context, ids []string, params SearchParams, fallback index.Facets) index.Facets {
	fc, ok := e.idx.(index.FacetCounter)
	if !ok || len(ids) == 0 {
		return fallback
	}
	f, err := fc.FacetsForIDs(ctx, ids, params.Filters)
	if err != nil {
		log.Printf("[retrieval] facets for fused candidates: %v, keeping leg facets", err)
		return fallback
	}
	return f
}

func hitIDs(hits []index.Hit) []string {
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ListingID)
	}
	return ids
}

func candidateIDs(cs []Candidate) []string {
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.ListingID)
	}
	return ids
}

// sameSet reports whether the fused candidates are exactly the given hits.
func sameSet(fused []Candidate, hits []index.Hit) bool {
	if len(fused) != len(hits) {
		return false
	}
	in := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		in[h.ListingID] = struct{}{}
	}
	for _, c := range fused {
		if _, ok := in[c.ListingID]; !ok {
			return false
		}
	}
	return true
}

// isKeySort reports whether sortBy orders by a document key rather than relevance.
func isKeySort(sortBy searchv1.SortBy) bool {
	switch sortBy {
	case searchv1.SortBy_SORT_BY_NEWEST, searchv1.SortBy_SORT_BY_PRICE_ASC, searchv1.SortBy_SORT_BY_PRICE_DESC:
		return true
	}
	return false
}

func toCandidates(hits []index.Hit, strategy string) []Candidate {
	cands := make([]Candidate, 0, len(hits))
	for i, h := range hits {
		cands = append(cands, Candidate{
			ListingID: h.ListingID,
			Score:     h.Score,
			Rank:      i + 1,
			Strategy:  strategy,
			Stock:     h.Stock,
			Text:      h.Text,
		})
	}
	return cands
}

func paginateHits(hits []index.Hit, from, size int) []index.Hit {
	if from >= len(hits) {
		return []index.Hit{}
	}
	end := from + size
	if end > len(hits) {
		end = len(hits)
	}
	return hits[from:end]
}

func paginateCandidates(cands []Candidate, from, size int) []Candidate {
	if from >= len(cands) {
		return []Candidate{}
	}
	end := from + size
	if end > len(cands) {
		end = len(cands)
	}
	return cands[from:end]
}
