## Why

`OrderService.ListSellerOrders` (team-order `internal/rpc/order.go`, `internal/store/order.go`) returns every order of the
seller with only a `status_filter`; the seller app loads them all and filters in Next.js. A seller with 5,000 orders loads
the whole history on each render of `/seller/orders` and `/seller`. Initiative `seller-centre` story 5 moves paging,
search and tab counts into team-order, using the optional fields added by `seller-centre-contract`.

initiative: seller-centre

## What Changes

- `ListSellerOrders` honours `page` (keyset cursor over `created_at DESC, id DESC`), `query` (order id prefix or item
  title substring, at most 100 runes) and `statuses` (OR-filter, wins over `status_filter`).
- The response carries `page` (`next_cursor`, `total` of the filtered set) and `status_counts` (one entry per real status,
  over all the seller's orders with `query` applied and `statuses` ignored, so tab counts stay stable).
- An unset `page` keeps today's full-list behaviour (same order, same orders); `page` and `status_counts` are still filled.
- New migration `0026_orders_seller_status_created_idx` adds index `orders (seller_id, status, created_at DESC, id DESC)`.
- Repository `ListSellerOrders` gains a filter struct, a paged variant and a counts query; fake repo in `internal/store/fake` mirrors them.

## Capabilities

### New Capabilities
- `order-seller-list-paging`: server-side paging, search and status counts for the seller's own order list.

### Modified Capabilities
none

## Impact

- `team-order/internal/rpc/order.go`, `internal/order/order.go`, `internal/store/order.go`, `internal/store/fake/order.go`,
  `migrations/0026_*`, `FEATURES.yaml` (by the e2e-author).
- `platform-e2e` feature and steps (API level through the gateway; the gateway already forwards `ListSellerOrders`).
- Depends on `seller-centre-contract` (proto fields). No new RPC, no event. Hot file `internal/store/order.go` is shared with
  `order-bundle-checkout`; initiative-build serialises them.

## Non-goals

- Sorting options, date-range or amount filters, full-text or fuzzy search (prefix and substring only).
- Buyer-side order lists (`ListOrders`), admin lists, and changing the page-size defaults of any other RPC.
- Seller app changes (`seller-server-paging-ui`, `seller-bulk-ship`).

## Non-functional requirements

- Paged `ListSellerOrders` p95 at most 150 ms locally for a seller with 5,000 orders at page size 20, with a status tab and/or
  query set, including `status_counts` (initiative NFR).
- Page size: unset uses 20, values above 100 are clamped to 100, negative is INVALID_ARGUMENT.
- No new availability dependency: filter and counts run in the existing Postgres; a database failure fails the call (no
  fallback, no flag).
- `query` is never logged.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `query` (search text) | seller-typed free text, may contain a buyer-visible item title or order id | match the seller's own orders for this call | contract (operating the shop) | request-scoped only: not stored, not logged, not in metric labels or traces |
| `statuses`, `status_counts` | non-personal enum and aggregates | filter, tab badges | n/a | request-scoped |
| order rows returned | buyer shipping name/address/phone already in `Order` for the owning seller | fulfilment | contract | unchanged from today |

## Threat model

- Assets: other sellers' orders and the buyer shipping data inside every order.
- Abuse cases: (1) a seller reads another seller's orders through any `query`/`statuses`/cursor combination; (2) a forged or
  foreign cursor to skip or leak rows; (3) `%`/`_`/`\` in `query` to widen the match or inject SQL; (4) oversized `query` or
  page size to force a costly scan; (5) `query` leaks into logs; (6) an unauthenticated caller.
- Authz rule: authenticated principal required (as today); the seller is always the principal id, never a request field, and
  every query, count and cursor continuation keeps `seller_id = principal`.
- Mitigations: parameterised SQL, LIKE metacharacters escaped, `query` capped at 100 runes, page size clamped, cursor is
  opaque and only a position (never trusted for ownership), no logging of `query`.
