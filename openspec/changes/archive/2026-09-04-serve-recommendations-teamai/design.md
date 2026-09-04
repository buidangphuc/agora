## Context

`team-ai` already serves gRPC (SearchService, AIService, ChatService) over a Qdrant-backed RAG
stack and a Redis client. The recommendation contract
(`platform.recommendation.v1.RecommendationService/Recommend`) is defined by
**add-recommendation-contract**; the item vectors and pre-computed Top-N user lists are produced
offline by **build-als-training-job**. This change is the online serving layer: it reads those
two artifacts and answers `Recommend` within a tight latency budget. It follows the same seam
`search.py` uses — a thin gRPC servicer over a module that owns the retrieval/ranking logic,
with all ML (vectorization / model training) kept out of process.

Assumed contract shape (from add-recommendation-contract — confirm at apply time):
`RecommendRequest { string user_id; string seed_listing_id; Page page }` →
`RecommendResponse { repeated ProductCard recommendations }`, where `ProductCard` matches the
shape already used by `AIService` (listing_id, title, price, currency, image_url, …). `user_id`
empty ⇒ anonymous; `seed_listing_id` set ⇒ PDP "similar items" context, empty ⇒ home "for you".

## Decisions

- **Two-stage retrieval → ranking.** Stage 1 (candidate retrieval): an ANN query against the
  Qdrant collection returns the Top-100 nearest item vectors to the query vector. Stage 2
  (ranking + business rules): score candidates, apply business-rule filters, and truncate to
  Top-10. Keeping retrieval (recall-oriented, cheap, ANN) separate from ranking (precision-
  oriented, business-aware) is the standard recsys split and lets each side evolve independently
  — retrieval Top-K and result Top-K are separate config knobs (`RECS_CANDIDATE_TOP_K=100`,
  `RECS_RESULT_TOP_K=10`).

- **Redis pre-computed cache is the fast path, Qdrant is the fallback.** The training job writes
  a ready-made Top-N list per user to Redis. On `Recommend`, the module first reads
  `recs:v1:user:{user_id}` (key prefix + schema version configurable). On hit, it applies only
  the cheap freshness business-rule filters (in-stock, dedupe) and returns — no Qdrant round trip.
  On miss (cold user, expired key, anonymous), it falls back to the live two-stage Qdrant path.
  This keeps the common logged-in case within budget while still serving everyone.

- **Qdrant collection contract with the training job.** team-ai does **not** create or write the
  collection — build-als-training-job owns its lifecycle. This change only reads it. The shared
  contract (must match the training job exactly):
  - Collection name: `RECS_QDRANT_COLLECTION` (default `item_als_vectors`), same value both sides.
  - Point id = listing id; vector = ALS item factor (fixed dim `RECS_VECTOR_DIM`, cosine distance).
  - Payload carries the business-rule fields read at serve time: `in_stock` (bool),
    `category_id`, `seller_id`, `title`, `price`, `currency`, `image_url` — so ranking/filtering
    needs no extra service call on the hot path.
  A dim/metric/name mismatch is a deploy-time contract break, surfaced by a startup collection
  check (log + `UNAVAILABLE`), not a silent empty result.

- **Redis cache key + fallback.** Key: `{RECS_CACHE_PREFIX}:{schema_ver}:user:{user_id}`; value:
  an ordered list of listing ids (+ optional score). TTL `RECS_CACHE_TTL_SECONDS` bounds staleness
  between training runs. Redis unavailable or malformed value ⇒ treat as a miss and fall back to
  Qdrant (never fail the RPC on a cache error). Anonymous (`user_id` empty) skips the cache read
  entirely and goes straight to the Qdrant path seeded from `seed_listing_id`.

- **Cold-start / anonymous handling.** Resolution order: (1) logged-in with a cache hit → serve
  the pre-computed list; (2) logged-in cache miss **with** a `seed_listing_id` → Qdrant ANN
  around that item's vector (PDP "similar"); (3) anonymous or no seed and no cache → a popularity
  fallback (a maintained `recs:v1:popular` list / global collection query) so the row is never
  empty. This ordering means a new user still sees relevant items and the UI always gets a
  non-empty Top-10 when any catalog exists.

- **Latency budget < 15ms (serve path, p99, excluding network to caller).** The cache fast path
  is a single Redis read + in-memory filter (well under budget). The Qdrant fallback path (one
  ANN query capped at Top-100 + in-memory ranking over payload already returned by Qdrant) is the
  budgeted worst case; `RECS_RETRIEVE_TIMEOUT_MS` caps it, and on timeout the module returns the
  popularity fallback rather than exceeding budget. No per-candidate external calls — all
  business-rule fields come from the Qdrant payload / Redis value, so the hot path makes at most
  one datastore round trip.

- **Backend switch (mirrors `RAG_BACKEND`).** `RECS_BACKEND = "qdrant" | "memory"`. `memory` is an
  in-process deterministic fixture (offline dev / e2e without real infra, like the RAG `memory`
  backend); `qdrant` is the real path. This is the ML/infra decoupling seam — team-ai runs no
  model in-process; it only reads vectors the training job produced.

- **Gating identical to RAG.** A `RECS_ENABLED` flag (default `false`) provides the module to the
  servicer; when off, `Recommend` aborts `UNAVAILABLE` with a clear message
  (`"recommendations are not enabled (RECS_ENABLED=false)"`) — exactly how `SearchServicer`
  handles `RAG_ENABLED=false`.

- **Thin transport, no business logic in the servicer.** `RecommendationServicer.Recommend`
  parses the request, enforces scope via `ensure_scopes` (read scope, e.g. `recommendations:read`
  or the existing public read scope forwarded by the gateway), calls the module, and maps results
  to the contract — mirroring `search.py`. All retrieval/ranking/filtering lives in the module.

## Risks / Trade-offs

- **Contract coupling with two unbuilt changes.** The Qdrant collection schema (name/dim/metric/
  payload) and the Redis key/value schema are shared with build-als-training-job; the message
  shape is shared with add-recommendation-contract. All three must agree. Mitigation: the
  assumed shapes above are the negotiation surface, and a startup collection check fails loud on
  mismatch. Values (`RECS_QDRANT_COLLECTION`, `RECS_CACHE_PREFIX`, schema version) are config so
  both sides pin the same literals.
- **Stale recommendations between training runs.** Bounded by the Redis TTL and the training
  cadence; acceptable — freshness beyond that is a training-job concern, not serving.
- **Popularity fallback can feel generic** for brand-new anonymous users; acceptable for a first
  cut and called out as the deliberate cold-start floor (real personalization arrives once the
  user has history the training job can use).
