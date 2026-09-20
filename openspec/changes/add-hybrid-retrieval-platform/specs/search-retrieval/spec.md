## ADDED Requirements

### Requirement: Multi-strategy candidate retrieval

The system SHALL support concurrent candidate retrieval across multiple independent strategies: Lexical (BM25), Semantic (Dense Vector k-NN), and Behavioral.

#### Scenario: Lexical strategy returns matching keyword candidates
- **WHEN** a search query is executed under lexical strategy
- **THEN** matching candidates with positive BM25 scores are returned

#### Scenario: Semantic strategy returns dense vector nearest neighbors
- **WHEN** a search query is vectorized and executed under semantic strategy
- **THEN** top nearest neighbor candidates based on cosine similarity are returned

#### Scenario: Multi-strategy retrieval executes strategies concurrently
- **WHEN** a hybrid query executes against the retrieval engine
- **THEN** both lexical and semantic retrieval stages run concurrently within configured deadlines

### Requirement: In-process Reciprocal Rank Fusion

The system SHALL combine ranked candidate lists using in-process Reciprocal Rank Fusion ($RRF(d) = \sum \frac{w_s}{k + r_s(d)}$) with rank deduplication.

#### Scenario: Items present in multiple candidate lists are boosted
- **WHEN** an item appears in both lexical and semantic candidate lists
- **THEN** its fused RRF score is strictly greater than if it appeared in only one list

#### Scenario: RRF constant k stabilizes rank position weights
- **WHEN** candidates are merged using RRF with parameter $k=60$
- **THEN** rank scores decrease smoothly without outlier score dominance

#### Scenario: Strategy weights scale individual strategy influence
- **WHEN** strategy weights are adjusted
- **THEN** the higher-weighted strategy exerts proportional influence on the final candidate ordering

### Requirement: Strategy Fail-Open Resiliency

The system SHALL fail open cleanly when an individual retrieval strategy encounters errors or timeouts.

#### Scenario: Semantic strategy timeout degrades to lexical results
- **WHEN** the semantic embedding or vector query exceeds its deadline
- **THEN** the engine completes the search using lexical candidates without failing the request

#### Scenario: Model server unavailability fails open
- **WHEN** the embedding model server is unreachable
- **THEN** the search request succeeds by falling back to lexical search

#### Scenario: Degraded search responses maintain facet aggregations
- **WHEN** hybrid search degrades to lexical
- **THEN** facet counts (category, price range, rating, seller) are accurately returned

### Requirement: Additive SearchMode Contract

The system SHALL support explicit search mode selection (`HYBRID`, `LEXICAL`, `SEMANTIC`) via the `search_mode` field in `SearchListingsRequest`.

#### Scenario: Unspecified search mode defaults to hybrid
- **WHEN** `search_mode` is `SEARCH_MODE_UNSPECIFIED`
- **THEN** the engine defaults to hybrid retrieval

#### Scenario: Explicit lexical mode skips vector embedding
- **WHEN** `search_mode` is `SEARCH_MODE_LEXICAL`
- **THEN** only lexical search is executed and no embedding call is made

#### Scenario: Explicit semantic mode queries vector index
- **WHEN** `search_mode` is `SEARCH_MODE_SEMANTIC`
- **THEN** dense vector retrieval is performed

### Requirement: Optional Cross-Encoder Re-ranking Stage

The system SHALL support an optional re-ranking stage on top fused candidates when enabled by configuration.

#### Scenario: Reranker scores top candidates
- **WHEN** `ENABLE_RERANKER` is enabled
- **THEN** top-$M$ fused candidates are re-scored via the cross-encoder endpoint

#### Scenario: Reranker failure falls back to RRF order
- **WHEN** the reranker endpoint fails or times out
- **THEN** results are returned in their original RRF fused order

### Requirement: Index-time Vector Embedding with Fail-Open

The system SHALL vectorize listings upon ingestion and fail open without blocking catalog writes or sending to DLQ.

#### Scenario: Published listing receives embedding at index time
- **WHEN** a `ListingChanged` event is ingested
- **THEN** its text is vectorized and indexed into the OpenSearch `embedding` field

#### Scenario: Embedding failure marks vector_pending without DLQ
- **WHEN** model server vectorization fails during indexing
- **THEN** the listing document is indexed with `vector_pending = true` and no error is raised to DLQ

#### Scenario: Fine-grained price or status update preserves existing embedding
- **WHEN** a `ListingPricingChanged` event occurs
- **THEN** the document price is partially updated without overwriting or clearing the existing embedding

### Requirement: Rebuild Read-Model by Kafka Event Replay

The system SHALL support full index rebuilds including vector embeddings by replaying Kafka `listing.events` onto a new index.

#### Scenario: Replaying listing events generates dense embeddings
- **WHEN** `listing.events` topic is replayed from offset zero
- **THEN** the target index is fully populated with both text and vector fields

#### Scenario: Version guard prevents stale events during re-indexing
- **WHEN** an out-of-order event arrives during replay
- **THEN** the external version guard rejects the stale version

### Requirement: Independent Scalability & Fusion Windowing

The system SHALL decouple query-time fusion resources from deep pagination offsets.

#### Scenario: Search within fusion window executes multi-strategy fusion
- **WHEN** `from + size <= HYBRID_FUSION_WINDOW`
- **THEN** multi-strategy candidate retrieval and RRF fusion are executed

#### Scenario: Deep pagination beyond fusion window falls back to BM25
- **WHEN** `from > HYBRID_FUSION_WINDOW`
- **THEN** pagination is served directly by lexical offset query
