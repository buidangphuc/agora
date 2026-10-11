## Context

Current state (read from `team-frontend`):

- `app/page.tsx` is a server component with `force-dynamic`; it awaits `listCategories` + `listListings`
  together and renders hero, hubs, category grid, flash sale, mall row, feed, then `RecentlyViewedRow`,
  `RecommendationsRow` (server) and `AiAssistantModal`. `FlashSaleSection` (client) exists but the page
  inlines its own copy.
- `app/search/page.tsx` is a server page reading `searchParams`; `FilterSidebar` and `SortBar` are client
  components that push URLs; `SearchImpressions` is a client leaf that fires one batched `view_item_list`.
  `searchListings` ignores `PageRequest`; errors are caught into `EMPTY_FACETS`.
- `app/vouchers/page.tsx` fetches `listVouchers()` but `VouchersView` (client) renders the static
  `AVAILABLE_VOUCHERS`; `VoucherManager` (seller-only create form, Server Action `createVoucherAction`) is
  untouched by this change except for token clean-up.
- `ListingCard` wraps itself in `TrackImpression` (IntersectionObserver, `view_item_list`) and uses
  `TrackLink` (`select_item`) on image and title; `FavoriteButton` is a client leaf. `SearchBar` is mounted
  in `app/layout.tsx` and calls `/api/suggest`.

## Goals / Non-Goals

Goals: Ant Pro page anatomies on the core components; CLS 0; every async block has a skeleton; URL-driven
state; every failure has a visible recovery; identical tracking. Non-goals: see proposal.

## Decisions

1. **Ant Design / Ant Design Pro mapping**

   | Ant Design / Pro | Used for | agora route / component |
   |---|---|---|
   | List › Search List: search header | query + context | `/search` header: `Breadcrumb`, result summary, `SearchBar` (global, in layout) |
   | List › Search List: filter bar (`StandardFormRow`/`Select`/`Tag.CheckableTag`) | facets | `FilterSidebar` built from `FormItem` + `Checkbox`/`Radio` + `Input` (price); mobile `Drawer` |
   | List › Search List: sort + result list | sorting, results | `SortBar` (`Tabs`-style links + `Select` for price), `ListingGrid` |
   | Active filter tags (`Tag` closable) | applied filters | `Tag` closable rendered as `<a>` to the URL without that filter |
   | List › Card List | product grid | `ListingGrid` + `ListingCard` (`Card`, `Image` 1:1, `Tag`/`Badge`, `PriceTag`, `Rate` only when real review data exists) |
   | `Card` with `loading` / `Skeleton` | async blocks | `ListingGridSkeleton`, `loading.tsx` per route, Suspense per home block |
   | `Empty` | no results / no vouchers | `ListingGrid` empty, `/search` zero results, `/vouchers` per tab |
   | `Result` (404/500 pattern) / `Alert` | failures | `/search` backend failure: inline `Alert` + retry; home block failure: block hidden or `Alert` |
   | `Pagination` | result paging | `/search` `Pagination` with `hrefFor(page)` |
   | `Statistic.Countdown` | flash-sale clock | `FlashSaleSection` `CountdownClock` leaf, only with a real `endsAt` (no source yet: block hidden) |
   | `Progress` (line) | voucher used % | `Progress` in the voucher card (real used/quota only) |
   | `Tabs` | voucher type | `/vouchers` `Tabs` with links to `?type=` |
   | `Drawer` | mobile filters | `FilterSidebar` mobile container |
   | `message` | save-search feedback | `ToastProvider` |
   | `Breadcrumb` | search trail | `/search` header |
   | `Tag` / `Badge` | freeship, voucher type, filter count | `Tag` (Freeship, voucher type), `Badge` (filter count). No MALL or discount badge: no real data |
   | `Image` | media | `Image` with `aspect="square"` / `"2/1"`, fallback, lazy |
   | `Carousel` / banners | hero | static hero server markup (no carousel; not in the component list) |

2. **Page anatomy**

   - `/` (Card List composition). Header: none (global shell). Body, top to bottom: `LoyaltyWidget`
     (logged-in only, existing); Hero (`Card`-like banner, `Image` `aspect-2/1` background, one primary CTA
     "Mua ngay" + one secondary "Lưu voucher"); `ServiceHubs` (8 tiles, neutral `Card` tiles, label 12px,
     `Tag` badge); `CategoryBar variant="grid"`; `FlashSaleSection` (rendered only when real campaign data exists, so
     currently hidden); feed `ListingGrid` + "Xem thêm gợi ý" link-button to `/search`; `RecentlyViewedRow`; `RecommendationsRow`;
     `AiAssistantModal` (unchanged). Each block after the hero is its own async server component inside
     `<Suspense fallback={<…Skeleton/>}>` so categories, flash sale and feed stream independently.
   - `/search` (Search List). Header: `Breadcrumb` (Trang chủ > category or "Kết quả cho “q”" or "Tất cả sản
     phẩm"), `h1` 20px, result count (Statistic-style, 14px). Body: two columns at `lg` (sidebar 256px +
     results); sidebar = `SavedSearches` (unchanged) + `FilterSidebar`; results = `SortBar`, active-filter
     `Tag`s, `ListingGrid` inside `data-testid="search-results"`, `Pagination`. Actions: sort, filter,
     clear all, save search (existing).
   - `/vouchers` (Card List of coupons). Header: banner + `h1`. Body: `Tabs` (Tất cả / Freeship / Giảm tiền /
     Giảm %) as links to `?type=`; grid of `VoucherCard` (`Card`, code `Tag`, title, `Progress` used % from real used/quota,
     expiry, "Dùng ngay" link to `/search`; no "Lưu mã" button); seller-only `VoucherManager` stays above (restyled to tokens only).

