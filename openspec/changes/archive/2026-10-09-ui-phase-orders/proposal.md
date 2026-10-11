## Why

Phase 4 of the UI system (`UI_SYSTEM_DESIGN.md` §6) covers order fulfilment: `/account/orders` and
`/account/orders/[id]`. Today both pages are client-heavy and off-contract:

- `BuyerOrdersList` and `OrderDetailView` are whole-page `"use client"` components that hold the order list in
  `useState`, so the list, status and cancellation are not shareable or back/forward friendly (§5B).
- There are no status tabs and no pagination; the list is one unbounded column.
- Status badges are three hand-rolled variants (emoji + raw `bg-yellow-100`-style classes, `text-[11px]`);
  `ReturnRequestSection` has a fourth, with `text-[11px]` and `rounded-2xs`, breaking principles 1 and 3.
- Cancel uses `window.confirm()` and a hand-built modal; Return is an always-open inline form; neither
  follows the principle 2 feedback contract (pending, disabled, toast).
- `OrderTimeline` is a bespoke `<ol>`, shows only shipment checkpoints or saga steps, and does not surface the
  saga failure checkpoint clearly or offer a recovery action.
- `getOrder()` swallows every error and returns `null`, so an order that belongs to another user renders the
  generic 404 instead of a 403.
- The detail page's "Hủy đơn / Yêu cầu hoàn tiền (RMA)" modal calls no Server Action: its confirm button only
  sets local state and reloads the page, so it reports success without cancelling anything.
- No `loading.tsx`, no skeletons (CLS), no `Image` aspect on order item thumbnails.

This change rebuilds the two routes on the `ui-core-components` set, following Ant Design Pro's
Profile > Advanced Profile (detail), List > Table/Basic List (list), Result and Exception 403 templates.

## What Changes

- `/account/orders`: server-rendered list with status `Tabs` (All / Pending / Paid / Shipped / Completed /
  Cancelled) and `Pagination`, both held in `searchParams` (`?status=&page=`); order rows are `Card`s with an
  `OrderStatusBadge`, `Image` thumbnails and `PriceTag`; `Empty` per tab; `Alert` with retry on load failure;
  route `loading.tsx` with `Skeleton` rows.
- `/account/orders/[id]`: Advanced Profile anatomy: header (breadcrumb, order id, `OrderStatusBadge`, primary
  actions), a `Stepper` progress band, `Descriptions` blocks (recipient, payment, amounts), item `Table`,
  `Tabs` for Timeline and Returns, route `loading.tsx`.
- `OrderStatusBadge`: one `Tag`-based component mapping `OrderStatus` and `ReturnStatus` to tone and label,
  replacing the four inline badge implementations.
- `OrderTimeline`: rebuilt on `Timeline` (vertical) with the shipment checkpoints, and the saga fallback with an
  explicit failure checkpoint (FAILED / COMPENSATED) rendered as an error item, with an `Alert` and a
  recovery action.
- `ReturnRequestSection`: the RMA form moves into a `Modal` opened from the header action
  "Yêu cầu trả hàng"; the section body shows the existing return as `Descriptions` plus status badge.
- Cancel and Reorder become Server Actions with pending, disabled and toast feedback; cancel confirmation is a
  `Modal` (no `window.confirm`).
- Ownership: an order that belongs to another user renders `Result status="403"` (Exception 403); a missing
  order renders `Result status="404"`; a transport failure renders `Alert` with retry.

Repos: `team-frontend` (code) and `platform-e2e` (tests). No other repo changes.

## Capabilities

### New Capabilities
- `ui-orders`: the buyer order list and order detail screens, their components, states and behaviour.

### Modified Capabilities
None.

## Non-goals

- No backend, gateway or proto changes. In particular no pagination or filter fields are added to
  `ListBuyerOrders`; paging is done in the server component over the returned list.
- No `antd` dependency.
- No routes owned by another phase: checkout and payment (`/checkout`, `/checkout/pay/[id]`) stay with the
  cart phase; `/seller/orders` and `SellerOrdersList` stay with the seller phase; `/account/addresses` etc. with
  the account phase.
- `ListingCard`, `ListingGrid`, `SearchBar`, `FilterSidebar` and `SortBar` belong to ui-phase-discovery and are
  neither used nor changed here.
- No change to the order saga, return or refund business rules; `mockRefundAction` behaviour is kept.
- No change to `ReviewModal` (review phase); it is only opened from the item rows as today.

## Real data and dependencies

- Only real data is shown: order rows and the detail page render values from the order, shipment and saga
  responses; nothing is invented (no fake carrier, delivery estimate or shop stats). A block with no data is
  hidden or shows `Empty`.
- The shop shown on an order row uses the real shop display name. This requires the backend change
  `shop-display-name`; "Shop #<6 chars>" is only the fallback for an empty name.

## Impact

- `team-frontend/src/app/account/orders/page.tsx`, `loading.tsx`, `error.tsx` (new);
  `src/app/account/orders/[id]/page.tsx`, `loading.tsx`, `not-found.tsx` (new).
- `team-frontend/src/features/order/`: `BuyerOrdersList.tsx`, `OrderDetailView.tsx`, `OrderTimeline.tsx`,
  `ReturnRequestSection.tsx` rewritten; new `OrderStatusBadge.tsx`, `OrderStatusTabs.tsx`, `OrderActions.tsx`;
  `actions.ts` adjusted to the `{ ok, error?, data? }` shape (callers updated).
- `team-frontend/src/lib/gateway/orders.ts`: new `getOrderResult()` (discriminated result) alongside `getOrder`.
- `team-frontend/FEATURES.yaml`, `platform-e2e/tests/e2e/features/frontend/orders_ui.feature` and steps.
- Depends on `ui-core-components` (Tag, Timeline, Table, Image, Skeleton, Empty, Alert, Pagination, Breadcrumb,
  Modal, Result, FormItem, Select, ...). Existing e2e features
  `buyer/order_history_tabs`, `buyer/order_tracking_and_rma`, `order/order_timeline_view`, `order/rma_return`
  must stay green.
