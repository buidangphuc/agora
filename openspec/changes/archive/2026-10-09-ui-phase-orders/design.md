## Context

Current code (read from `team-frontend`):

| Piece | Today | Kind |
|---|---|---|
| `app/account/orders/page.tsx` | `force-dynamic`, `listBuyerOrders()`, passes `initialOrders` to `BuyerOrdersList` | server |
| `BuyerOrdersList` | whole list in `"use client"`, `useState` orders, `window.confirm`, inline badge fns, `ReviewModal` | client (too large) |
| `app/account/orders/[id]/page.tsx` | `getOrder` (null on any error, then `notFound()`), `getShipmentTracking`, `getSagaState` in parallel | server |
| `OrderDetailView` | whole view `"use client"`, hand-built stepper (5 steps), hand-built cancel modal | client (too large) |
| `OrderTimeline` | server-compatible; checkpoints, else saga steps, else empty; `data-testid` order-timeline, timeline-checkpoint, timeline-saga, timeline-saga-step, timeline-empty | server |
| `OrderDetailView` cancel modal | select of reasons; confirm sets `cancelDone` and `window.location.reload()` without calling an action (e2e POM `order_detail_page.py` clicks "Hủy đơn / Yêu cầu hoàn tiền" and "Xác nhận gửi yêu cầu") | client, fake |
| `ReturnRequestSection` | `"use client"`, inline form, own `StatusBadge`; testids return-section, return-reason, return-amount, return-submit, return-status | client |
| `actions.ts` | `cancelOrderAction`, `reorderAction`, `createReturnRequestAction`, `mockRefundAction` (result shapes differ per action: `OrderActionResult`, `ReturnActionResult`) | server actions |

Tracking: the order routes contain no `TrackLink`, `TrackImpression`, `SearchImpressions` or
recommendation placement attributes today. Only the app-level `AnalyticsProvider` page view applies, and
`getImageUrl()` is used for thumbnails. This change must not add or remove any of these, and must keep
every existing `data-testid` / `data-*` attribute (the e2e features and tracking assertions depend on them).

## Goals / Non-Goals

Goals: both routes server-first with client leaf islands; list state in the URL; one status badge; a clear
failure checkpoint in the timeline; RMA in a Modal; 403/404/empty/error states; CLS = 0; usable at 375px.
Non-goals: backend pagination, new RPCs, new order states, seller views.

## Decisions

1. **Ant Design / Ant Design Pro mapping**

   | Ant Design (Pro) template / component | agora route or component |
   |---|---|
   | Pro List > Basic List / Table List | `/account/orders`: server `OrderList` of `Card` rows (mobile) with `Table` columns at desktop |
   | `Tabs` (status filter) | `OrderStatusTabs` leaf island, active tab = `?status=` |
   | `Pagination` | `/account/orders` footer, `?page=`, server-compatible link variant |
   | `Tag` | `OrderStatusBadge` |
   | `Empty` | per-tab empty state of the list; empty items table |
   | `Skeleton` | `loading.tsx` of both routes, Suspense fallbacks for timeline and returns |
   | `Alert` | list load failure (retry), saga failure banner, cancelled notice |
   | Pro Profile > Advanced Profile | `/account/orders/[id]` page anatomy |
   | PageHeader (title, tags, extra actions) | detail header: Breadcrumb, `Order #id`, `OrderStatusBadge`, action Buttons |
   | `Steps` | `Stepper` progress band (Đã đặt, Đã thanh toán, Đóng gói, Vận chuyển, Đã nhận) |
   | `Descriptions` | order detail blocks: recipient, payment, amounts; return details |
   | `Table` | order items (thumbnail, title/variant, unit price, qty, subtotal) |
   | `Timeline` | `OrderTimeline` (vertical; shipment checkpoints, saga fallback, failure checkpoint) |
   | `Modal` | `ReturnRequestSection` RMA form; cancel confirmation |
   | `Form.Item` | `FormItem` around the reason `Select`/textarea and refund amount `Input` |
   | `message` | `ToastProvider` success/error toasts for cancel, reorder, return, refund |
   | `Result` 403 / 404 / 500 | Exception pages inside the route: other user's order, missing order |
   | `Image` | item thumbnails, fixed 1:1 box |
   | `Statistic` / `PriceTag` | order total in header (`PriceTag`), amounts in `Descriptions` |
   | `Breadcrumb` | detail header: Tài khoản > Đơn hàng của tôi > Chi tiết #id |

