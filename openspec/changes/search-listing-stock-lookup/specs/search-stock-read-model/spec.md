## ADDED Requirements

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
