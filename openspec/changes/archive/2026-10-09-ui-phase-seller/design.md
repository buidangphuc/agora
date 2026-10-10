## Context

The seller cockpit lives under `team-frontend/src/app/seller/**` and `/sell`, all behind the `listing.write`
scope. Current structure (verified in code):

| Route | Today |
|---|---|
| `seller/layout.tsx` (server) | scope gate + static sidebar (`Card` + emoji links), no active state, no collapse |
| `/seller` (server, `force-dynamic`) | header card, 3 metric cards (third is hard-coded `0`, to be removed), raw `<table>`, `DeleteListingButton` (client, `confirm()`), first 20 listings only |
| `/seller/new`, `/seller/[id]/edit` (server) | header + `ListingForm` (client, `useFormState`, DOM-driven Magic fill) |
| `/seller/orders` (server) | `SellerOrdersList` (client; tab state in `useState`, optimistic list in state, toasts) |
| `/seller/orders/[id]` (**client**) | hard-coded mock order; `window.print()`; fake ship handler |
| `/seller/analytics` (server) | `SellerFunnelPanel` (real) + `SellerAnalyticsMock` (mock, client) |
| `/seller/wallet`, `/plans` (server) | hand-built cards + `WalletPayoutButton` / `SubscribeButton` (client, `useTransition`) |
| `/seller/ads`, `/bundles` (server) | `AdCampaignForm`, `BundleManager` (client, `useTransition`) |
| `/sell` | `redirect("/seller/new")` |

None of the seller routes uses `TrackLink`, `TrackImpression`, `SearchImpressions`, `AnalyticsProvider` or
recommendation attribution; they only read analytics (`getSellerFunnel`, `getRevenueBreakdown`) that those
hooks produce elsewhere. The product-name links to `/listing/[id]` and the public-shop link are plain `Link`s
and stay plain links.

Existing e2e (`platform-e2e/tests/e2e/features/seller/`): `create-listing`, `edit-listing`, `listing_delete`,
`seller_order_management` ("order status tabs"), `seller_analytics` ("revenue metrics summary"),
`fulfillment_and_payout`. FEATURES.yaml entries live in the owning services (team-domain, team-order,
team-analytics, team-payment) and reference these files. This change extends the files and adds UI-owned
entries to `team-frontend/FEATURES.yaml`.

## Goals / Non-Goals

Goals: one seller shell and page anatomy per Ant Pro template; server-first pages with URL state; every
mutation a Server Action with a pending/disabled/toast contract; CLS 0; explicit 375px behaviour.
Non-goals: see proposal.

## Decisions

1. **Ant Design / Ant Design Pro to agora mapping**

   | Ant Design (Pro) template / component | agora route or component |
   |---|---|
   | Pro Layout (collapsible `Sider`, mobile `Drawer`) | `seller/layout.tsx` + `SellerSidebar` (client island) + `SellerNavDrawer` |
   | `Breadcrumb` (PageContainer) | `SellerPageHeader` (server) on every seller page |
   | Dashboard › Workplace | `/seller`: `Statistic` KPI row, quick actions `Card`, recent orders `Table`, product table below |
   | Dashboard › Analysis | `/seller/analytics`: `Statistic` row, funnel as `Progress` rows, revenue-by-day `Table`, range `Tabs` |
   | List › Table List | product table on `/seller`, order table on `/seller/orders`: filter bar (URL), `Table`, `Pagination`, row actions, delete `Modal` |
   | Form › Basic Form | `/seller/new`, `/seller/[id]/edit` (`ListingForm` single-column `Card` sections, `FormItem` validation) |
   | Form › Advanced Form | same form, variant editor and media section as repeatable `Card` blocks; sticky action bar |
   | Profile › Advanced Profile | `/seller/orders/[id]`: header `Descriptions` + status `Statistic`, `Stepper`, `Tabs` (items / shipment), `Timeline` |
   | `Statistic` | KPI cells on `/seller`, `/seller/analytics`, wallet balance |
   | `Table` | products, orders, order items, ledger entries, revenue by day, campaigns |
   | `Tabs` | order status (URL), analytics range (URL), order-detail panels |
   | `Pagination` | products, orders, ledger (links, URL `?page=`) |
   | `Modal` | delete listing, confirm payout, confirm ship |
   | `Drawer` | mobile seller navigation |
   | `Result` | no seller scope (403), listing not owned, route error, submit success |
   | `Empty`, `Alert`, `Skeleton`, `Spin` | empty tables, inline error with retry, `loading.tsx`, pending inline |
   | `Progress` | funnel step ratios, ad budget used |
   | `Image` | product thumbnail (1:1), listing media grid (1:1) |
   | `Badge` / `Tag` / `Avatar` | order and listing status, plan tier, shop card |
   | `ToastProvider` (Ant `message`) | success/error feedback for every mutation |
   | `Stepper` / `Timeline` | fulfilment steps and shipment checkpoints |
   | `FormItem`, `Input`, `Select`, `Radio`, `Checkbox`, `QuantityPicker` | listing studio, ad campaign, bundle, payout amount |