2. **List state in the URL (§5B).** `/account/orders?status=<all|pending|paid|shipped|completed|cancelled>&page=<n>`.
   The server component reads `searchParams`, calls `listBuyerOrders()` once (the existing RPC, no status
   filter), derives per-tab counts, filters by status, and slices 10 per page. Invalid `status` falls back to
   `all`; an out-of-range `page` clamps to the last page. `OrderStatusTabs` is the only client code on the list
   chrome (it calls `router.replace` with the new params and resets `page`). Pagination renders as links.

3. **Page anatomy: list (Ant Pro Basic List).**
   - Header: title "Đơn hàng của tôi" (20px), subtitle with total count (14px).
   - Toolbar: `OrderStatusTabs` with count `Badge` per tab.
   - Body: for each order a `Card` (shop display name from `shop-display-name`, fallback "Shop #<6 chars of sellerId>" for an empty name, `OrderStatusBadge`, item rows with `Image` 1:1 64px +
     title + variant + qty + `PriceTag`, footer with order total and actions). At >= 768px item rows use
     a `Table` layout inside the card; at 375px they stack.
   - Footer: `Pagination`.
   - Actions per row: "Xem chi tiết" (link, secondary), "Mua lại" (Server Action button), "Hủy đơn" (opens
     cancel `Modal`, only for statuses where cancel is offered today), "Đánh giá" (opens the existing
     `ReviewModal`, unchanged).

4. **Page anatomy: detail (Ant Pro Advanced Profile).**
   - Header: `Breadcrumb`; line with `Order #<id8>` (24px), `OrderStatusBadge`; `PriceTag` total on the right;
     action row: primary "Mua lại" (brand), secondary "Yêu cầu trả hàng" (opens RMA `Modal`, shown for
     COMPLETED), outline "Hủy đơn" (opens cancel `Modal`, shown where cancel is offered today).
   - Progress band: horizontal `Stepper` (5 steps, current step from `OrderStatus`). When CANCELLED the
     stepper is replaced by an `Alert` "Đơn hàng đã hủy".
   - Body: `Descriptions` (recipient, phone, address, payment method, voucher), then the item `Table`, then
     an amounts `Descriptions` (subtotal, shipping, discount, total with `PriceTag`).
   - `Tabs` (client island, state in `?tab=timeline|returns`, default `timeline`): "Hành trình" renders
     `OrderTimeline`; "Trả hàng / Hoàn tiền" renders `ReturnRequestSection` body.
   - Section order matches the existing page (details, timeline, returns); the e2e step "order summary
     shows the 5-step delivery timeline and items" keeps passing.

5. **`OrderStatusBadge`** (`Tag`). Single mapping table, tokens only, no emoji:
   PENDING = warning tone, PAID = info, SHIPPED = info, COMPLETED = success, CANCELLED = neutral;
   return statuses PENDING = warning, APPROVED = info, REJECTED = danger, REFUNDED = success. Brand colour is
   not used for statuses (principle 1); it stays on primary CTAs and prices. The label is `statusText` from
   the view model; unknown statuses render neutral.

6. **`OrderTimeline` on `Timeline`.** Server-compatible. Source precedence is unchanged: shipment
   checkpoints if present, else saga steps, else `Empty`. Newest checkpoint first and marked current.
   Saga fallback maps step `status`: SUCCESS = success dot, PENDING = pending (animated only via
   `motion-safe`), FAILED / COMPENSATED = error dot. A FAILED or COMPENSATED step is the **failure
   checkpoint**: its item shows the step name, `detail` and timestamp in error tone, and the timeline header
   renders an `Alert type="error"` ("Thanh toán/giữ hàng thất bại, đơn đã được hoàn tác" using the detail)
   with the recovery action "Mua lại" (re-adds items to the cart) and a link to support is out of scope.
   Existing testids are kept: `order-timeline`, `timeline-checkpoint`, `timeline-saga`,
   `timeline-saga-step`, `timeline-empty`; a failure item additionally carries
   `data-testid="timeline-failure"`.

