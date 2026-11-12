# search-stock-read-model Specification

## Purpose
Defines how the search read-model learns each listing's stock from `listing.events`, keeps it correct when events
arrive late, twice or out of order, returns it on search hits and lets a buyer restrict a search to listings in stock.

## Requirements

### Requirement: Stock changes reach search results

`team-search` SHALL apply every `ListingStockChanged` envelope on `listing.events` to the listing's document in its
read-model, and SHALL take `Listing.stock` from every `ListingChanged` it applies. `SearchListings` (in every search
mode) and `RunSavedSearch` SHALL return the listing's current read-model stock on each `SearchHit.stock`. When the
read-model holds no stock for a listing (a document indexed before stock was projected and not yet replayed),
`SearchHit.stock` SHALL be absent; a stock of zero SHALL be present as `0`. A change SHALL be visible through the
gateway within 30 seconds of the event being written to `listing.events`.

#### Scenario: A checkout lowers the stock shown in search

- **GIVEN** a published listing with stock 10 that appears in search
- **WHEN** a buyer checks out quantity 2 of it through the gateway
- **THEN** within 30 seconds `SearchListings` through the gateway returns that listing with `stock` 8

#### Scenario: Cancelling the order restores the stock shown in search

- **GIVEN** that order has lowered the listing's search stock to 8
- **WHEN** the buyer cancels the order through the gateway
- **THEN** within 30 seconds `SearchListings` returns that listing with `stock` 10

#### Scenario: A saved search run shows the current stock

- **GIVEN** a buyer has saved a search whose query matches a listing with stock 10
- **WHEN** another buyer checks out quantity 3 of that listing and the first buyer then runs the saved search through the gateway
- **THEN** within 30 seconds `RunSavedSearch` returns that listing with `stock` 7

### Requirement: Stock never regresses under out-of-order or redelivered events

The read-model SHALL order stock writes by the envelope's `occurred_at`, independently of the order in which other
listing fields are written: a `ListingStockChanged` or `ListingChanged` whose `occurred_at` is not newer than the
stock already stored SHALL leave the stored stock unchanged. A `ListingStockChanged` SHALL NOT create a document for a
listing the read-model does not hold. A `ListingStockChanged` with an empty `listing_id`, a negative `stock` or no
`occurred_at` SHALL NOT be applied and SHALL be retried and then parked on the dead-letter topic
(`listing.events.dlq`), never acknowledged silently.

#### Scenario: A stale stock event does not roll stock back

- **GIVEN** a listing whose search stock is 8 after a real checkout
- **WHEN** a `ListingStockChanged` with stock 10 and an `occurred_at` earlier than that checkout's event is published to `listing.events`
- **THEN** after the search indexer has consumed past that event, `SearchListings` still returns that listing with `stock` 8

#### Scenario: A stock event for an unknown listing creates nothing

- **WHEN** a valid `ListingStockChanged` with stock 5 for a listing id that was never created is published to `listing.events`
- **THEN** after the search indexer has consumed past that event, the read-model holds no document for that id and no search returns it

#### Scenario: A malformed stock event is parked, not applied

- **GIVEN** a listing whose search stock is 10
- **WHEN** a `ListingStockChanged` for that listing with stock -1 is published to `listing.events`
- **THEN** within 60 seconds that record appears on `listing.events.dlq` and `SearchListings` still returns the listing with `stock` 10

### Requirement: A listing update keeps newer stock

When a `ListingChanged` is applied, its `Listing.stock` SHALL replace the stored stock only if the event is newer than
the stored stock; otherwise the stored stock SHALL be carried forward while the event's other fields are applied
under the document's own version guard.

#### Scenario: A late listing update does not overwrite newer stock

- **GIVEN** a listing created with stock 10 whose search stock is 8 after a real checkout
- **WHEN** a `ListingChanged` (UPDATED) for that listing with a new title, stock 10 and an `occurred_at` between its creation and that checkout is published to `listing.events`
- **THEN** within 30 seconds a search for the new title returns that listing with `stock` 8

#### Scenario: A seller's stock edit is reflected in search

- **GIVEN** a seller's published listing with search stock 8
- **WHEN** the seller updates the listing's stock to 50 through the gateway
- **THEN** within 30 seconds `SearchListings` returns that listing with `stock` 50

### Requirement: Buyers can restrict a search to listings in stock

`filters["in_stock"] = "true"` on `SearchListings`, and in a saved search's `filters_json`, SHALL restrict the hits,
the total and every facet count to listings whose read-model stock is greater than zero, in every search mode
(lexical, semantic, hybrid and any fallback between them) and on `RunSavedSearch`. A listing with no read-model stock
SHALL NOT match. Without the filter, stock SHALL NOT affect which listings match. Any other value of
`filters["in_stock"]` SHALL be rejected with `INVALID_ARGUMENT` by `SearchListings`, `SaveSearch` and
`RunSavedSearch`.

#### Scenario: An in-stock search hides a sold-out listing

- **GIVEN** a published listing with stock 1 that matches a query
- **WHEN** a buyer checks out quantity 1 of it through the gateway
- **THEN** within 30 seconds `SearchListings` for that query with `filters["in_stock"]="true"` does not return it and its total no longer counts it, while the same search without the filter returns it with `stock` 0

#### Scenario: The in-stock filter applies in semantic and hybrid search

- **GIVEN** a sold-out listing and an in-stock listing that both match a query
- **WHEN** a buyer runs that query with `filters["in_stock"]="true"` in `SEARCH_MODE_SEMANTIC`, then in `SEARCH_MODE_HYBRID`
- **THEN** both responses return the in-stock listing and neither returns the sold-out one

#### Scenario: A saved in-stock search excludes sold-out listings