2. **Server-first split.** Pages, `SellerPageHeader`, KPI rows, tables (rendered as server `Table` with link
   cells), `Pagination` (links), `Tabs` as link tabs (`?status=`), `Descriptions`, `Result`, `loading.tsx`
   are server components. Client islands only: `SellerSidebar` (collapse state, `usePathname` for active
   link), `SellerNavDrawer`, `SellerFilterBar` (debounced search input that does `router.replace` with
   searchParams), `DeleteListingModal`, `ShipOrderButton`, `PayoutButton`, `SubscribeButton`, `ListingForm`,
   `AdCampaignForm`, `BundleManager`, `PrintButton`.
3. **State in the URL.** `/seller`: `?q=&status=&page=`; `/seller/orders`: `?status=&q=&page=`;
   `/seller/analytics`: `?range=7d|30d|90d`; `/seller/wallet`: `?page=`. Invalid values fall back to defaults
   (never throw). Back/forward and reload preserve the view. Sidebar collapsed state is the only
   non-URL state (client `localStorage`, default expanded, ignored on mobile) and does not affect server
   output.
4. **Mutation contract.** Each Server Action returns `{ ok: boolean; error?: string; data?: T }` and calls
   `revalidatePath` for the affected route(s). Existing actions in `features/seller/actions.ts` and
   `features/listing/actions.ts` return `{ ok, message }`; the new shape is additive during migration (`error`
   mirrors `message` for failures) and call sites move to the new field. Order status moves
   (`updateOrderStatusAction`) get the same shape.
5. **Pagination by cursor, presented as pages.** `listMyListings` is cursor-based (`cursor`, `pageSize`).
   `?page=N` is resolved server-side by walking N-1 cursors (page size 20); `total` from the response drives
   `Pagination`. Orders (`listSellerOrders`) and ledger entries return full arrays; they are sliced
   server-side by `?page=` (page size 20). No proto change.
6. **Filters the gateway cannot do.** `listMyListings` has no text or status argument and `listSellerOrders`
   only a status. `?q=` and listing `?status=` are applied to the page already fetched (documented in the UI
   as "trong trang này"), and order `?status=` uses the RPC `statusFilter`. See Open Questions.
7. **Pending, disabled, toast.** `Button isLoading` + `disabled` while a Server Action is pending
   (`useTransition` or `useFormStatus`); `toast.success` / `toast.error` on settle; row actions on a pending
   row are disabled (not the whole table). Delete uses a `Modal` (title, consequence text, danger confirm
   `Button` with pending, cancel), replacing `confirm()`.
8. **Listing studio.** `ListingForm` keeps `saveListingAction` (`useFormState`) and the hidden `imageKeys` /
   `variants` inputs so the e2e and the server contract do not change. Fields become controlled (value in
   React state) so Magic Listing sets state instead of `document.getElementById` + synthetic events. Magic
   Listing never overwrites a non-empty field without an explicit "Áp dụng" per field (suggestion `Card`),
   which also removes the current hidden fallback title. Validation (required title, price > 0, stock >= 0,
   at least 1 image when status is published) shows on `FormItem` after blur/submit and again from the
   server `SellState`.
9. **Seller order detail uses real data.** `/seller/orders/[id]` becomes a server component calling the
   existing `getOrder(id)` and `getShipmentTracking(id)`; a missing order or one whose `sellerId` is not the
   principal (and no `admin` scope) yields `not-found.tsx` / a 403 `Result`. The fake local state and
   hard-coded buyer/product are deleted; "Bàn giao vận chuyển" is a Server Action
   (`updateOrderStatusAction`) with a confirm `Modal`.