7. **`ReturnRequestSection` with `Modal`.** Section body (server-compatible view of `initialReturn`):
   `Descriptions` (reason, refund amount) + `OrderStatusBadge`, or `Empty` ("Chưa có yêu cầu trả hàng") when
   none. The form lives in a `ReturnRequestModal` client island opened by the header button: `FormItem` +
   `Select` reason (required), `FormItem` + `Input` refund amount (number, min 0, max order total, default
   order total), footer `Button` cancel + primary submit. Validation: reason required; amount 0 < x <= total,
   shown inline via `FormItem` status. Submit calls `createReturnRequestAction`; while pending the submit
   button shows `isLoading`, both buttons and fields are disabled and the Modal cannot be dismissed; success
   closes the Modal, toasts, and `revalidatePath` refreshes the page; failure keeps the Modal open with a
   toast error. The buyer section has no refund control by design: the refund is a seller action
   (`SellerReturns`, "Hoàn tiền" + confirm Modal, `refundReturnAction`), so the former "mock refund" scenario
   was removed from the spec (e2e "buyer sees the return status without a refund button"). Existing testids return-reason, return-amount, return-submit move into the
   Modal unchanged; return-section and return-status stay on the section.

8. **Server Actions contract (§5B).** `cancelOrderAction`, `reorderAction`, `createReturnRequestAction`,
   `mockRefundAction` return `{ ok, error?, data? }` and call `revalidatePath("/account/orders")` and
   `revalidatePath("/account/orders/" + id)`. The old fields (`message`, `returnRequest`) are replaced by
   `error` and `data`; call sites and `actions.test.ts` / `returns.actions.test.ts` are updated in the same
   change. Local `useState` mirrors of server data (the `orders` array, `ret`) are removed; the UI re-renders
   from the revalidated server data.

9. **Principle 2 feedback matrix.**

   | Action | Pending | Disabled while | Success | Error |
   |---|---|---|---|---|
   | Cancel order | confirm button `isLoading` | the order's other actions, Modal close | toast.success, badge becomes Đã hủy | toast.error, Modal stays open |
   | Reorder | button `isLoading`, label "Đang thêm..." | that button | toast.success then navigate `/cart` | toast.error |
   | Submit return | submit `isLoading` | form fields, close | toast.success, Modal closes | toast.error, Modal stays |
   | Mock refund | button `isLoading` | button | toast.success | toast.error |

10. **403 / 404 / error states.** New `getOrderResult(id)` in `lib/gateway/orders.ts` returns
    `{ kind: "ok", order } | { kind: "forbidden" } | { kind: "not_found" } | { kind: "error" }` by inspecting
    the `ConnectError` code (PermissionDenied = forbidden, NotFound = not_found, else error). `getOrder` stays
    for other callers. The detail page renders `Result status="403"` ("Bạn không có quyền xem đơn hàng này",
    action "Về đơn hàng của tôi") for forbidden, `Result status="404"` with action for not_found, and an
    `Alert` with a retry link for error. No order data from the other user is rendered or fetched
    (shipment and saga are not requested). Unauthenticated users still redirect to `/login`.

11. **CLS = 0.**
    - Both routes get `loading.tsx` with `Skeleton` blocks matching the footprint (list: tabs bar + 3 card
      skeletons; detail: header, stepper, descriptions, table).
    - `OrderTimeline` and the returns tab are wrapped in `Suspense` with a `Skeleton` fallback of the same
      min-height, since shipment and saga are fetched in parallel after the order.
    - Item thumbnails use `Image` with a fixed 1:1 box and `getImageUrl()` fallback; the first order card's
      images are eager, images in later cards and in the items table below the fold use `loading="lazy"`.
    - Tab content, status badge and buttons reserve their size; the pending button preserves width.

12. **Type scale and colour (principle 1).** Page title 24, section titles 20 (headers) / 16 (card titles),
    body 14, meta/secondary 12; no `text-[11px]` or arbitrary values. Brand colour only on primary CTAs
    ("Mua lại", RMA submit), `PriceTag` totals and the count badge on the active tab; statuses use the
    semantic tones in decision 5.

