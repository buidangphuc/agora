## Why

`ListingService.ListMyListings` (team-domain `internal/rpc/listing.go`) pages a seller's listings by id but cannot
filter. The seller app walks up to 10 pages and filters in Next.js (`getAllListings`) for `/seller`, the ads, bundle and
featured pickers. A seller with 2,000 listings therefore loads the whole catalogue per render. Initiative
`seller-centre` story 6 moves the filter into team-domain: a status filter and a title search, with the total for the
filtered set so the seller UI can show counts and page.

initiative: seller-centre

## What Changes

- `ListMyListings` honours the new `ListMyListingsRequest.statuses` (OR-filter, empty = all) and `query`
  (case-insensitive title substring, at most 100 runes), both added by `seller-centre-contract`.
- `PostgresListingRepository.ListByOwner` gains the filter; `PageResponse.total` counts the filtered set, so a
  caller sees the number of matches, not the seller's whole catalogue.
- New migration `0017_listings_owner_status_idx` adds index `listings (seller_id, status, id)`.
- A request without `statuses` and `query` behaves exactly as today (same order, cursor, total), so existing callers
  do not change.

## Capabilities

### New Capabilities
- `listing-my-listings-filter`: server-side status and title filtering of the seller's own listing list.

### Modified Capabilities
none

## Impact

- `team-domain/internal/rpc/listing.go`, `internal/listing/listing.go`, `internal/store/listing.go`,
  `internal/store/listing_pg.go`, `internal/store/storetest/listing.go`, `migrations/0017_*`, `FEATURES.yaml` (by the e2e-author).
- `platform-e2e` feature and steps for the API-level scenarios.
- Depends on `seller-centre-contract` (proto fields). No event, no new RPC, no gateway change (the gateway already forwards `ListMyListings`).

## Non-goals

- Sorting options, filtering by category or price, full-text or fuzzy search (substring only).
- Changing `ListListings` (buyer/admin) or the page-size defaults of existing callers.
- Seller app changes (`seller-server-paging-ui`, `seller-marketing-hub`, `seller-shop-customisation-ui`).

## Non-functional requirements

- Paged `ListMyListings` p95 at most 150 ms locally for a seller with 2,000 listings, page size 20, with status
  and/or query set (initiative NFR).
- Page size: unset uses the existing default (50), values above 100 are clamped to 100.
- No new availability dependency: the filter runs in the existing Postgres query; a database failure fails the call
  (no fallback, no flag).
- The `query` string is never logged.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `query` (search text) | seller-typed free text, may be incidental personal data | match own listing titles for this one call | contract (operating the shop) | request-scoped only: not stored, not logged, not in metrics labels or traces |
| `statuses` | non-personal enum | filter | n/a | request-scoped |
| listing rows returned | seller's own shop content (title, price, status), including non-public drafts and moderation states | seller center | contract | unchanged from today |

## Threat model

- Assets: the seller's draft, pending-review, rejected, held and banned listings (not public).
- Abuse cases: (1) a caller lists another seller's listings by guessing a filter; (2) a buyer or caller without
  `listing.write` reads the seller view; (3) `query` carries `%`/`_` wildcards or SQL to widen or break the match;
  (4) oversized `query` or page size to force a costly scan; (5) the search text leaks into logs.
- Authz rule: `listing.write` scope required; the owner is always the principal (`principalOwner`), never a request
  field, and every filtered query keeps `seller_id = principal`. Other sellers' rows are unreachable through any
  `statuses`/`query` combination.
- Mitigations: parameterised SQL with LIKE metacharacters escaped (wildcards match literally); `query` capped at 100
  runes and `page_size` clamped to 100; no logging of `query`.
