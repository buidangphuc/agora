## Why

`team-ai` must drop sold-out candidates from recommendation placements that ask for `in_stock_only`, using live stock
(batch item attributes can be up to 24 h stale). The stock already lives in the `team-search` read-model (projected from
`ListingStockChanged` and `ListingChanged`) but can only be read through a search query. A direct lookup by listing id
gives a caller the current projected stock for up to 10 listings in one cheap call.

initiative: discovery-followups

## What Changes

- `team-search` implements `SearchService.GetListingStock` (declared by `discovery-followups-contract`, not redefined
  here): `GetListingStockRequest{listing_ids}` returns `GetListingStockResponse{listings: ListingStock{listing_id, optional stock}}`.
- The handler validates 1..10 non-empty ids, reads the read-model documents by id in one batch (`internal/index`, same
  `stock` field `SearchHit.stock` uses) and returns one entry per requested id in request order. `stock` is absent when
  the listing has no document, is a tombstone, or holds no projected stock; `0` is present as `0`.
- The RPC is service-to-service only: it requires principal type `service` (`x-principal-type`) and scope `search:read`; it is not added to
  `team-gateway` routes.
- `stock` is the projected base stock (net of reservations), no variant rollup.
- Dependency for `recs-in-stock-filter`: team-ai's service principal must carry scope `search:read`.
- No change to `SearchListings`, the `in_stock` filter, the read model or the indexer.

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `search-stock-read-model`: adds "Services can read live stock by listing id".

## Impact

- `team-search/internal/rpc` (new handler file beside `search.go`), `team-search/internal/index` (batch get by ids on the
  `Index` port and `OpenSearchIndex`), `team-search/FEATURES.yaml`, generated proto bindings (from the contract slice).
- `platform-e2e` feature and steps for the lookup.
- Depends on `discovery-followups-contract` (proto). Consumed by `recs-in-stock-filter` (team-ai).

## Non-goals

Exposing the RPC through `team-gateway`; more than 10 ids per call; stock for variants; changing what counts as in stock
(the caller applies `stock > 0`, same as the `in_stock` filter).

## Non-functional requirements

- p95 <= 20 ms for a 10-id call (200 sequential calls) on `mainx` with the OpenSearch overlay; server-side index deadline 200 ms; one index round trip per call (no per-id queries).
- Availability follows the read-model: if OpenSearch is unreachable or slower than the deadline the call fails with `UNAVAILABLE` (never a partial or
  zero-filled answer), so callers can fail closed.
- Limit: at most 10 ids per call, enforced before any index access.

## Data classification

| field | class | purpose | consent basis | retention |
|---|---|---|---|---|
| `listing_id`, `stock` | none (catalogue data, no personal data) | stock lookup for recommendation filtering | n/a | not stored; read-only, nothing logged beyond counts |

Requested ids are not written to logs (log id count only).

## Threat model

- Assets: projected stock counts (low sensitivity, but competitive for sellers).
- Abuse cases: a user or anonymous caller scraping stock for arbitrary listings; oversized batches to load the index;
  probing tombstoned or draft listings.
- Authz rule: only a principal of type `service` with `search:read` may call (the gateway grants `search:read` to anonymous callers, so scope alone is insufficient); anonymous gets `UNAUTHENTICATED`, a user principal
  `PERMISSION_DENIED`; the gateway has no route. The 10-id cap bounds load. Tombstoned listings return absent stock, so
  deletion is not distinguishable from "not projected".
