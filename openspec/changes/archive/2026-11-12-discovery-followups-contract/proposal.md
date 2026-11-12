## Why

The discovery-followups initiative needs two contract additions before the consumer slices can build: an explicit `DELETED` listing status (team-analytics, featurestore and recsys need a value to write instead of null) and a live stock lookup on the search service (team-ai needs it to drop sold-out recommendations). Both are additive proto changes owned by platform-core. Shipping them first, with the vendored consumer copies refreshed, lets `analytics-listing-deleted-status` and `search-listing-stock-lookup` proceed.

initiative: discovery-followups

## What Changes

All edits are in `platform-core/packages/proto/platform`; no field is renamed, renumbered or removed.

- `listing/v1/listing.proto`: `ListingStatus` gains `LISTING_STATUS_DELETED = 7`, commented as set by read models for a hard-deleted listing; team-domain never returns it. Values 0 to 6 are unchanged.
- `search/v1/search.proto`: `SearchService` gains `rpc GetListingStock(GetListingStockRequest) returns (GetListingStockResponse)` with
  - `GetListingStockRequest { repeated string listing_ids = 1; }` (1 to 10 ids, otherwise INVALID_ARGUMENT),
  - `ListingStock { string listing_id = 1; optional int32 stock = 2; }` (stock absent = not projected or unknown),
  - `GetListingStockResponse { repeated ListingStock listings = 1; }` (one entry per requested id, in request order).
- Proto comments document the semantics: the 1 to 10 id bound, in stock means the listing's projected base stock, net of reservations, is greater than 0, exactly as search's `in_stock` filter and `SearchHit.stock` (no variant rollup), absent stock means unknown, service-to-service only (team-gateway does not expose it).
- Baseline for every compatibility check: the listing status table and the `SearchService` rpc and message tables, pinned in the test files, copied from `origin/main` at the time of this change.
- Vendored copies under `<repo>/proto/platform/...` are refreshed byte-for-byte in every consumer repo that already holds the file: listing.proto (team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-identity, team-notification, team-order, team-payment, team-promotion, team-search, team-ai) and search.proto (team-ai, team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-identity, team-notification, team-order, team-payment, team-search). Stubs checked in by a consumer are regenerated with that repo's own `buf generate`.

## Capabilities

### New Capabilities
- `discovery-followups-contract`: the proto contract for `LISTING_STATUS_DELETED` and `SearchService.GetListingStock`.

### Modified Capabilities
none

## Impact

- `platform-core/packages/proto/platform/{listing,search}/v1/*.proto`, new `followups_compat_test.go` files beside them and `followups_consumer_copies_test.sh`.
- Vendored `proto/` copies and regenerated stubs in the consumer repos listed above (copy refresh only, no behaviour change).
- team-gateway gets the refreshed copy but no route: the RPC stays service-to-service.

## Non-goals

Implementing the RPC (team-search slice `search-listing-stock-lookup`), writing or reading `DELETED` (team-analytics, featurestore, recsys), calling the RPC (team-ai). Any change to `ListingChanged`, `ListingStockChanged` or `ListingSalesChanged`. Any gateway route or auth scope for `GetListingStock`.

## Non-functional requirements

- `buf lint` and `buf breaking` (FILE) against `origin/main` pass: purely additive.
- Wire compatibility: a payload from the previous schema parses with the new one, and the reverse, with existing fields unchanged. A consumer that meets status 7 before it is updated reads it as an unknown enum number and must not crash.
- Availability: no runtime component; rollout is proto first, consumers after.
- Latency budget for the RPC (p95 at most 20 ms locally for 10 ids) is met and tested in the `search-listing-stock-lookup` slice.

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| `GetListingStockRequest.listing_ids`, `ListingStock.listing_id` | no: public listing identifiers | name the listings to look up | not applicable | not stored by the contract |
| `ListingStock.stock` | no: inventory count | decide whether a recommendation is purchasable | not applicable | not stored by the contract |
| `LISTING_STATUS_DELETED` | no: lifecycle state of a listing | mark a deleted listing in read models | not applicable | as the read model that carries it |

## Threat model

- Assets: stock counts and listing ids (public marketplace data, no personal data).
- Abuse cases: a client enumerating stock for many listings; an oversized request used as a cheap amplification against OpenSearch; an external caller reaching the RPC through the edge.
- Authorisation rule: the RPC is internal. The 10-id bound is part of the contract; team-gateway does not route it. Enforcement of the bound lives in `search-listing-stock-lookup`.