13. **Responsive.**
    - 375px: single column; tabs scroll horizontally (no wrap, no page overflow); order rows stack with
      thumbnail left; item `Table` collapses to stacked rows; header actions wrap into a full-width button
      row with the primary action first; Stepper is vertical; RMA `Modal` is full width with a sticky footer.
    - >= 1024px: content max width 960px; item `Table` shows columns; Stepper horizontal; action row inline
      at the right of the header; RMA `Modal` 480px.

14. **Tracking hooks unchanged.** No `TrackLink`, `TrackImpression`, `SearchImpressions` or placement
    attributes exist on these routes and none are introduced or removed; the layout-level
    `AnalyticsProvider` page view and the cart events fired after Reorder (`/cart` navigation) are
    untouched. All `data-testid` values listed above are preserved.

## Risks / Trade-offs

- Slicing in the server component loads the whole order list per request. Acceptable at current volumes;
  revisit when `ListBuyerOrders` gains paging (separate backend change).
- `getOrderResult` relies on the order service returning PermissionDenied for another user's order. If it
  returns NotFound, the page shows 404 instead of 403 (see Open Questions).
- Replacing result fields on the four Server Actions touches their tests; done in the same change.

## Open Questions

0. Decided: order rows show the real shop display name (backend change `shop-display-name`; this change
   depends on it) and never invent data; blocks without real data are hidden.

1. Does `team-order`'s `GetOrder` return `PermissionDenied` or `NotFound` for an order owned by another
   buyer? If NotFound (to avoid leaking existence), the 403 scenario can only be covered at unit level, and
   product must decide whether 404 is the desired UX instead.
2. Which statuses currently allow Cancel, and which statuses should show "Yêu cầu trả hàng"? The spec assumes
   cancel stays as today and RMA shows for COMPLETED only (the existing `order.rma-return` feature uses a
   "delivered, paid" order); confirm with the order owner.
3. Tab counts require one unfiltered `listBuyerOrders()` call; acceptable, or should counts be dropped?
4. Should the status tabs work without JS (plain links with `?status=`)? The core `Tabs` is a client island;
   a link variant would be a `ui-core-components` addition.
5. The e2e POM `OrderDetailPage` and step `submit_rma_refund_request` are guarded with `is_visible()` and match the old
   labels. The e2e track updates the POM to the new labels and removes the guard so the scenario cannot pass
   vacuously; confirm that tightening is wanted.
6. Is the reason `Select` option list (changed_mind, defective, wrong_item, other) the right set? The current
   free-text reason is kept in a textarea shown when "other" is chosen.

## E2E coverage of the failure-path scenarios

Failure paths are verified through the real stack, never faked: a `@destructive` browser scenario stops (or `docker pause`s, for
"slow") the `agora` compose-project container behind the read and restores it in teardown after the gateway answers again
(`platform-e2e/tests/e2e/support/uif_support.py`; serial lane only). A scenario that cannot be produced through the edge
without fault-injection code in the product is verified by a named Vitest test instead: its delta-spec scenario carries a
`**VERIFIED BY**` line (file + test name) and its FEATURES.yaml entry is `status: not-testable`, the repo's existing exclusion
status. `platform-e2e/scripts/spec_sync.py` does not read that status, so these scenarios still print as uncovered there.

- Real outage (A), `frontend/uif_orders.feature` (team-order stopped): "A failed load shows an Alert with retry" and "A transport failure
  offers retry". The retry is a same-URL `Link`, which Next serves from its 30 s router cache, so the step retries until the page
  recovers (a click right after the outage keeps the cached error).
- Not through the edge (B), each verified by a named Vitest test: "An unknown status does not break the row"
  (`features/order/OrderStatusBadge.test.tsx` › "renders an unknown status as a neutral tag with its text"), "Shipment checkpoints are
  listed newest first" (`features/order/OrderTimeline.test.tsx` › "lists checkpoints newest first and marks the newest as current",
  two checkpoints), "No tracking information shows Empty" (same file › "shows an empty state when there is neither shipment nor
  saga"), "Actions return the standard shape and revalidate" (`features/order/actions.test.ts` and `returns.actions.test.ts`).
- Spec drift (fixed 2026-10-09: the requirement now names the three buyer actions and points refunds to the seller's `refundReturnAction`): the requirement named four actions including `mockRefundAction`, which no longer exists in
  `features/order/actions.ts`; three are unit-tested.
