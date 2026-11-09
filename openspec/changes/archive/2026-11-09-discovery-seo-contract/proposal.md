## Why

The discovery-seo initiative needs five contract additions before any consumer slice can build: a best-selling sort, search attribution fields, per-hit sold and rating numbers, two projection events, and cart context for recommendations. All of them are additive proto changes owned by platform-core. Shipping them first, and refreshing the vendored consumer copies, lets the order, engagement, search, team-ai, gateway, analytics and frontend slices proceed in parallel.

initiative: discovery-seo

## What Changes

All edits are in `platform-core/packages/proto/platform`; no field is renamed, renumbered or removed.

- `search/v1/search.proto`
  - `SortBy` gains `SORT_BY_BEST_SELLING = 5` (sold_count desc, created_at desc, id).
  - `SearchListingsResponse` gains `string request_id = 4` and `string model_version = 5`.
  - `RunSavedSearchResponse` gains the same two fields at the same numbers.
  - `SearchHit` gains `int64 sold_count = 4`, `double rating_avg = 5`, `int64 review_count = 6`, `string seller_id = 7` (sitemap shop URLs, clarification C4). Today `SearchHit` ends at `stock = 3`.
- `order/v1/order.proto`: new `message ListingSalesChanged { string listing_id = 1; int64 sold_count = 2; google.protobuf.Timestamp changed_at = 3; }`, the payload of an `EventEnvelope` on `order.events`.
- `engagement/v1/events.proto`: new `message ListingRatingChanged { string listing_id = 1; double average_rating = 2; int64 review_count = 3; google.protobuf.Timestamp changed_at = 4; }`, the payload of an `EventEnvelope` on `engagement.events`.
- `recommendation/v1/recommendation.proto`: `RecommendRequest` gains `repeated string context_listing_ids = 6`, documented as cart lines, at most 50. `seed_listing_id = 3` stays.
- `analytics/v1/analytics.proto`: new admin-only `AnalyticsQueryService.GetSearchPerformance(GetSearchPerformanceRequest) returns (GetSearchPerformanceResponse)`, mirroring `GetRecommendationPerformance` (no fallback field; field numbers differ). Request `uint32 window_hours = 1` (0 = 24, max 168). Response `repeated RecommendationPerformanceRow rows = 1`, `uint32 window_hours = 2`, `uint32 attribution_window_hours = 3`, rows with `placement_id = search_results`. The gateway adds the route and the `adminScope` entry in `team-gateway/internal/edge/policy.go` and a forwarder method in `internal/edge/analytics.go`, as for `GetRecommendationPerformance`.
- Proto comments document the semantics: absolute values, counts never below 0, `rating_avg` within 0 to 5 and 0 when review_count is 0, topic, key and envelope type of both events, `model_version` format `<mode>[+rerank]@<version>`, and `context_listing_ids` over 50 rejected with INVALID_ARGUMENT at both the gateway and team-ai (clarification C1).
- Baseline for every compatibility check: a field table pinned in each test file, copied from `origin/main` at the time of this change.
- Vendored copies under `<repo>/proto/platform/...` are refreshed byte-for-byte in every consumer repo that already holds the file: search.proto (team-ai, team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-identity, team-notification, team-order, team-payment, team-search), order.proto (team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-notification, team-order, team-payment, team-promotion, team-referral, team-search), engagement events.proto (team-analytics, team-engagement, team-order), recommendation.proto (team-ai, team-analytics, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway). analytics.proto (team-analytics, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway). Stubs checked in by a consumer are regenerated with that repo's own `buf generate`.
- Contract tests in `platform-core` (descriptor shape, wire compatibility both ways, comment text, consumer-copy equality) in the style of `reviewhidden_compat_test.go` and `q7_consumer_copies_test.sh`.

## Cross-repo edits

Besides platform-core and the vendored `proto/` copies, this slice edits `team-gateway` (`internal/edge/policy.go`, `internal/edge/analytics.go`, `internal/edge/policy_test.go`): the admin-scope policy entry and forwarder for `GetSearchPerformance`. team-gateway has no checked-in analytics stubs (`generated/` is gitignored), so the refreshed `proto/` copy plus its own `buf generate` before task 1.4a is enough. Runtime behaviour of the query (rows, window rejection) belongs to the `search-attribution` slice.

## Capabilities

### New Capabilities
- `discovery-seo-contract`: the proto contract for best-selling sort, search attribution, hit sold and rating numbers, `ListingSalesChanged`, `ListingRatingChanged` and `RecommendRequest.context_listing_ids`.

### Modified Capabilities
none

## Impact

- `platform-core/packages/proto/platform/{search,order,engagement,recommendation,analytics}/v1/*.proto`, new `*_compat_test.go` and `discovery_seo_consumer_copies_test.sh` beside them.
- Vendored `proto/` copies and regenerated stubs in the consumer repos listed above (copy refresh only, no behaviour change).
- `team-gateway/internal/edge/{policy.go,analytics.go,policy_test.go}`: route, admin scope and forwarder for `GetSearchPerformance` (the serving query lives in the `search-attribution` slice).
- No other runtime behaviour changes in this slice; consumers adopt the fields in their own slices.

## Non-goals

Producing or consuming the events, filling the new fields, the `GetSearchPerformance` query implementation in team-analytics, and the 50-id cap logic (other slices). Any `rank_training` or LTR field. A "top-rated" sort value.

## Non-functional requirements

- `buf lint` and `buf breaking` (FILE) against `origin/main` pass: purely additive.
- Wire compatibility: a payload from the previous schema parses with the new one, and the reverse, with all existing fields unchanged.
- The new messages stay small: `ListingSalesChanged` and `ListingRatingChanged` carry 3 and 4 scalar fields, one event per listing change.
- Availability: no runtime component; rollout order is proto first, consumers after (deploy order consumers-before-producers is safe because unknown fields are ignored).

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| `SearchListingsResponse.request_id`, `RunSavedSearchResponse.request_id` | no: identifies a query, not a person | join a click to the search that produced it | not applicable | as the analytics event that carries it |
| `model_version` | no | compare ranking versions | not applicable | as above |
| `SearchHit.sold_count`, `rating_avg`, `review_count` | no: listing aggregates | sort, filter, display | not applicable | rebuildable read-model |
| `ListingSalesChanged` | no: no buyer or order ids | project sold count into search | not applicable | Kafka topic retention |
| `ListingRatingChanged` | no: no reviewer id or review text | project rating into search | not applicable | Kafka topic retention |
| `SearchHit.seller_id` | no: public listing attribute | sitemap shop URLs | not applicable | rebuildable read-model |
| `GetSearchPerformanceResponse` rows | no: aggregates, no per-user data | compare ranking versions | not applicable | computed on read |
| `RecommendRequest.context_listing_ids` | no on its own; sent with the existing user or anonymous id under existing consent rules | cart-driven recommendations | existing personalization consent | not stored by this contract |

## Threat model

- Assets: contract stability for 14 consuming repos; absence of identifiers in the two new events.
- Abuse cases: a breaking edit (renumber or reuse) silently corrupts consumers; a later edit adds buyer or reviewer ids to the events; an unbounded `context_listing_ids` is sent to the recommender; a non-admin reads search performance.
- Authz rule: none new. The fields ride existing RPCs and topics with their existing authz. `GetSearchPerformance` requires scope admin at the gateway and in the serving service. The 50-id cap is a documented contract rule, enforced with INVALID_ARGUMENT by the gateway and team-ai in slice `recsys-cart-cross-sell`. Tests assert the events carry no user fields.
