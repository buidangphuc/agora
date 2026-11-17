## Why

Q35 (PM review 2026-10-14), stories 5 and 6 of initiative `seller-centre`. `/seller/orders` and `/seller` load every order with `ListSellerOrders` and filter, count and slice in Next.js (`ordersList.ts`, `slicePage`). `/seller` pages listings by walking cursors and filters the page in memory (`filterListings`). The ads and bundle pickers call `getAllListings` (up to 10 pages, 200 rows). A shop with 5,000 orders or 2,000 listings therefore pays a full catalogue read per render. `order-seller-list-paging` and `domain-my-listings-filter` now filter, search, count and page in the owning services. This slice moves the seller app onto those fields.

initiative: seller-centre

## What changes

- `src/lib/gateway/orders.ts`: new `listSellerOrdersPage({ statuses, query, cursor, pageSize })` returning `{ orders, nextCursor, total, statusCounts }`. The existing `listSellerOrders(status, opts)` stays for its other callers.
- `src/lib/gateway/listings.ts`: `listMyListings` accepts `statuses` and `query` and sends them.
- `/seller/orders`: tab -> `statuses` (pending = PENDING+PAID, shipped, completed, all = none), search box -> `query`, tab badges -> `status_counts`, rows = one page of 20. One RPC per render. `ordersList.ts` loses `inTab`, `tabCounts`, `searchOrders`; it keeps the tab -> status map.
- `/seller`: product list calls `ListMyListings` with the URL status and `q` (page size 20); KPI "Tổng sản phẩm" uses `page.total` of an unfiltered 1-row call, "Đang bán" uses `page.total` of a `PUBLISHED` 1-row call. "Sắp hết hàng" stays computed on the first 20 rows (no server field; note kept). Recent orders = one `ListSellerOrdersPage` call of 5 rows; "Cần gửi" = `status_counts` PENDING + PAID.
- Paging in the URL moves from `?page=N` to a cursor: `?cursor=<opaque>&page=<n>` where `n` is a display number only. `SellerPagination` gets a cursor mode ("Trang đầu", "Trang sau", "Trang N", total text); numeric mode stays for other callers. Changing tab or filter drops `cursor` and `page`.
- `listingsPage.ts`: `getListingsPage` (cursor walking) and `getAllListings` (10-page walk) are removed. New `searchMyListings({ query, statuses, pageSize })` returns at most 20 options in one call.
- New shared client component `ListingPicker` (debounced search over `searchMyListings` through a server action `searchMyListingsAction`). `/seller/ads` and `/seller/bundles` use it instead of preloaded option arrays. Other slices (`seller-shop-customisation-ui`, `seller-marketing-hub`) reuse it.
- Search placeholder of `/seller/orders` changes to "Mã đơn hoặc tên sản phẩm" because the server searches only order id prefix and item title.

## Capabilities

### New Capabilities
- `seller-server-paging-ui`: server-side paged, filtered orders and listings and searchable pickers in the seller app.

### Modified Capabilities
None.

## Impact

`team-frontend-seller` only: `src/lib/gateway/orders.ts`, `listings.ts`, `src/features/seller/{ordersList,listParams,listingsPage,ProductList,SellerPagination,RecentOrders,kpi,AdCampaignForm,BundleManager,actions}.ts(x)`, new `ListingPicker.tsx`, pages `src/app/seller/orders/(list)/page.tsx`, `src/app/seller/(dashboard)/page.tsx` (hot, shared with `seller-getting-started`), `ads/page.tsx`, `bundles/page.tsx`, their tests; `FEATURES.yaml` (written by the e2e-author); `platform-e2e` feature. Consumes `OrderService.ListSellerOrders` and `ListingService.ListMyListings` with the fields added by `seller-centre-contract`. No proto, backend or gateway change. `seller-bulk-ship` builds on the paged orders list.

## Non-goals

Bulk ship, batch print, bulk price/stock editing; sorting options; searching by recipient name or phone (server does not index them); numeric page-jump links; changes to order detail, returns or analytics pages; any proto or backend change.

## NFR

- `/seller/orders` server render p95 <= 1 s at 5,000 orders locally; it issues exactly one `ListSellerOrders` call with `page_size` 20 and never an unpaged call.
- `/seller` issues bounded calls: at most 1 product page, 2 count calls, 1 recent-orders call, 1 to-ship call, all in parallel; no cursor walking.
- Picker search returns within 300 ms p95 locally at 2,000 listings, at most 20 rows, debounced 300 ms, query <= 100 chars.
- Availability: a failing source degrades only its own block (error state with retry link); the page still answers HTTP 200. No `*_ENABLED` flag, no cached or in-memory fallback.

## Data classification

- Order rows (recipient name, phone, address) are already shown to the seller on the list/detail; this slice shows a subset per page, does not store, cache or log them.
- Search `q` (orders and listings): seller-typed text, held only in the URL and the request; never logged by the seller app, never persisted. Purpose: filter the seller's own data; basis: contract; retention: none.
- Cursors are opaque, short-lived URL values, not personal data.
- Seller id from the session principal is never logged raw nor rendered.

## Threat model

- Assets: the seller's orders (buyer PII) and listings.
- Abuse: (1) a seller edits `cursor` to read another seller's page - the cursor is resolved by the owning service under the caller's principal, so a foreign or malformed cursor yields INVALID_ARGUMENT and never foreign rows; (2) oversized `q` or `page_size` to load the DB - `q` is cut to 100 chars in `parseListParams`, page size is fixed at 20 (picker 20, dashboard 5/1), servers clamp to 100; (3) unauthenticated or non-seller access to the pages - guarded by `redirectGuestToLogin` and `hasScope("listing.write")` as today; (4) picker server action called without session - answers an error, no data.
- Authz rule: ownership is enforced only by team-order and team-domain from the principal; the UI sends no seller id.
