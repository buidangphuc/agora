# ADR-0012 — Recommendation Placement Engine & Fallback Architecture

**Status:** Accepted · **Date:** 2026-09-20 · **Relates to:** ADR-0005, ADR-0011, ADR-0014

## Context

Recommendations across the marketplace serve distinct placements with different business intents and context:
- `home_feed`: Broad personalization based on historical interactions and real-time session signals.
- `similar_items`: Item-to-item visual/semantic similarity and co-purchasing on product detail pages.
- `cart_cross_sell`: Complementary category add-ons based on basket contents.
- `search_recommend`: Post-search suggestions and query completion augmentations.

Previously, `team-ai` served an unstratified mock recommendation endpoint with no placement configuration, no fallback ladder, and no unified handling for cold-start edge cases.

## Decision

Adopt a unified **Placement Engine** in `team-ai` structured around 4 foundational design pillars:

### 1. Three-Layer Configuration (WHAT vs. HOW)
- **Layer 1: Global Platform Defaults**: Timeout budgets (e.g., 50ms SLA), circuit breaker thresholds, default candidate counts.
- **Layer 2: Placement Definitions (WHAT / HOW)**:
  - *WHAT*: Target items to surface, business rules, diversity constraints, category exclusions.
  - *HOW*: Candidate retrieval sources (ALS, Qdrant vectors, Nearline session, Popularity floor) and ranking pipelines.
- **Layer 3: Request-Time Overrides**: Dynamic caller parameters (e.g., category filter, excluded listing IDs, active session interactions).

### 2. The Fallback Ladder (Relaxation Ladder)
When candidate retrieval returns fewer items than requested (due to filters, sparse data, or timeout), the engine progressively steps down the fallback ladder without failing the request:
- **Tier 1 (Personalized ML)**: Personalized candidate generation (ALS / Two-Tower embeddings + Nearline real-time session vectors + GBDT ranker).
- **Tier 2 (Contextual / Semantic)**: Item-to-item similarity in Qdrant vector index or category co-occurrence.
- **Tier 3 (Category Popular)**: Trending items within the queried category.
- **Tier 4 (Global Floor)**: Cached global popularity list (`recs:v1:popular`), guaranteed to return in $< 2$ms.

### 3. Cold-Start 4-Outcome Framework
1. **New User, Known Item** (e.g., Product Detail Page): Surface item-to-item vector similarities + category trending.
2. **Known User, Empty Session** (e.g., Homepage after lapse): Long-term ALS user factors + popular fallback.
3. **Completely Cold User** (e.g., Anonymous first-time visitor): Real-time nearline session clicks if present; otherwise, global trending floor.
4. **Exhausted / Depleted Filter**: Relax category/price bounds down the ladder to fulfill the requested `limit`.

### 4. Call Budget and Latency SLA
- Hard budget: 50ms total SLA.
- Timeouts per retrieval stage (e.g., 20ms for vector search/model inference). If a stage times out, the engine gracefully degrades to the next fallback tier.
- Every response stamps diagnostic metadata: `placement_id`, `model_version`, `fallback_tier`, `attribution_id`.

## Alternatives Rejected

- **Monolithic Single Model for All Placements**: Fails to leverage placement-specific context (cart basket vs. product detail vs. home feed).
- **Hard-Failing on Empty Candidates**: Unacceptable user experience; cold-start must degrade smoothly to trending items.

## Consequences

- Frontend / Gateway can call a single unified recommendation endpoint specifying `placement_id` and receive consistently formatted, attributed recommendations.
- Placements can be tuned independently via configuration without modifying inference pipelines.
