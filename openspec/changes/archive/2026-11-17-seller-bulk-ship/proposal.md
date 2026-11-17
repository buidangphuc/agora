## Why

Q35 (PM review 2026-10-14, seller#10): a seller with many orders ships and prints one order at a time (`ShipOrderButton` per row, `PrintButton` on the detail page only). This slice adds multi-select on the seller order list, a bulk "Gửi hàng loạt" that moves up to 50 selected orders to SHIPPED with a per-order result list, and "In phiếu hàng loạt" that prints one packing slip per selected order. Bulk price/stock editing stays out of scope (next initiative).

initiative: seller-centre

## What changes

- `SellerOrdersTable` (`team-frontend-seller/src/features/seller/SellerOrdersTable.tsx`) gains a checkbox column for rows the seller can still ship (`isToShip`: PAID, or PENDING with COD), a select-all-on-page checkbox, and a selection bar with the count, "Gửi hàng loạt" and "In phiếu hàng loạt". The selection is client state of the current page and holds at most 50 orders; the 51st checkbox is refused with a message.
- "Gửi hàng loạt" opens a confirm modal listing the selected orders, each with an optional tracking-number input. Confirming calls the new server action `bulkShipOrdersAction` (`src/features/order/actions.ts`), which loops over the existing `OrderService.UpdateOrderStatus` (status SHIPPED, optional `tracking_number`) with at most 4 calls in flight and returns one result per order (`ok` or a Vietnamese reason mapped from the Connect code). No bulk RPC exists or is added (initiative decision): per-order ownership and saga rules stay in team-order.
- The modal then shows the per-order result list (success or reason). Rows that failed stay selected so the seller can retry; rows that shipped leave the selection. The action revalidates `/seller/orders` and `/seller` once at the end.
- Batch budget: the action stops starting new calls after 15 s; orders not started are reported "Chưa xử lý, hãy thử lại" (not attempted, never silently dropped).
- "In phiếu hàng loạt" opens the new route `/seller/orders/print?ids=<id>,<id>,...` (server component, `force-dynamic`, noindex). It reads each order with `OrderService.GetOrder` (at most 4 in flight), renders one packing slip per order separated by a page break, and offers a `PrintButton`. An order that cannot be read (PERMISSION_DENIED, NOT_FOUND, error) is listed in a screen-only notice ("Không in được: ...") and produces no slip. The slip reuses the content of the existing detail-page slip (recipient name, phone, address, items, tracking number).
- Helpers: `bulkShip.ts` (pure: id parsing, 50 cap, dedupe, concurrency runner, error mapping) and `BulkShipBar.tsx` / `BulkShipModal.tsx` client components; `src/app/seller/orders/print/page.tsx`.
- Tab: the bulk bar is shown on the order list whatever the tab, but only shippable rows carry a checkbox, so the "Chờ gửi" queue (shippable orders, tab defined by `seller-server-paging-ui`) is where it is used.

## Capabilities

### New Capabilities
- `seller-bulk-ship`: multi-select, bulk ship with per-order results, and batch packing-slip print on the seller order list.

### Modified Capabilities
None.

## Impact

`team-frontend-seller` only: `SellerOrdersTable.tsx`, `src/features/order/actions.ts`, `src/lib/gateway/orders.ts` (hot files shared with `seller-server-paging-ui`, which this slice depends on: rebase on its paged-list changes), `src/app/seller/orders/(list)/page.tsx` (only to mount the bar), new `bulkShip.ts`, `BulkShipBar.tsx`, `BulkShipModal.tsx`, `src/app/seller/orders/print/page.tsx`, tests; `FEATURES.yaml` entry (written by the e2e-author); `platform-e2e` feature. Consumes `OrderService.UpdateOrderStatus` and `OrderService.GetOrder` unchanged. No proto, gateway or backend change. No new image.

## Non-goals

A bulk RPC; bulk cancel, bulk return or bulk status other than SHIPPED; bulk price or stock editing (inline or CSV); selecting across pages or "select all N matching orders"; generating or inventing tracking numbers; carrier label or waybill generation; storing or logging the printed buyer data; changing order state rules (team-order stays authoritative).

## NFR

- Bulk ship of 50 orders: at most 4 `UpdateOrderStatus` calls in flight; whole batch settles within 15 s locally (the runner stops starting calls at 15 s and reports the rest as not attempted).
- Batch print of 50 orders: at most 4 `GetOrder` calls in flight; the print page renders p95 <= 3 s locally.
- Selection and modal interactions are client-side and instant; no list is refetched until the batch ends.
- Availability: a failing order fails only its own row; a down team-order fails the rows with a reason, never a blank screen. No `*_ENABLED` flag, no retry loop, no cached fallback.

## Data classification

- Buyer shipping name, phone and address (print route only): personal data already shown to the seller on order detail; purpose: fulfil and pack the order; basis: contract; retention: none in the seller app (rendered in the response, not stored, not cached: `force-dynamic`, no-store, noindex; not logged).
- Tracking numbers typed by the seller: shop operational data, sent to `UpdateOrderStatus`, not logged by the seller app.
- Order ids in the print URL: not personal data; the URL carries no buyer data.
- Seller and buyer ids are never logged raw (sloglint forbidden-keys); error mapping logs nothing from the order.

## Threat model

- Assets: buyer contact data on slips, order state (SHIPPED is hard to undo), the seller session.
- Abuse cases: (1) shipping or printing another seller's order by forging ids in the action input or the print URL: team-order remains the authority and answers PERMISSION_DENIED per order; the UI never decides ownership and shows only the reason; (2) a buyer or anonymous visitor calling the action or opening the print route: both check the principal and `listing.write` before any upstream call; (3) denial of service by a huge batch: the action and the print route refuse more than 50 distinct ids before any call and cap concurrency at 4; (4) PII leakage through caches, search engines or logs: no-store, noindex, no logging of slip data; (5) leaking upstream error text: reasons are mapped from the Connect code to fixed Vietnamese strings.
- Authz rule: seller scope `listing.write` checked in the seller app (coarse); ownership and state transitions enforced by team-order per `UpdateOrderStatus` / `GetOrder`.
