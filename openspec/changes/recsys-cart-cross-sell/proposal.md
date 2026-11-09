# Proposal: recsys-cart-cross-sell

initiative: discovery-seo

## Why

The `cart_cross_sell` placement exists in `team-ai/app/modules/business/recommendation/placements.yaml` but does
nothing useful. `service.py` returns `[]` for the `category_popular` and `cart_cross_similarity` strategies, so every
ladder falls through to `global_popular`. `ranking/filter.py` `eligible()` hard-codes "no seed, in stock" and ignores
the `filters` block. `placements.py` ignores `name`, `enabled`, `timeout_ms`, `allow_cache`, `filters` and `tier`,
yet the file declares them, so the config claims behaviour it does not have. The cart has no recommendations row.

## What changes

- `RecommendQuery` carries `context_listing_ids` (from `RecommendRequest.context_listing_ids`, D5, provided by
  `discovery-seo-contract`); the Recommend servicer in team-ai maps it onto the query. Max 50 ids (the gateway caps; team-ai re-checks).
- `cart_cross_similarity` returns the listings most similar to the context listings (merged by best score).
- `category_popular` returns popular listings from the category of the seed listing, or of the context listings.
- `Placement` declares `filters` (`in_stock_only`, `deduplicate_sellers`, `exclude_seed`, `exclude_cart_items`) and
  `ranking/filter.py` applies exactly the enabled ones (D6). `eligible()` no longer hard-codes seed/stock rules.
- `placements.yaml` and the `placements.py` docstring lose every key the loader does not use (`name`, `enabled`,
  `timeout_ms`, `allow_cache`, `tier`). 
- `cart_cross_sell` enables `deduplicate_sellers: true` (it was absent, so off); `home_feed` keeps `false`, `similar_items` keeps `exclude_seed: true`.
- `placements.yaml` has no `tier` key after this change, neither at placement level nor inside any `candidate_ladder` step (the three `tier:` lines per ladder are deleted). `Placement`, `LadderStep` and `Filters` forbid unknown keys, so a step carrying `tier` is rejected at load.
- `home.trending` does not exist in team-ai (grep over team-ai finds no such placement or key); there is no task for it.
- The `placements.py` docstring stops listing `filters` as ignored and lists only what the loader now declares.
- Gateway (`team-gateway/internal/edge/recommendation.go`, `RecommendationForwarder.Recommend`): forwards `context_listing_ids` and rejects more than 50 with INVALID_ARGUMENT before calling team-ai (C1). team-ai re-validates the same limit with INVALID_ARGUMENT.
- Frontend cart page (`team-frontend/src/app/(shop)/cart/page.tsx`): a "Thường được mua cùng" row from placement `cart_cross_sell`, at most 8 items, sending `context_listing_ids` holding the distinct listing ids of at most 50 cart lines (the last 50 in `GetCart` order, the 50 newest), hidden when the cart is empty, the result is empty or the call errors (the cart still renders), beacons with `impressionId = request_id` and `modelVersion` as in `RecommendationsRow.tsx:36,58-59`.

## Dependency: recsys-candidate-attributes

depends_on: discovery-seo-contract (RecommendRequest.context_listing_ids = 6).

depends_on: recsys-candidate-attributes (owner platform-recsys, decision C10). That slice writes `seller_id`, `category_id` and `visible` into the Qdrant point payload and into the Redis popular/user cache entries; there is no request-time enrichment call. This slice keeps those exact payload key names and adds no proto field.

Today team-ai reads none of them: `Candidate` has only `listing_id`, `score`, `in_stock` (`recommendation/schemas.py:11-16`), the Qdrant adapter reads `listing_id` and `in_stock` (`adapters/qdrant.py:99-108`), the Redis cache entry holds id, score and `in_stock` (`adapters/redis_cache.py:17,119`). This change extends `Candidate` with `seller_id`, `category_id`, `visible`, and the Qdrant adapter and the Redis cache parsing to read them.

Stock: nothing in the platform writes `in_stock` today and team-ai defaults it to true (`adapters/qdrant.py:107`), so `in_stock_only` is a documented no-op in this initiative (C14). A real stock source is follow-up C3-U1. Scenarios that rely on out-of-stock exclusion seed `in_stock=false` directly in the fixture or cache and do not depend on any producer. `seller_id` already exists upstream (C13); `visible` comes from the analytics status export.

The category of a seed or context listing is read from its item-collection payload `category_id`. `category_popular` returns the entries of the popular list whose `category_id` is in that set, and returns nothing when none match.

Rules:
- Fail closed: a candidate whose payload lacks `visible`, or has it false, is not eligible for any placement.
- A candidate lacking `seller_id` is never deduplicated (it is kept and does not count toward any seller). A candidate lacking `category_id` is never categorised (it contributes nothing as a context category and cannot match a category).

Cart-line ordering: neither the cart proto (`platform-core/packages/proto/platform/order/v1/order.proto:33-49`, `CartItem` has id, listing_id, variant_id, quantity, prices, title, image, seller_id) nor the frontend cart actions (`team-frontend/src/features/cart/actions.ts`) expose a line timestamp or add order. `team-order/internal/store/cart.go:65` returns the lines `ORDER BY created_at ASC`, so the frontend takes the last 50 lines `GetCart` returns when the cart holds more than 50, which are the 50 newest.

## NFR

- `cart_cross_sell` p95 under 100 ms server-side at 50 context ids is measured and non-gating (the removed `timeout_ms: 35` was never enforced; no new timeout is introduced).
- Recommend stays available when the new strategies find nothing: the ladder falls through to the next step, never errors.
- Limits: at most 50 context ids (gateway and team-ai both reject more); `result_limit` 8 for `cart_cross_sell`.
- Strategy fan-out is bounded by the 50-id cap.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `context_listing_ids` | Behavioural (cart contents, linked to session only via the request) | Pick cross-sell candidates | Contract necessity (serving the cart page the buyer opened) | Not stored by team-ai; request-scoped; never logged raw (logs carry `context_count` only) |
| `user_id` / `anonymous_id` (existing) | Personal pseudonymous | Ranking features | Unchanged | Unchanged |

Listing ids are public catalogue identifiers. Logs carry counts only (`context_count`), never ids joined to a user.

## Threat model

- Assets: cart contents privacy; recommendation availability.
- Abuse cases: oversized `context_listing_ids` to amplify ANN lookups (cap 50, over-cap rejected); ids of unpublished
  or hidden listings leaking into results (candidates come from the serving index, filtered by the fail-closed `visible` payload flag (recsys-candidate-attributes));
  enumerating private cart data (the request carries only ids the caller supplies; nothing is read from another user's cart).
- Authz rule: `Recommend` stays a public read (same as `home_feed`); no per-user resource is accessed, so no permission scenario applies.

## Clarifications

- `RecommendRequest` has no `placement_id` field. The caller selects the cart placement with `context = RECOMMENDATION_CONTEXT_CART`, and team-ai maps that context to placement `cart_cross_sell` (`team-ai/app/transport/grpc/servicers/recommendation.py`). Scenario text saying the request carries `context=RECOMMENDATION_CONTEXT_CART` means exactly this; the response's placement is `cart_cross_sell`.
- C22: `cart_cross_sell` keeps `gbdt` ranking. The maximum similarity across cart lines is the candidate score fed to the ranker; the response score is the ranker's, so the scenario asserts descending order and membership, not that the response score equals the max similarity.
