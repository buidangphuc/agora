## Why

Phase 1 of the UI system (`UI_SYSTEM_DESIGN.md` §6) covers the three discovery routes: `/` (home),
`/search` (catalogue + facets) and `/vouchers`. Today they predate the design contract:

- `/` is one 300-line server page with inline hero, hubs, category grid, flash sale and feed. It uses
  `text-[9px]`/`text-[10px]` micro-text, a rainbow of eight hub colours, raw `<img>` without `Image`, a
  hard-coded fake countdown and `82% sold` bar, and no `loading.tsx`/Suspense, so any slow gateway call
  blocks the whole page.
- `/search` renders results with no pagination, swallows search errors into an empty list, has no loading
  state, a sidebar that is unusable on a 375px screen, an unwired "Bán Chạy" sort, and copy left over from a
  room-rental WIP ("Tất cả phòng cho thuê").
- `/vouchers` is a client page that renders a static `AVAILABLE_VOUCHERS` list and keeps "saved" state in
  `useState`, ignoring the real vouchers the server already fetched; its tab state is lost on refresh and the
  save action gives no pending state.

Ant Design Pro solves the same screens with **List › Search List** (search header, filter, result list) and
**List › Card List** (card grid with skeleton and empty), plus Pagination, Skeleton and Empty. We adopt those
page anatomies, composed from the `ui-core-components` set, and make this change the owner of the shared
discovery components every later phase reuses.

## What Changes

- Rebuild `/`, `/search` and `/vouchers` page anatomy on the core components (design.md maps each Ant Pro
  template to the agora component).
- Own and rework the shared discovery components, keeping their public props additive:
  `ListingCard`, `ListingGrid`, `SearchBar`, `FilterSidebar`, `SortBar`, `CategoryBar`, `FlashSaleSection`.
- Add `loading.tsx` for `/`, `/search`, `/vouchers`, and per-block Suspense skeletons on the async blocks of
  the home page (flash sale, feed, recommendations, recently viewed).
- `/search`: URL-driven `page`, numbered `Pagination`, active filters as closable `Tag` links, mobile
  filter `Drawer`, an `Alert` with retry on search failure (instead of a silent empty list) and an `Empty`
  with a "clear filters" action.
- `/vouchers`: render the vouchers fetched by the server, tab state in `?type=`, `Empty` per tab. No "Lưu mã"
  button and no browser wallet: voucher saving is hidden until a backend exists (see Non-goals).
- Replace micro-text and the rainbow hub palette with the 12/14/16/20/24 scale and neutral tiles; brand
  colour only on primary CTAs, prices and badges.
- Remove all fabricated data: no fake ratings or sold counts, no computed strike-through price, no fake
  discount badges, no fake flash-sale countdown or "% sold" bar, no title- or price-based Mall heuristic
  (`isMall` is deleted from `ListingCard`), no hard-coded hero/hub figures. Only gateway data is shown; a block
  with no real data is hidden, or shows `Empty` where the block is the page's purpose.
- Hide features without a backend instead of simulating them: the "Bán Chạy" sort, "Lưu mã" on `/vouchers`,
  and the home flash-sale and Mall rows (no promotion or shop-verified source).
- Existing tracking (`TrackImpression`, `TrackLink`, `SearchImpressions`, placement attribution, `data-*`
  attributes) is kept byte-for-byte in behaviour.

Repos: `team-frontend` (code), `platform-e2e` (tests). Capability: `ui-discovery`.

## Capabilities

### New Capabilities
- `ui-discovery`: page anatomy, state handling and shared components of the home, search and vouchers routes.

### Modified Capabilities
None. (Existing FEATURES.yaml entries `home.landing`, `promo.vouchers-page` and the facet scenarios keep their
meaning; they are extended, not changed.)

## Non-goals

- No backend, gateway or proto change: pagination uses the existing `PageRequest` cursor; no new RPC.
- Follow-up backend change: server-side voucher claim/save (`ClaimVoucher`); until then "Lưu mã" is hidden and
  there is no `localStorage` wallet.
- Follow-up backend change: a "best selling" `SortBy` (and sold-count data); until then the "Bán Chạy" sort
  option is hidden.
- Follow-up backend change: real flash-sale campaign data (promotion list RPC with `endsAt` and sold/stock)
  and a real verified-shop flag for a Mall row; until then neither block is rendered.
- Follow-up backend change: review aggregates on search/list listings, if the gateway does not already return
  them; until then cards show no rating.
- No `antd` dependency, no CSS-in-JS.
- No route owned by another phase (`/listing/[id]`, `/cart`, `/checkout`, `/account/*`, `/seller/*`,
  `/shop/*`, `/favorites`). `ListingCard`/`ListingGrid` are consumed there and must stay prop-compatible.
- No change to tracking events, payloads, `data-*` attributes or placement ids.
- No new ranking, recommendation or flash-sale business logic.

## Impact

- `team-frontend/src/app/{page,loading}.tsx`, `src/app/search/{page,loading}.tsx`,
  `src/app/vouchers/{page,loading}.tsx`, `src/features/{listing,search,home,voucher}/*`,
  `src/lib/gateway/search.ts` (page option), unit tests.
- `team-frontend/FEATURES.yaml`, `platform-e2e/tests/e2e/features/{frontend,promo}/*.feature`, steps and
  page objects.
- Depends on `ui-foundation` and `ui-core-components`. Phases 2-6 depend on this change for the shared cards.
