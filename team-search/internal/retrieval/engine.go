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
		return res, false, nil
	}

	// 3. Hybrid Mode (Multi-Strategy Fusion)
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
	}()

	wg.Wait()

	// Handle strategy outcomes (D2: Fail-open)
	if lexErr != nil && semErr != nil {
		return index.SearchResult{}, false, lexErr
	}

	// If semantic failed, fail open cleanly to lexical
	if semErr != nil || len(semResult.Hits) == 0 {
		isDegraded = true
		if lexErr != nil {
			return index.SearchResult{}, false, lexErr
		}
		// Paginate lexical results
		hits := paginateHits(lexResult.Hits, params.From, params.Size)
		return index.SearchResult{
			Hits:   hits,
			Total:  lexResult.Total,
			Facets: lexResult.Facets,
		}, isDegraded, nil
	}

	// If lexical failed, fall back to semantic
	if lexErr != nil || len(lexResult.Hits) == 0 {
		isDegraded = true
		hits := paginateHits(semResult.Hits, params.From, params.Size)
		return index.SearchResult{
			Hits:   hits,
			Total:  semResult.Total,
			Facets: semResult.Facets,
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
		candidateIDs := make([]string, rerankLimit)
		for i := 0; i < rerankLimit; i++ {
			candidateIDs[i] = fused[i].ListingID
		}
		reorderedIDs, err := e.rerankClient.Rerank(ctx, params.Query, candidateIDs)
		if err == nil && len(reorderedIDs) == len(candidateIDs) {
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
		hits = append(hits, index.Hit{ListingID: c.ListingID, Score: c.Score})
	}

	// Total estimate is max of both strategies
	total := lexResult.Total
	if semResult.Total > total {
		total = semResult.Total
	}

	// Primary facet source is lexical (or semantic if lexical empty)
	facets := lexResult.Facets

	return index.SearchResult{
		Hits:   hits,
		Total:  total,
		Facets: facets,
	}, false, nil
}

func toCandidates(hits []index.Hit, strategy string) []Candidate {
	cands := make([]Candidate, 0, len(hits))
	for i, h := range hits {
		cands = append(cands, Candidate{
			ListingID: h.ListingID,
			Score:     h.Score,
			Rank:      i + 1,
			Strategy:  strategy,
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
