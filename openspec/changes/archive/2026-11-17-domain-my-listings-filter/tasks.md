## 1. Code track (team-domain, depends on seller-centre-contract generated stubs)

- [x] 1.1 Store unit tests first (red): `ListByOwner` with statuses, query, both, wildcard escape, total, cursor, clamp, on Postgres and `storetest` in-memory repo (scenarios: A single status returns only that status; Several statuses are OR-ed; LIKE wildcards match literally; Filtered pages cover the filtered set exactly once; Page size above the maximum is clamped; The DELETED status matches nothing)
- [x] 1.2 Migration `0017_listings_owner_status_idx` (up/down) creating `listings (seller_id, status, id)`; `migrations_test.go` still green (scenario: Filtered page latency at 2,000 listings)
- [x] 1.3 Extend the `ListingRepository.ListByOwner` signature with a filter struct; implement in `listing_pg.go` (parameterised, `ILIKE` with escaped metacharacters, filtered `count(*)`) and in `storetest/listing.go` (scenarios: Empty filter keeps today's behaviour; Query and status combine; Substring match ignores case)
- [x] 1.4 `ListingService.ListMine` passes the filter; handler `ListMyListings` maps proto statuses to stored strings, trims `query`, rejects UNSPECIFIED and `query` over 100 runes with INVALID_ARGUMENT, keeps `listing.write` and owner-from-principal (scenarios: An unspecified status is rejected; A query of exactly 100 runes is accepted, 101 is rejected; A blank query is ignored; A caller without the listing.write scope is denied; Another seller's listings are never returned)
- [x] 1.5 Handler test that the log output of a call with a `query` never contains it (scenario: The query text is not logged)
- [ ] 1.6 Gate through `gate_digest.py` with `GO_SDK_REQUIRE_DOCKER=1`; sloglint clean

## 2. E2E track (platform-e2e, API level through the gateway; data seeded through real services)

- [x] 2.1 Feature file `platform-e2e/tests/e2e/features/domain/my_listings_filter.feature` and steps: seed a seller with 6 mixed-status listings via gateway API; status, OR, query, combined, no-match, blank-query scenarios (scenarios: A single status returns only that status; Several statuses are OR-ed; Empty filter keeps today's behaviour; Substring match ignores case; Query and status combine; No match returns an empty page; A blank query is ignored)
- [x] 2.2 Error and permission steps: UNSPECIFIED status, 101-rune query, buyer without scope, second seller sees nothing (scenarios: An unspecified status is rejected; A query of exactly 100 runes is accepted, 101 is rejected; A caller without the listing.write scope is denied; Another seller's listings are never returned; The DELETED status matches nothing)
- [x] 2.3 Paging and scale: seed 300 listings, walk pages, clamp check, 2,000-listing p95 check, cross-filter cursor (scenarios: Filtered pages cover the filtered set exactly once; Page size above the maximum is clamped; Filtered page latency at 2,000 listings; A cursor from a different filter yields no foreign rows)
- [x] 2.4 Log-leak check against `team-domain-svc` logs (scenario: The query text is not logged; LIKE wildcards match literally)
- [x] 2.5 FEATURES.yaml entries for each requirement are added by the e2e-author (not in this change)