3. **Server / client split (§5)**
   - Server: both pages, `ListingGrid`, `ListingCard` shell, `CategoryBar`, hero, hubs, `FilterSidebar`
     markup, `SortBar` (links, no JS), active-filter tags, `Pagination` (links), `VoucherCard`, vouchers
     `Tabs`.
   - Client leaves only: `TrackImpression`, `TrackLink`, `SearchImpressions`, `FavoriteButton`,
     `SearchBar` (autocomplete), `CountdownClock` (only mounted with a real `endsAt`), `FilterDrawerTrigger` + price-range form (needs
     controlled input), `AiAssistantModal`, `LoyaltyWidget`.
   - `SortBar` and `FilterSidebar` become link/`<form method="get">` based so they work without JS; the
     client leaf only adds optimistic pending feedback (`useTransition` + `aria-busy`).
   - `FlashSaleSection` becomes a server component taking `listings` and an optional `endsAt`; only
     `CountdownClock` is `"use client"`.

4. **URL state.** `/search`: `q, category, seller, rating, minPrice, maxPrice, sort, page`. `/vouchers`:
   `type` (`all|shipping|fixed|percent`). Changing any filter or sort resets `page`. Home has no state.
   Unknown values fall back to defaults (no throw).

5. **Pagination without a proto change.** `SearchListingsRequest.page` is a cursor + `page_size`. `?page=N`
   is resolved server-side in `searchListings`: it walks `next_cursor` N-1 times at `page_size=24` (capped at
   page 20) and returns the Nth page; `total` drives `Pagination`. Same RPC, no contract change.

6. **Mutations.** "Save voucher" is not a mutation of this change: there is no claim RPC (proto has
   Create/Get/List only), so "Lưu mã" is hidden and nothing is simulated in the browser (no `localStorage`
   wallet). The remaining mutations are save search and create voucher. `saveSearchAction` and `createVoucherAction` already return `{ ok, message }` and `revalidatePath`; they
   are normalised to `{ ok, error?, data? }` (additive: `message` kept).

7. **CLS = 0.** Every image goes through `Image` with a fixed aspect (`square` for cards, `2/1` for hero,
   hub icons are emoji in a fixed 48px box). The first 6 cards on `/` and `/search` render with
   `loading="eager"`/`priority` (above the fold); all others `lazy`. Skeletons share the exact grid, card
   height and aspect of the real content (`ListingCardSkeleton`). Where a real countdown is shown it reserves fixed
   width digits (`tabular-nums`, min-width box).

8. **Type and colour (Principle 1).** Scale 12/14/16/20/24 only: hero title 24 (20 on mobile), section
   titles 20/16, card title 14, meta 12, price 16 (`PriceTag size="md"`). Brand colour: primary CTAs, prices
   (`PriceTag`), badges (discount, flash-sale). Service-hub tiles become neutral `surface-muted`; the eight
   rainbow backgrounds are removed. There is no MALL badge on cards.

9. **Tracking is frozen.** `ListingCard` keeps the exact tree: `TrackImpression` (props `listingId,
   placementId, impressionId, modelVersion, position`) around the article; `TrackLink` on image and title
   with the same props; `ListingGrid` keeps passing `placementId/impressionId/modelVersion` and
   `position=index+1`; `RecommendationsRow` keeps `home_feed`/`similar_items`; search keeps
   one `SearchImpressions` with the rendered ids and `q`; `data-testid="search-results"` is kept. A unit
   test snapshots the `trackEcommerce` payloads before and after.

10. **Responsive.** 375px (mobile): grid `grid-cols-2` gap 8; hubs `grid-cols-4`; category grid 2 columns
    scrollable pills for `variant="pills"`; flash sale shows 2 columns with horizontal snap; `/search`
    sidebar collapses into a "Bộ lọc" button opening a bottom `Drawer`, sort `Select` replaces tab links;
    `Pagination` shows prev/next + current only; vouchers 1 column; tap targets >= 44px. Desktop (>= 1024px):
    grid 6 columns (4 at `md`), sticky 256px sidebar, full numbered `Pagination`, vouchers 2 columns.