10. **Analysis without a chart library.** The funnel is four `Progress` rows (value = step / impressions), the
    revenue-by-day series is a `Table` plus a `Statistic` total, `?range` narrows `getRevenueBreakdown` days
    client-free (server slice). `SellerAnalyticsMock` and its fake payout button are removed (payout lives
    on `/seller/wallet`).
11. **Tokens and type scale.** Only the 12/14/16/20/24 scale and Tier 2 aliases; no `text-[10px]`/`text-xs`
    micro-text below 12px; brand colour only on the primary CTA per page, prices and status badges; success
    / danger colours only through `success` / `danger` aliases (the current `text-emerald-600` /
    `text-rose-600` are replaced).
12. **Responsive.** Desktop (>= 1024px): sidebar 256px expanded or 64px collapsed (icons + tooltips via
    `title`), content `max-w-6xl`. Tablet 768-1023px: sidebar collapsed by default. Mobile 375px: no sidebar;
    top bar with menu button (opens `Drawer` from the left, closes on navigation and Escape), single-column
    KPI (2-up `Statistic` grid), tables become horizontally scrollable inside a `overflow-x-auto` container
    with the product/order name column sticky; filter bar stacks; form actions sit in a sticky bottom bar;
    `Pagination` shows prev/next + current page only.
13. **CLS = 0.** `Image` with `aspect-square` for thumbnails and listing media; `loading="lazy"` for table
    thumbnails and the media grid below the first screen; each route has `loading.tsx` whose skeleton has the
    same footprint as the page (KPI row, filter bar, table rows); `Statistic loading` keeps its height; the
    sidebar width is set by a CSS class from the first paint (the persisted collapsed state applies after
    hydration on the same width transition, with the layout reserving the expanded width so content does not
    jump on mobile/tablet).
14. **Tracking untouched.** Seller routes carry no tracking hooks today; this change neither adds nor removes
    any. Shared components that do (`TrackLink`, `TrackImpression`, listing links to `/listing/[id]` and
    their `data-*` attributes) are not imported into seller routes. A scenario asserts the analytics
    funnel panel still reads the same `getSellerFunnel` / `getRevenueBreakdown` and that
    `/listing/[id]` row links are unchanged.

15. **Real data only.** No KPI or table cell uses a hard-coded value (the old hard-coded `0` is removed). Each
    `Statistic` is derived from a gateway response; if its source call fails or no source exists the cell is
    hidden (not shown as `0`). The shop card in the sidebar shows the real shop display name from the backend
    change `shop-display-name` (this change depends on it), with "Shop #<6 chars of sellerId>" only when the
    name is empty. No fake shop stats or response rate appear anywhere in the cockpit.

## Page anatomy

- **Shell:** top bar (mobile) / sidebar (desktop); `SellerPageHeader` = `Breadcrumb` (Seller > page) + title
  (20px) + description (14px) + primary action on the right (one brand `Button`).
- **`/seller`:** header (title, "Thêm sản phẩm" primary). Body: KPI row (`Statistic`: total products,
  published, low stock <= 5, open orders), quick actions `Card` (New product, Orders, Wallet, Ads, Bundles as
  `Button variant="outline"` links), recent orders (`Table`, 5 rows, link to `/seller/orders`), product table
  (`SellerFilterBar`, `Table`, `Pagination`). Row: thumbnail `Image` 1:1 48px, title link, `PriceTag`, stock,
  status `Tag`, actions (Xem, Sửa, Xoá -> `Modal`).
- **`/seller/new` / edit:** header (back link, title); body `Card` sections (Thông tin cơ bản, Hình ảnh, Giá &
  tồn kho, Phân loại, Trạng thái); Magic Listing as an `Alert`-style assist `Card` above basic info; sticky
  action bar (Huỷ, Lưu; primary `isLoading`).
- **`/seller/orders`:** header; `Tabs` (Tất cả, Chờ xử lý, Đang giao, Hoàn thành) as links with counts;
  search; `Table` (order id, buyer, items, total `PriceTag`, status `Tag`, actions: Chi tiết, Xác nhận gửi);
  `Pagination`.
