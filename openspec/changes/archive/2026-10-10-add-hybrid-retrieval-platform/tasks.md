# Tasks: Hybrid Retrieval Platform

## 1. Proto Contract (`platform-core`)
- [x] Add `SearchMode` enum and `search_mode` field to `platform/search/v1/search.proto`.
- [x] Re-vendor proto to `team-search` and regenerate protobuf Go stubs.

## 2. Config & Observability (`team-search`)
- [x] Add `MODEL_SERVER_URL`, `EMBEDDING_DIM`, `HYBRID_FUSION_WINDOW`, `HYBRID_RRF_K`, `ENABLE_HYBRID_SEARCH`, `ENABLE_RERANKER` to `internal/config/config.go`.

## 3. Retrieval Strategy & Fusion Engine (`team-search/internal/retrieval`)
- [x] Implement `fusion.go` (Reciprocal Rank Fusion with $k=60$ and deduplication).
- [x] Implement `embed_client.go` (HTTP client to `platform-modelserve` `/embed`).
- [x] Implement `rerank_client.go` (HTTP client to `platform-modelserve` `/rerank`).
- [x] Implement `engine.go` (Concurrent multi-strategy execution and per-strategy fail-open).
- [x] Add unit tests for fusion, clients, and engine in `internal/retrieval/`.

## 4. OpenSearch Vector Index & Query (`team-search/internal/index`)
- [x] Update `indexMapping` with `knn_vector` and `vector_pending` fields.
- [x] Implement `SearchVector` and k-NN query builder in `internal/index/opensearch.go`.
- [x] Add unit tests for k-NN queries and mapping.

## 5. Ingestion Pipeline Vectorization (`team-search/internal/consumer`)
- [x] Update `ListingEventHandler` to vectorize listing text on create/update events.
- [x] Implement fail-open `vector_pending: true` on embedding failures.
- [x] Add unit tests in `internal/consumer/`.

## 6. gRPC Handler & Mode Routing (`team-search/internal/handler`)
- [x] Wire `RetrievalEngine` into `SearchHandler`.
- [x] Support `req.GetSearchMode()` and deep pagination fallback.
- [x] Update unit tests in `internal/handler/search_test.go`.

## 7. E2E & Manifests (`team-search`, `platform-e2e`)
- [x] Update `team-search/FEATURES.yaml` with `search.hybrid-retrieval`.
- [x] Add `platform-e2e/tests/e2e/features/search/hybrid_retrieval.feature`.

## 8. Verification & Gate
- [x] Validate OpenSpec change with `openspec validate add-hybrid-retrieval-platform --strict`.
- [x] Run `go test ./...` in `team-search`.
- [x] Run `python3 scripts/features.py --strict` in `platform-e2e`.

## Evidence (2026-10-10)

- Code and unit tests: each repo's `make check` / test suite was green at merge (see the commit bodies).
- e2e after rebuilding team-ai, team-search (server and indexer), gateway, frontend and the recsys image, with
  platform-recsys-nearline and the modelserve overlay (fake TEI + router) running:
  - ML scenarios: 23/23, twice;
  - modelserve, hybrid and taxonomy: 27/27, three times;
  - placement and serve-trained scenarios: green three times.
- Scenarios that cannot be produced end to end carry a VERIFIED BY line in the spec and a not-testable FEATURES
  entry.
- spec_sync --strict reports e2e-ready.

- Final gate (2026-10-10): parallel lane 775/775 (w10-par) and 775/776 (w9-par; its one failure was the gateway-wide denylist gauge scenario, moved to the serial lane in e8373e00). Destructive lane 100/101 (w9-dfull); its one failure, backpressure, was fixed in 7f4ae454 and 4d325f2f and then passed twice in the outage-then-backpressure order.
