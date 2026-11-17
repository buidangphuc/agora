# Design

## Clarifications
- Contract fields are those of `plans/seller-centre/00-spec.md` Contracts (`ListSellerOrdersRequest.{page,query,statuses}`, `ListSellerOrdersResponse.{page,status_counts}`, `ListMyListingsRequest.{statuses,query}`). Not redefined here.
- `PageRequest` is cursor-only; there is no offset. A numeric page link cannot be built without walking cursors (what `getListingsPage` does today, one round trip per page: 250 calls for page 250 of 5,000 orders).

## Decisions
- URL carries the cursor of the page being shown (`?cursor=...&page=n`). "Trang sau" links to `next_cursor` and `page+1`; "Trang đầu" drops both. There is no "previous" link (browser back is previous); `page` is only a label. Why: one RPC per render at any depth.
- Cursor error -> INVALID_ARGUMENT (`Code.InvalidArgument`) -> the list shows the existing error Alert; its retry link is the URL without `cursor`/`page`.
- Tab counts come from `status_counts` of the same response (query applied, statuses ignored by the server). `ALL` = sum of the five counts. Tab "Chờ xử lý" = PENDING + PAID, as today.
- Dashboard KPI "Cần gửi" = `status_counts` PENDING + PAID. Today's `isToShip` (PAID, or PENDING with COD) cannot be computed server-side, so the number can differ for prepaid PENDING orders. The overdue note is derived from the first 100 rows of `statuses=[PENDING,PAID]` and shown only when > 0.
- Listing KPIs: totals from `page.total` of 1-row calls (all, `PUBLISHED`); low-stock stays an in-memory count over the 20-row product page, with the existing footnote.
- `ListingPicker`: client component, input + listbox, 300 ms debounce, calls server action `searchMyListingsAction(query)`, which wraps `searchMyListings`. Empty query shows the first 20 `PUBLISHED` listings; the ads and bundle pickers pass `statuses=[PUBLISHED]`. Selected ids live in the parent form state (unchanged), so a selection survives a new search.
- Wrapper stays server-only (`import "server-only"`); the view model keeps bigint -> number mapping.

## Risks
- Hot files shared with other slices (`listingsPage.ts`, dashboard `page.tsx`, orders `page.tsx`, `orders.ts`): this slice is serialised by initiative-build; it keeps `listSellerOrders` and `listMyListings` exports source-compatible (new optional args) so later slices compile.