11. **Empty / error / not-found.** Zero search results: `Empty` ("Không tìm thấy sản phẩm") + `Button`
    "Xóa bộ lọc" (link to `/search`, keeps `q` only when present). Search backend failure: `Alert
    type="error"` + retry link to the same URL, results area only (sidebar and header still render). Page
    beyond last: redirect to last page. Vouchers empty/outage: `Empty` + "Xem sản phẩm". Home block failure or no
    real data (flash sale, recommendations, recently viewed): block omitted; feed failure shows `Alert` + retry. `not-found.tsx`/`error.tsx` come from `ui-foundation`.

12. **Real data only (Principle: no fabricated data).** `ListingCard` renders rating (`Rate` + count) only
    when the gateway listing carries a review aggregate with count > 0, and never renders a sold count unless
    the gateway returns one; the strike-through price (`price * 1.25`), the "-25%"/"-20%" discount badges, the
    `isMall` heuristic and the MALL `Badge` are deleted. `FlashSaleSection` takes real `endsAt` and per-item
    sold/stock; with no campaign source the home does not mount it, and the "ĐÃ BÁN 82%" bar and fake
    countdown are deleted. The Mall brands row is removed (no verified-shop source). A hidden block leaves no
    empty heading or gap.

## Risks / Trade-offs

- Walking cursors for deep pages adds RPC calls (<= 19 extra, cheap, no listing fan-out); capped at page 20.
- `ListingCard` is used by phases 2-6; changes are visual-only and prop-additive, guarded by a prop-contract
  test.
- Server `SortBar`/`FilterSidebar` lose instant client-side updates; mitigated with a pending state and
  Next prefetch of links.

## Open Questions

1. Decided: the "Bán Chạy" sort is hidden until a backend `SortBy` exists (follow-up backend change); it is
   never mapped to relevance.
2. Decided: the flash-sale countdown, "-25%", "ĐÃ BÁN 82%", "-20%", the 5.0 rating and the sold counts are
   removed, not kept as placeholders. Only real gateway data is shown; blocks without data are hidden.
3. Decided: no voucher wallet. "Lưu mã" is hidden and there is no `localStorage` simulation until a
   `ClaimVoucher` RPC exists (follow-up backend change).
4. Should `/vouchers` fall back to the static `AVAILABLE_VOUCHERS` when `listVouchers()` returns empty (demo
   data), or show `Empty`? Default here: `Empty`; the static list is removed from the route. (Consistent with
   the real-data-only rule; confirm.)
5. Decided: the `isMall` heuristic is deleted from `ListingCard` and no Mall badge or row is rendered until a
   real verified-shop flag exists.
6. The existing e2e scenario "Visitor browses available vouchers" (`promo/vouchers.feature`) asserts at least
   one "Lưu mã" button. With the static list and the button removed, the scenario must instead seed a voucher
   through the gateway (seller token, `CreateVoucher`) and assert its code card, with no "Lưu mã" assertion;
   confirm the seed tag name with platform-e2e owners.

## E2E coverage of the failure-path scenarios

Failure paths are verified through the real stack, never faked: a `@destructive` browser scenario stops (or `docker pause`s, for
"slow") the `agora` compose-project container behind the read and restores it in teardown after the gateway answers again
(`platform-e2e/tests/e2e/support/uif_support.py`; serial lane only). A scenario that cannot be produced through the edge
without fault-injection code in the product is verified by a named Vitest test instead: its delta-spec scenario carries a
`**VERIFIED BY**` line (file + test name) and its FEATURES.yaml entry is `status: not-testable`, the repo's existing exclusion
status. `platform-e2e/scripts/spec_sync.py` does not read that status, so these scenarios still print as uncovered there.

- Real outage (A), `frontend/uif_discovery.feature`: "Block failure degrades locally" (team-ai and team-engagement stopped after the
  Vừa xem block was seen), "Server action failure shows an error toast" (team-search stopped after the search page rendered, so
  `saveSearchAction` fails), "Feed failure shows a recovery action" (team-domain stopped).
- Defect found, "Feed failure shows a recovery action" kept red: with team-domain stopped the whole home page is replaced by the root
  error ("Đã có lỗi xảy ra"); the server log shows `ConnectError [unavailable]` thrown from `getListing` in a `Promise.all` of
  `(home)/page.js`. `RecentlyViewedRow` hydrates ids with `getListing`, which only swallows NotFound, so one failing block takes the
  page down and the feed Alert never shows.
- Not through the edge (B): "Countdown ticks without layout shift" and "No listings hides the section" (FlashSaleSection is mounted on
  no route) are verified by Vitest `team-frontend/src/features/home/FlashSaleSection.test.tsx` › "CountdownClock › ticks without
  changing its box" and "FlashSaleSection › renders nothing for an empty list"; "Loading skeleton matches the grid" (Next does not show
  `loading.tsx` for search-param navigations) by `team-frontend/src/app/(shop)/search/loading.test.tsx` › "search loading › renders
  the header, filter column and 24 card skeletons in the real grid".
