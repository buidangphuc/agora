# Proposal: Add Hybrid Retrieval Platform

## Why

Search in Agora currently operates under three distinct, uncoordinated retrieval silos:
1. `team-search` provides pure lexical keyword retrieval (BM25 over `title^2 + description`) in OpenSearch.
2. `platform-recsys` trains collaborative / latent factor representations (ALS) exported into Redis & Qdrant for recommendations.
3. `team-ai` / `platform-modelserve` manages dense semantic embeddings and cross-encoder rerankers.

Pure BM25 suffers from vocabulary mismatch (synonyms, phrasing, and multilingual nuances). Conversely, pure vector k-NN lacks precise keyword/token filtering and exact-match guarantees for SKU/brand queries.

This change unifies these streams into a **Hybrid Retrieval Platform** inside `team-search`, bringing together Lexical (BM25), Dense Semantic (k-NN vector), and Behavioral signals using Go in-process Reciprocal Rank Fusion (RRF), per-strategy fail-open resiliency, and index-time embedding generation via `platform-modelserve`.

## What Changes

- **Contract** (`platform-core/packages/proto`):
  - Add `enum SearchMode { SEARCH_MODE_UNSPECIFIED = 0; SEARCH_MODE_HYBRID = 1; SEARCH_MODE_LEXICAL = 2; SEARCH_MODE_SEMANTIC = 3; }`.
  - Add `SearchMode search_mode = 9;` to `SearchListingsRequest`.
- **Read-Model & Indexing** (`team-search`):
  - Update OpenSearch mapping to support `knn_vector` (`dimension: 384`, `engine: lucene`, `space_type: cosinesimil`) and `vector_pending: boolean`.
  - On listing events, asynchronously vectorize text (`title + description`) via `platform-modelserve` `/embed`.
  - If embedding fails at index time, fail open: save document with `vector_pending: true` without blocking ingestion or sending to DLQ.
- **Retrieval Engine & Fusion** (`team-search/internal/retrieval`):
  - Implement concurrent multi-strategy execution: `LexicalStrategy` (BM25), `SemanticStrategy` (vector k-NN), and `BehavioralStrategy`.
  - In-process Reciprocal Rank Fusion (RRF) with default $k=60$.
  - Strategy fail-open: if semantic search fails or times out, degrade cleanly to lexical matches.
  - Deep paging fallback: queries beyond fusion window (e.g., offset $> 200$) fall back to BM25 pagination.
  - Optional reranker stage calling `platform-modelserve` `/rerank`.
- **E2E & Manifests** (`team-search`, `platform-e2e`):
  - Update `team-search/FEATURES.yaml`.
  - Add `platform-e2e/tests/e2e/features/search/hybrid_retrieval.feature`.

## Non-goals

- Image/multimodal search is out of scope.
- Full Learning-to-Rank (LTR) model training in Go; reranking uses the cross-encoder in `platform-modelserve`.
