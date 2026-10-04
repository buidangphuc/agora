## 1. Code — team-frontend: data and actions

- [x] 1.1 Add `getOrderResult(id)` to `src/lib/gateway/orders.ts` returning `ok | forbidden | not_found | error` from the `ConnectError` code, keep `getOrder`; verify a Vitest file covers the four outcomes
- [x] 1.2 Convert `cancelOrderAction`, `reorderAction`, `createReturnRequestAction`, `mockRefundAction` to `{ ok, error?, data? }` with `revalidatePath` for the list and the detail; update `actions.test.ts` and `returns.actions.test.ts`; verify the tests pass
- [x] 1.3 Add a pure helper `paginateOrders(orders, status, page)` (filter, counts, clamp, slice 10) with tests for invalid status, out-of-range page and counts; verify the tests pass

## 2. Code — team-frontend: shared order components

- [x] 2.1 Add `OrderStatusBadge` on `Tag` (order and return statuses, tokens only, no emoji) and replace the four inline badge implementations; verify its tone-mapping and unknown-status tests pass
- [x] 2.2 Rebuild `OrderTimeline` on `Timeline` with the shipment header, newest-first checkpoints, saga fallback, failure checkpoint (`timeline-failure`) with error `Alert` and "Mua lại"; keep the existing testids; verify the existing `OrderTimeline.test.tsx` still passes untouched plus new failure/pending scenario tests
- [x] 2.3 Add `OrderActions` client island (Reorder, Cancel `Modal`, Return trigger) with pending, disabled and toast behaviour; verify unit tests for pending, success and failure of each action

## 3. Code — team-frontend: /account/orders

- [x] 3.1 Rewrite `app/account/orders/page.tsx` as a server component reading `searchParams` (`status`, `page`), using `paginateOrders`; keep the `/login` redirect; verify the page test renders tabs, rows and pagination from fixtures
- [x] 3.2 Add `OrderStatusTabs` leaf island (URL-held tab, resets `page`, horizontal scroll at 375px, count `Badge`) and server `Pagination` links; verify the tab-to-URL and invalid-query scenario tests pass
- [x] 3.3 Rewrite `BuyerOrdersList` as a server `OrderList` of `Card` rows with `Image` 1:1 thumbnails (lazy below the first card), `PriceTag`, row actions, `Empty` per tab and `Alert` with retry on load failure; keep `ReviewModal` unchanged; render the real shop display name on each row (depends on `shop-display-name`; "Shop #<6 chars>" only for an empty name); verify empty, error, lazy-image, real-name and empty-name-fallback tests pass
- [ ] 3.4 Add `app/account/orders/loading.tsx` and `error.tsx` with `Skeleton` blocks matching the footprint; verify a render test and a Lighthouse/Playwright CLS check on the route report 0 shift (render test done; the CLS check needs the running stack, left unticked)

## 4. Code — team-frontend: /account/orders/[id]

- [x] 4.1 Rewrite `app/account/orders/[id]/page.tsx` to use `getOrderResult`; render `Result 403`, `Result 404` or `Alert` retry, and skip shipment/saga fetches unless `ok`; verify tests for forbidden (no order fields rendered), not_found and error
- [x] 4.2 Rewrite `OrderDetailView` as server composition: header (Breadcrumb, title, `OrderStatusBadge`, `PriceTag`, actions), `Stepper` band (vertical at 375px) or cancelled `Alert`, `Descriptions`, items `Table`, amounts `Descriptions`; verify the detail render tests per status pass
- [x] 4.3 Add detail `Tabs` island with `?tab=timeline|returns`, `Suspense` + `Skeleton` around the timeline and returns; verify the tab-in-URL test passes
- [x] 4.4 Rebuild `ReturnRequestSection` (Descriptions + `OrderStatusBadge` + `Empty`) and add `ReturnRequestModal` (`FormItem`, `Select`, `Input`, validation, pending, toasts, mock refund); keep testids; verify the existing `ReturnRequestSection.test.tsx` passes and new validation/failure tests pass
- [x] 4.5 Replace the fake detail cancel modal (local state + `window.location.reload()`) with `cancelOrderAction` via `OrderActions`; verify a test asserts the action is called once and the badge updates after revalidation
- [x] 4.6 Add `app/account/orders/[id]/loading.tsx` and `not-found.tsx`; verify render tests and that no page under the order routes contains `"use client"`

## 5. Code — team-frontend: cross-cutting checks

- [x] 5.1 Run the token lint over `src/features/order/` and `src/app/account/orders/` (no `text-[..px]`, hex, `rounded-2xs`, emoji status icons); verify it is clean
- [x] 5.2 Confirm no tracking hook or `data-*` attribute was added, removed or changed (`git diff` over the order files for `Track`, `data-`); verify with a grep-based Vitest guard and `npm run check` plus `npx next build` pass

## 6. E2E — platform-e2e

- [x] 6.1 Add `team-frontend/FEATURES.yaml` entries (`status: planned`): `orders.list-status-tabs`, `orders.list-pagination`, `orders.list-empty-and-recovery`, `orders.detail-anatomy`, `orders.timeline-saga-failure`, `orders.rma-modal`, `orders.detail-forbidden`, `orders.mobile-layout`, each with `acceptance` lines mirroring the spec scenarios; verify `make -C platform-e2e features-check`
- [ ] 6.2 Extend `OrderDetailPage` (`src/pages/order_detail_page.py`) and the order steps in `tests/e2e/step_definitions/order_steps.py`: new labels for "Yêu cầu trả hàng" and the Modal fields, drop the `is_visible()` guards so the RMA scenario cannot pass vacuously; verify `buyer/order_tracking_and_rma.feature` and `order/rma_return.feature` stay green (code written: POM labels, guards removed, new `order_has_been_delivered` and `I open the order detail from the list` steps, feature extended; `pytest --collect-only` passes; NOT run green because the docker stack is not running, left unticked)
- [ ] 6.3 Add `OrdersListPage` page object (tabs, pagination, empty) and extend `buyer/order_history_tabs.feature` assertions only where it already covers the list; verify it stays green (`OrdersListPage` added and mapped to PageName.ACCOUNT_ORDERS; `order_history_tabs.feature` needed no change, its "Mã đơn:" assertion is kept; stays unticked until it is run against the stack)
- [ ] 6.4 Add `tests/e2e/features/frontend/orders_ui.feature` with scenarios: tab changes URL and list, reload keeps tab and page, empty tab recovery, saga failure checkpoint on a compensated order (reuse the `order/saga_compensation.feature` setup), RMA Modal validation and success toast, other buyer's order shows 403, 375px no horizontal scroll, tracking page view still fires after reorder; verify it runs green against the local stack (`orders_ui.feature` + `orders_ui_steps.py` + binder written, collect-only passes; not run, needs the stack, left unticked)
- [ ] 6.5 Flip the FEATURES.yaml entries to `status: automated` with `covered_by`; verify `make -C platform-e2e features-check` is green (blocked on 6.2-6.4 running green; entries are `status: planned`)
- [x] 6.6 Run `openspec validate ui-phase-orders --strict`; verify it is valid