- **`/seller/orders/[id]`:** header (Breadcrumb, `Descriptions` summary: order id, created, payment, tracking;
  status `Statistic`; actions Print + Ship); `Stepper` (Pending > Paid > Shipped > Completed); `Tabs`
  (Hàng hoá `Table`, Vận chuyển `Timeline`); `Descriptions` for buyer/address.
- **`/seller/analytics`:** header + range `Tabs`; `Statistic` row (revenue, orders, impressions, conversion);
  funnel `Card` (`Progress`); revenue `Table`.
- **`/seller/wallet`:** balance `Statistic` + payout `Button` (confirm `Modal`, `QuantityPicker`-style amount
  `Input`); ledger `Table` + `Pagination`.
- **`/seller/plans`:** plan `Card` grid; current plan `Tag`; subscribe `Button` (pending, toast).
- **`/seller/ads`, `/seller/bundles`:** form `Card` + existing list `Table`.

## Risks / Trade-offs

- URL `?q=` / listing `?status=` on a cursor API filter only the fetched page; sellers with > 20 products may
  not find an item on another page. Mitigated by the visible "trong trang này" hint and the Open Questions.
- Replacing the DOM-driven Magic fill touches the most-used form; guarded by the existing create/edit e2e
  scenarios plus new ones.
- Walking N cursors for `?page=N` costs N-1 extra RPCs; acceptable for page counts seen in practice.
- Removing `SellerAnalyticsMock` removes demo numbers; the real funnel/revenue panel remains.

## Migration Plan

1. Shell and shared seller components, then Workplace and Table List pages (read paths).
2. Listing studio, then order detail, then wallet/plans/ads/bundles mutations.
3. Each route's `loading.tsx` / `error.tsx` lands with its page. Keep the e2e `data-testid`/role selectors
   used by current `.feature` steps stable until the e2e track updates them in the same change.

## Open Questions

1. Seller order detail RPC: `getOrder` is exposed for buyers; confirm the gateway allows a seller principal to
   fetch an order they sold (spec `order-domain-correctness` says a service principal with `order.read` may
   `GetOrder`). If not, fall back to finding the order in `listSellerOrders` (works today, no proto change).
2. Out-of-stock / locked count on the Workplace: no field for "locked" exists; derive "stock = 0" from the
   fetched page only, or hide the card? (Open. Whatever is shown must be derived from real fetched data, never
   a hard-coded value; if it cannot be derived, the KPI is hidden.)
3. Server-side listing text/status filter and numbered pages would need a gateway change (out of scope). Accept
   page-local filtering for now?
4. Analysis without charts: is `Progress` + `Table` acceptable, or should a lightweight chart be approved in
   a later change?
5. Magic Listing price suggestion: apply only on explicit "Áp dụng" (proposed) or keep the current
   auto-fill-when-low behaviour?
6. Sidebar collapsed preference: `localStorage` only, or persist per seller server-side later?

## E2E coverage of the failure-path scenarios

Failure paths are verified through the real stack, never faked: a `@destructive` browser scenario stops (or `docker pause`s, for
"slow") the `agora` compose-project container behind the read and restores it in teardown after the gateway answers again
(`platform-e2e/tests/e2e/support/uif_support.py`; serial lane only). A scenario that cannot be produced through the edge
without fault-injection code in the product is verified by a named Vitest test instead: its delta-spec scenario carries a
`**VERIFIED BY**` line (file + test name) and its FEATURES.yaml entry is `status: not-testable`, the repo's existing exclusion
status. `platform-e2e/scripts/spec_sync.py` does not read that status, so these scenarios still print as uncovered there.

- Real outage (A), `frontend/uif_seller.feature`: "A KPI without a source is hidden, not zeroed" (team-order stopped), "Magic Listing
  failure is recoverable" (team-ai stopped), "Analytics failure is recoverable" (team-analytics stopped), "List read failure shows an
  Alert" (team-domain stopped), "Route error offers recovery" (team-domain stopped, seller edit page). "Payout confirms, pends and
  toasts" (`frontend/ui_seller_orders_money.feature`) waits out the refund window on the stack and passes serially.
- Defect found, "Route error offers recovery" kept red: the Result and the link to `/seller` render, but "Thử lại" calls only `reset()`
  and the page stays on the error after the service is back (`seller/error.tsx`).
