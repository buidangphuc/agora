# Design Decisions — Hybrid Retrieval Platform

## D1: In-process Go Fusion (RRF) vs. OpenSearch Hybrid Search Pipeline
- **Decision**: Perform Reciprocal Rank Fusion ($RRF$) inside `team-search` Go application logic.
- **Rationale**: OpenSearch hybrid search pipeline requires specific search-pipeline configurations and newer plugins. In-process fusion allows merging candidate lists across disparate backend sources (OpenSearch BM25, OpenSearch k-NN vector, and external behavioral caches) with unified timeout management, deduplication, and zero plugin dependency.

## D2: Per-Strategy Fail-Open Resiliency
- **Decision**: Execute retrieval strategies concurrently using goroutines with independent deadlines.
- **Rationale**: If dense vector search or `platform-modelserve` experiences high latency or errors, the lexical strategy still returns results with zero user-facing errors.

## D3: `knn_vector` on OpenSearch Document
- **Decision**: Store dense embeddings directly on the `ListingDoc` in OpenSearch (`dimension: 384`, `engine: lucene`).
- **Rationale**: Single read-model projection, single version guard (AD2), and unified structured filtering (price range, category, status) across all search modes.

## D4: Ingestion Vectorization Failure Policy
- **Decision**: If embedding generation fails during ingestion, mark `vector_pending: true` and index the document without DLQ.
- **Rationale**: Prevents catalog indexing stalls during ML model downtime. Documents remain searchable via BM25 immediately.

## D5: Additive `SearchMode` Contract
- **Decision**: Add `SearchMode` enum (`SEARCH_MODE_UNSPECIFIED`, `SEARCH_MODE_HYBRID`, `SEARCH_MODE_LEXICAL`, `SEARCH_MODE_SEMANTIC`) to `SearchListingsRequest`.
- **Rationale**: Non-breaking additive proto update. Defaults to hybrid when unspecified.

## D6: Optional Reranker Stage
- **Decision**: Support cross-encoder reranking on top-$M$ fused candidates via `platform-modelserve` `/rerank`.
- **Rationale**: Two-stage retrieval pattern (Fast recall of Top-200 $\to$ High precision rerank of Top-20).

## D7: Config-Driven Fusion Parameters
- **Decision**: Make RRF constant $k$, strategy weights, timeouts, and fusion windows configurable via environment variables.
- **Rationale**: Enables tuning in staging/prod without redeploying code binaries.

## D8: Fusion Window and Deep Paging Fallback
- **Decision**: Limit multi-strategy fusion to `from + size <= 200`. For offsets $> 200$, fall back to pure BM25 lexical pagination.
- **Rationale**: High offset vector retrieval and rank fusion requires scanning massive candidate pools; deep pagination is primarily an edge case best served by index offsets.

## D9: Alias Flip + Kafka Replay for Index Migration
- **Decision**: Index mapping changes apply via new index creation, alias pointer flip, and Kafka `listing.events` replay.
- **Rationale**: Conforms to CQRS architecture (ADR-0005).

## D10: Metrics and Observability
- **Decision**: Export Prometheus metrics for strategy latencies, candidate counts, fusion hits, and fallback occurrences.
- **Rationale**: Real-time visibility into retrieval health and degradation states.

## D11: Test Coverage & Isolation
- **Decision**: Comprehensive unit tests covering in-process RRF, strategy fail-over, embed client retries, and gRPC handler delegation.

## E2E verification

The e2e track (`platform-e2e/tests/e2e/features/search/hybrid_retrieval.feature`, `hybrid_indexing.feature`) names each scenario after its
spec title and drives team-search through the gateway. Semantic behaviour needs a model server, so the stack runs the real
`platform-modelserve` router in front of a deterministic TEI fake (`platform-e2e/fakes/tei_fake`, overlay
`platform-e2e/compose/modelserve.override.yaml`; no model or TEI image is downloaded). The fake's request log is the observation point
for "an embedding was / was not requested", and `[[fake ...]]` directives inside a query or title inject the failures (slow or failing
embedding, failing or reversing rerank). Vectors and `vector_pending` are read from the OpenSearch read-model document.

Four scenarios are verified by named Go unit tests instead and carry a `**VERIFIED BY**` line (FEATURES.yaml `status: not-testable`):

- *executes strategies concurrently*: that two stages overlap in time is not visible to a client; `TestEngine_StrategiesRunConcurrently`
  makes each leg take 250 ms and requires the whole call under 500 ms.
- *Items present in multiple candidate lists are boosted*, *RRF constant k ...*, *Strategy weights ...*: the fused score never leaves the
  process, only an order that depends on every other document in the shared index; k and the weights are deployment configuration
  (D7), not request fields. `TestRRF_BoostsItemsAppearingInMultipleLists`, `TestRRF_ConstantKFlattensRankScores` and
  `TestRRF_WeightsScaleStrategyInfluence` check the arithmetic.

Two scenarios are weaker than their wording and say so here. *Version guard prevents stale events during re-indexing* publishes the
stale event to the live index, not mid-replay (the guard is the same script on both paths). *Replaying listing events ...* runs a second
indexer with a new consumer group on a new index and waits for one listing; it reads the whole topic, so it is tagged `@destructive @slow`.
The rating facet of *Degraded search responses maintain facet aggregations* is not asserted: no rating is indexed (see
`search_rating_removed.feature`).
