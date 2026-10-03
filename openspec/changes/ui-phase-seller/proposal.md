## Why

The seller cockpit (Phase 6 in `UI_SYSTEM_DESIGN.md` §6) is the least consistent surface in `team-frontend`.
Today:

- `/seller/layout.tsx` is a static sidebar stacked above the content on mobile (no collapse, no Drawer, no
  active-link state, emoji icons, `text-[10px]` micro-text).
- `/seller` is one page that mixes a header, three hand-rolled metric cards (one hard-coded `0`), and a raw
  `<table>` with a native `confirm()` for delete. There is no filter, no pagination (`listMyListings` is
  cursor-based and the page reads only the first 20), no loading state.
- `/seller/orders/[id]` is a client page with **hard-coded mock data** (a fixed buyer, product and tracking
  number) and local `useState` instead of the real `getOrder` / `getShipmentTracking` gateway calls.
- `/seller/orders` keeps the tab filter in `useState` (lost on reload or back), `/seller/analytics` renders a
  real funnel panel above a mock dashboard, `/seller/wallet` and `/seller/plans` are hand-built cards, and
  `ListingForm` (Magic Listing included) drives inputs through `document.getElementById` and no inline
  validation.
- Mutations return `{ ok, message }` (not the `{ ok, error?, data? }` contract of §5.B) and several use
  `alert`/inline text instead of the toast.

This change rebuilds the seller routes on the core components (`ui-core-components`) following Ant Design Pro
templates — Dashboard › Workplace, Dashboard › Analysis, List › Table List, Form › Basic/Advanced Form,
Profile › Advanced Profile — without a runtime dependency on `antd`.

## What Changes

- `SellerLayout`: a collapsible sidebar (desktop) that becomes a `Drawer` opened from a top bar button (375px),
  `Breadcrumb` in the content header, active-link state, `Badge`/`Avatar` shop card, scope-gate screen moved to
  `Result`.
- `/seller` (Workplace): `Statistic` KPI row, quick actions, recent orders, and the product table (Table
  List) with URL-driven search/status filter and `Pagination`, row actions and a delete confirm `Modal`.
- `/seller/new` and `/seller/[id]/edit` (listing studio, Basic/Advanced Form): `ListingForm` regrouped into
  `Card` sections (basic info, media, pricing and stock, variants, status) built from `FormItem` + `Input` /
  `Select` / `Radio`, field-level validation, pending submit, Magic Listing as a controlled assist (no DOM
  poking) with an `Alert` for failure.
- `/seller/orders` (Table List): status `Tabs` + search in URL, `Table`, `Pagination`, row actions with
  pending state.
- `/seller/orders/[id]` (Advanced Profile): real order data, `Stepper`/`Timeline` of fulfilment, `Descriptions`
  for buyer, address and payment, items `Table`, print packing slip, "Ship" action as a Server Action.
- `/seller/analytics` (Analysis): `Statistic` row, funnel as `Progress`-based steps, revenue by day as
  `Table`, range `Tabs` in URL; the mock panel (`SellerAnalyticsMock`) is removed.
- `/seller/wallet`, `/seller/plans`, `/seller/ads`, `/seller/bundles`, `/sell`: restyled on the same shell and
  components; mutations (payout, subscribe, create ad campaign, create bundle) become Server Actions that
  return `{ ok, error?, data? }` and call `revalidatePath`.
- Only real data is shown: KPI values come from gateway responses (no hard-coded `0` or placeholder numbers);
  a KPI with no source is hidden, and a block with no data shows `Empty`. The sidebar shop card shows the real
  shop display name (fallback "Shop #<6 chars>" only for an empty name).
- Every `loading.tsx` / `error.tsx` / `not-found.tsx` for the seller segment, so each route has a skeleton, an
  error `Result` with retry, and a not-found state.
- A FEATURES.yaml + `.feature` e2e track that extends the existing `seller/*.feature` files.

Repos: `team-frontend` (code), `platform-e2e` (tests). Capability: `ui-seller` (new).

## Capabilities

### New Capabilities
- `ui-seller`: the seller cockpit shell, page anatomy, state, mutation and responsive contract.

### Modified Capabilities
None.

## Non-goals

- No backend, gateway or proto change in this change (the shop display name comes from the separate change
  `shop-display-name`). Where a screen would like data the gateway does not expose (listing
  status filter, listing text search, numbered pages, an out-of-stock count, a seller order detail RPC) the UI
  filters what it already has or shows an honest empty value; see the design's Open Questions.
- No `antd` / `@ant-design/*` dependency and no chart library: Analysis uses `Statistic`, `Progress` and
  `Table` only.
- No routes owned by another phase (discovery, product detail, cart/checkout, buyer orders, account).
- `ListingCard`, `ListingGrid`, `SearchBar`, `FilterSidebar` and `SortBar` belong to `ui-phase-discovery`; this
  change does not use or change them.
- No change to tracking: `TrackLink`, `TrackImpression`, `SearchImpressions`, `AnalyticsProvider`,
  recommendation placement attribution and `data-*` attributes stay as they are.
- No new seller capabilities (no coupon, return-handling or chat redesign; the `/chat` link only keeps
  pointing at the existing page).

## Dependencies

- Requires the backend change `shop-display-name` (adds a shop/seller display name to the contract) for the
  seller shop card in `SellerLayout`.

## Impact

- `team-frontend`: `src/app/seller/**` (layout, pages, new `loading.tsx` / `error.tsx` / `not-found.tsx`),
  `src/app/sell/page.tsx`, `src/features/seller/*`, `src/features/listing/{ListingForm,DeleteListingButton,
  actions}.tsx`, `src/features/order/SellerOrdersList.tsx`, `FEATURES.yaml`.
- `platform-e2e`: `tests/e2e/features/seller/*.feature` (extended) plus steps and page objects.
- Depends on `ui-foundation` and `ui-core-components`. Independent of the other phase changes.