- **GIVEN** a buyer has saved a search with `filters_json` `{"in_stock":"true"}` and a query matching one sold-out and one in-stock listing
- **WHEN** the buyer runs the saved search through the gateway
- **THEN** the response returns the in-stock listing with its `stock` and not the sold-out one

#### Scenario: An invalid in_stock value is rejected

- **WHEN** a buyer calls `SearchListings` with `filters["in_stock"]="maybe"`, then `SaveSearch` with `filters_json` `{"in_stock":"yes"}`
- **THEN** both calls fail with `invalid_argument` and no saved search is created

### Requirement: Services can read live stock by listing id

`SearchService.GetListingStock` SHALL accept 1 to 10 non-empty listing ids and SHALL return one `ListingStock` per
requested id, in request order (a repeated id yields a repeated entry). Each entry's `stock` SHALL be the listing's
current projected base stock (net of reservations, no variant rollup), the same value `SearchHit.stock` returns, and SHALL be absent when the read-model holds no
document for the id, holds a tombstone, or holds no projected stock; a stock of zero SHALL be present as `0`. A request
with no ids, more than 10 ids or any empty id SHALL fail with `INVALID_ARGUMENT` before the index is read. The caller
SHALL be a principal of type `service` (go-sdk `auth.PrincipalType`, carried in `x-principal-type`) holding scope
`search:read`; scope alone is not enough because the gateway grants `search:read` to anonymous callers (anonymous callers
get `UNAUTHENTICATED`, user principals `PERMISSION_DENIED`). The RPC SHALL NOT be routed by `team-gateway`. The index read
SHALL have a server-side deadline of 200 ms; when the index cannot be read or the deadline passes the call SHALL fail
with `UNAVAILABLE` and SHALL NOT return partial entries. The call SHALL NOT log listing ids, only their count. A 10-id
call SHALL complete within 20 ms at p95 on the reference stack (`mainx` with the OpenSearch overlay).

#### Scenario: Stock is returned in request order

- **GIVEN** published listings A with stock 5, B with stock 0 and C with stock 12 in the read-model
- **WHEN** a service calls `GetListingStock` with `[C, A, B]`
- **THEN** the response has exactly three entries `[C: 12, A: 5, B: 0]`, with B's `stock` present as `0`

#### Scenario: A checkout is visible in the lookup

- **GIVEN** a published listing with stock 1 that appears in search
- **WHEN** a buyer checks out quantity 1 of it through the gateway
- **THEN** within 30 seconds `GetListingStock` for that listing returns `stock` 0

#### Scenario: An unknown listing has no stock

- **GIVEN** listing A with stock 5 and an id U that was never indexed
- **WHEN** a service calls `GetListingStock` with `[A, U]`
- **THEN** the response has two entries, A with `stock` 5 and U with `stock` absent

#### Scenario: A repeated id yields a repeated entry

- **GIVEN** listing A with stock 5
- **WHEN** a service calls `GetListingStock` with `[A, A]`
- **THEN** the response has two entries, both for A with `stock` 5
- **VERIFIED BY**: `team-search/internal/rpc/listing_stock_test.go` `TestGetListingStockRepeatedID`

#### Scenario: A deleted listing has no stock

- **GIVEN** a listing that was indexed with stock 5 and then deleted (tombstone in the read-model)
- **WHEN** a service calls `GetListingStock` with its id
- **THEN** the response has one entry for it with `stock` absent

#### Scenario: A document without projected stock has no stock

- **GIVEN** a document indexed before stock was projected and not replayed
- **WHEN** a service calls `GetListingStock` with its id
- **THEN** the entry's `stock` is absent, not `0`
- **VERIFIED BY**: `team-search/internal/rpc/listing_stock_test.go` `TestGetListingStockNoProjectedStock`, `team-search/internal/index/listing_stock_integration_test.go` `TestOpenSearchGetListingStockBatch`

#### Scenario: Ten ids is accepted, eleven is rejected

- **WHEN** a service calls `GetListingStock` with exactly 10 ids, then with 11 ids
- **THEN** the first call returns 10 entries and the second fails with `invalid_argument` without reading the index

#### Scenario: An empty or malformed request is rejected

- **WHEN** a service calls `GetListingStock` with no ids, then with `["A", ""]`
- **THEN** both calls fail with `invalid_argument`

#### Scenario: A user or anonymous caller is refused

- **WHEN** an anonymous caller, then a user principal with `search:read`, then a service principal without `search:read`, calls `GetListingStock` with one valid id
- **THEN** the first fails with `unauthenticated`, the second and third with `permission_denied`, and no response carries stock

#### Scenario: The gateway does not expose the lookup

- **WHEN** a client sends an HTTP request to the gateway for any path derived from `SearchService/GetListingStock`
- **THEN** the gateway answers HTTP `404` or `501` (unimplemented) (observed at the gateway's HTTP response), and `team-search` records no `GetListingStock` call

#### Scenario: An unreachable index fails the call

- **GIVEN** the search index is unreachable
- **WHEN** a service calls `GetListingStock` with one valid id
- **THEN** the call fails with `unavailable` and returns no entries

#### Scenario: A slow index fails the call within the deadline

- **GIVEN** the search index answers slower than 200 ms
- **WHEN** a service calls `GetListingStock` with one valid id
- **THEN** the call fails with `unavailable` within 400 ms and returns no entries

#### Scenario: Requested ids are not logged

- **WHEN** a service calls `GetListingStock` with ids `[A, B]`
- **THEN** the captured `team-search` log lines for that call contain the id count 2 and neither id value

#### Scenario: A 10-id lookup is fast

- **GIVEN** the `mainx` stack with the OpenSearch overlay and 10 indexed listings
- **WHEN** a service makes 200 sequential `GetListingStock` calls with those 10 ids
- **THEN** the p95 latency is at most 20 ms
