# ADR-0005 — Search read-model (OpenSearch lexical → hybrid)

**Status:** Accepted · **Date:** 2026-08-30

## Context

Buyers need to find listings: free-text search, filters, and type-ahead
suggestions — and later semantic (embedding) search and recommendations. The
write-model (`team-domain`, source of truth in `listing_db`) must not be coupled
to how search is served, and search must be able to grow from basic lexical to
hybrid without a rewrite or a contract change.

## Decision

- **CQRS read-model.** Search is a **derived read-model**, separate from the write
  side. `team-domain` emits `listing.events` (ADR-0002); the search service consumes
  them and maintains its own index. The read-model is **rebuildable by replaying the
  topic** — it holds no data the events don't carry.
- **Owner: `team-search` (Go, new).** It owns `platform.search.v1.SearchService`
  (ownership moved from team-ai) and its OWN index store (Rule 3). It runs two
  processes over one codebase: an **indexer** (Kafka consumer) and a **query API**
  (gRPC server).
- **Engine now: OpenSearch, lexical.** Free-text (`multi_match`), structured filters
  (term), and suggestions (`search_as_you_type`). This is the "build the basics
  first" step.
- **Stable contract, swappable engine.** `SearchService` does not change as the
  engine evolves: lexical → **hybrid** (lexical + vector kNN) → learned ranking. The
  query API is the seam; clients (Gateway/buyers) never see the change.
- **AI comes through a seam, not into this service.** Embeddings, rerankers, and
  recommendation models stay owned by **team-ai behind the model-server seam**;
  team-search *calls* model-server for vectors when it adds semantic/hybrid search.
  Ownership split: **AI team = models; search team = the search product** (index
  shape, relevance, filters, suggest).

## Consequences

- `infra/docker-compose.yaml` runs `opensearch` (local, security disabled).
- `search.proto` header + ownership move to team-search; it gains a `Suggest` RPC.
  `SearchListings` already carried `query` + `filters` + pagination — enough for
  lexical.
- Recommendation is a *separate* future read-model/service (`team-reco`), consuming
  user-behavior events + calling model-server — not folded into search.
- Because the index is a replayable projection, schema/mapping changes are handled
  by reindex-from-Kafka, not fragile in-place migrations.
