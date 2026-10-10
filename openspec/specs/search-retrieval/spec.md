# search-retrieval Specification

## Purpose
Defines team-search's hybrid retrieval: lexical and semantic candidates fused with Reciprocal Rank Fusion, filters that constrain every leg, a semantic similarity floor, fail-open behaviour when a strategy or the model server is down, optional re-ranking, index-time embedding, and rebuilding the read model by replaying listing events.

## Requirements

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
- **VERIFIED BY**: team-search/internal/retrieval/spec_hybrid_test.go › TestEngine_StrategiesRunConcurrently (two legs of 250 ms finish in under 500 ms). Not verifiable end to end: overlap in time is not observable through the gateway (see design.md, E2E verification).

### Requirement: In-process Reciprocal Rank Fusion

The system SHALL combine ranked candidate lists using in-process Reciprocal Rank Fusion ($RRF(d) = \sum \frac{w_s}{k + r_s(d)}$) with rank deduplication.

#### Scenario: Items present in multiple candidate lists are boosted
- **WHEN** an item appears in both lexical and semantic candidate lists
- **THEN** its fused RRF score is strictly greater than if it appeared in only one list
- **VERIFIED BY**: team-search/internal/retrieval/fusion_test.go › TestRRF_BoostsItemsAppearingInMultipleLists. Not verifiable end to end: the fused score is not on the wire, only an order that depends on the whole index (see design.md, E2E verification).

#### Scenario: RRF constant k stabilizes rank position weights
- **WHEN** candidates are merged using RRF with parameter $k=60$
- **THEN** rank scores decrease smoothly without outlier score dominance
- **VERIFIED BY**: team-search/internal/retrieval/spec_hybrid_test.go › TestRRF_ConstantKFlattensRankScores. Not verifiable end to end: `HYBRID_RRF_K` is deployment configuration and the scores are not on the wire (see design.md, E2E verification).

#### Scenario: Strategy weights scale individual strategy influence
- **WHEN** strategy weights are adjusted
- **THEN** the higher-weighted strategy exerts proportional influence on the final candidate ordering
- **VERIFIED BY**: team-search/internal/retrieval/fusion_test.go › TestRRF_WeightsScaleStrategyInfluence. Not verifiable end to end: the weights are deployment configuration (`HYBRID_LEXICAL_WEIGHT`, `HYBRID_SEMANTIC_WEIGHT`), not a request field (see design.md, E2E verification).

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

### Requirement: Structured filters constrain every retrieval leg

The system SHALL apply every structured filter of a request (status, `in_stock`, category, seller, price range, `tag.<group>`, `sku.<group>`) to every retrieval leg, the semantic k-NN leg included, inside the vector query's own filter so the nearest neighbours are found among the matching listings rather than post-filtered. In HYBRID mode the total and the facet aggregations SHALL describe the same filtered, fused candidate set the hits come from; a leg's own k is never reported as the total.

#### Scenario: In-stock filter excludes a sold-out listing from semantic candidates
- **WHEN** a buyer searches in HYBRID mode with `in_stock=true` for a word that only the semantic leg matches, and a sold-out listing is semantically identical to an in-stock one
- **THEN** the in-stock listing is among the hits and the sold-out listing is not

#### Scenario: Price range filter constrains semantic candidates
- **WHEN** a buyer searches in HYBRID mode with a price range for a word that only the semantic leg matches, and one listing is inside the range and one outside
- **THEN** only the listing inside the range is among the hits

#### Scenario: Hybrid facets and total describe the fused candidate set
- **WHEN** a buyer searches in HYBRID mode and the semantic leg contributes a listing the lexical leg does not match
- **THEN** the category facet counts sum to the response total and count that semantic-only listing

### Requirement: Semantic similarity floor

The system SHALL drop a semantic candidate whose cosine similarity to the query is below `HYBRID_SEMANTIC_MIN_SCORE` (default 0.65, calibrated for bge-small-en-v1.5 by `team-search/scripts/semantic_floor_probe.py`; a value of -1 or less disables it), in HYBRID and SEMANTIC mode, because k-NN has no natural cutoff and would otherwise return the k nearest listings for any text. A query whose lexical leg is empty and whose semantic candidates are all below the floor SHALL return zero hits and a zero total. OpenSearch reports `(1 + cosine) / 2` for a `cosinesimil` k-NN query on the Lucene engine; the floor is configured as a cosine and converted.

#### Scenario: Unrelated text returns no semantic candidates
- **WHEN** a buyer searches in HYBRID mode for a term that matches no listing lexically and is semantically unrelated to every listing
- **THEN** the answer is 200 with no hits and a total of zero

#### Scenario: A related listing survives the similarity floor
- **WHEN** a buyer searches in HYBRID mode for a term that matches no listing lexically but is a semantic alias of a listing's title
- **THEN** that listing is among the hits and an unrelated listing is not

#### Scenario: The default floor is calibrated
- **WHEN** team-search starts without `HYBRID_SEMANTIC_MIN_SCORE`
- **THEN** the floor is a cosine of 0.65
- **VERIFIED BY**: team-search/internal/config/config_test.go › TestSemanticMinScoreDefault. Not verifiable end to end: the e2e overlay sets its own floor for the fake TEI, so a black-box run cannot observe the default.

#### Scenario: Unrelated neighbours do not inflate total or paging
- **WHEN** a buyer searches in HYBRID mode for a keyword shared by three listings in an index holding many unrelated listings
- **THEN** the total is three and there is no next page

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
