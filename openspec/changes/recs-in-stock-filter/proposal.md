## Why

`in_stock_only` placements (`home_feed`, `similar_items`, `cart_cross_sell`) do not drop sold-out listings today.
`Filters.in_stock_only` (`team-ai/app/modules/business/recommendation/placements.py`) checks `Candidate.in_stock`
(`ranking/filter.py`), but no producer writes that value: `adapters/qdrant.py` and `adapters/redis_cache.py` default
it to `True`, so every candidate passes. Batch attributes would be up to 24 h stale, so decision C3-U1
(`plans/ai-first/DECISIONS.md`) asks team-search for live stock instead.

initiative: discovery-followups

## What changes

- team-ai reads live stock through `platform.search.v1.SearchService.GetListingStock` (added by
  `discovery-followups-contract`, implemented by `search-listing-stock-lookup`) for `in_stock_only` placements, but only
  for the top ranked candidates: after the other filters and ranking, the first `limit` x 2 ids capped at 20, in at most
  10 ids per call (at most 2 concurrent calls). If fewer than `limit` in-stock candidates remain, it looks up the next
  ranked candidates (refill) until `limit` is reached, candidates run out or the budget is spent.
- In stock means the projected base stock is greater than 0 (no variant rollup). A missing entry or absent `stock` drops
  only that candidate.
- Fail closed: a call error, a timeout, an auth rejection or a malformed response (entry count or order not matching the
  request) drops every candidate of that placement; the response is empty for that placement (the popular fallback is not
  used, since it needs the same lookup). The RPC never fails because of search.
- The outgoing call carries team-ai's service principal with scope `search:read`; team-search requires a principal of
  type `service` with that scope.
- `Candidate.in_stock` stops defaulting to `True`: it becomes unknown (`None`) when the Qdrant payload or the Redis
  entry carries no value, and `in_stock_only` keeps a candidate only when the value is `True`. For an `in_stock_only`
  placement the live lookup overwrites whatever the payload or cache carried.
- The lookup is memoised per id within a request and capped by the existing `RECS_RETRIEVE_TIMEOUT_MS` budget (no new
  budget). The search address is a new team-ai setting; the gateway is not involved.
- Placements without `in_stock_only` make no search call.

## Capabilities

### Modified Capabilities
- `recommendation-placements`: `in_stock_only` filters on live search stock, fail closed.
- `recommendations`: the serve-path budget covers the stock lookup.

## NFR

- Latency: the lookup shares the `RECS_RETRIEVE_TIMEOUT_MS` budget with retrieval; first phase is one concurrent fan-out
  of at most 2 calls (20 ids), refill calls are serial and only when too few are in stock. Search p95 is at most 20 ms for
  10 ids (initiative NFR). The stock call is a service call and is not counted as a datastore round trip; the 15 ms p99
  of `Recommend` is not asserted by the stock-lookup tests.
- Availability: search being down, slow or rejecting empties in-stock placements (deliberate fail closed); placements
  without `in_stock_only` and the rest of `Recommend` are unaffected.
- Limits: at most 10 ids per call; at most 20 ids in the first phase; refill never exceeds the candidates of the request.

## Data classification

No personal or sensitive data: listing ids and integer stock counts only. No consent basis or retention applies.
Logs carry counts and outcome (ok, timeout, error), never listing ids.

## Threat model

- Assets: eligibility of listings for recommendation; availability of the `Recommend` RPC.
- Abuse cases: (1) forcing search failures to empty a placement, an availability degradation accepted for fail closed,
  bounded by the timeout; (2) a caller inflating lookups through `context_listing_ids` (at most 50, already capped);
  lookup ids come from ranked candidates, never from request ids directly, and are capped at 20 in the first phase;
  (3) a stale `in_stock=true` in Qdrant or Redis resurfacing a sold-out listing, closed by ignoring it on
  `in_stock_only` placements.
- Authz: no new external surface. team-ai calls search with its service principal (type `service`, scope `search:read`);
  a call without it is rejected and treated as fail closed. `GetListingStock` is not exposed by team-gateway.
