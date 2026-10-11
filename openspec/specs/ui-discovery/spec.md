# ui-discovery Specification

## Purpose
Defines the discovery routes of `team-frontend` (`/`, `/search`, `/vouchers`) and the shared components they
own (`ListingCard`, `ListingGrid`, `SearchBar`, `FilterSidebar`, `SortBar`, `CategoryBar`,
`FlashSaleSection`), following Ant Design Pro's List › Search List and List › Card List anatomies on the
`ui-core-components` set.

## Requirements

### Requirement: Home page is composed of independently streamed blocks

`/` SHALL be a server component composed, in order, of: loyalty widget (logged-in only), hero, service hubs,
category grid, flash sale (only with real campaign data), feed, recently viewed, recommendations. There SHALL be
no Mall brands row. Every block that awaits data (categories, flash sale, feed, recommendations, recently
viewed) SHALL sit in its own
`Suspense` boundary with a `Skeleton` fallback of the same footprint, and `/` SHALL have a `loading.tsx`.
Client code SHALL be limited to leaf islands (countdown, favorite, tracking, assistant, loyalty).

#### Scenario: Slow block does not block the page

- **WHEN** the listings call for the feed takes several seconds while categories resolve immediately
- **THEN** the hero, hubs and category grid are visible first and the feed area shows `ListingCard` skeletons
  until its data arrives

#### Scenario: Block failure degrades locally

- **WHEN** the recommendations or recently-viewed query fails or returns no items
- **THEN** that block is not rendered (no empty heading) and every other block still renders

#### Scenario: Feed failure shows a recovery action

- **WHEN** the feed query fails
- **THEN** an `Alert` of type error with a retry link to `/` is shown in the feed area instead of an empty grid

### Requirement: Shared listing grid and card are the single product tile

`ListingGrid` and `ListingCard` SHALL render every product tile on the discovery routes. A card SHALL use
`Image` with `aspect="square"`, show title (14px, two-line clamp), `PriceTag` (brand colour) with the real price only, a Freeship `Tag`
when the listing is actually free-ship, rating (12px) only when the gateway returns a review aggregate with
count > 0, and a favorite button. A card SHALL NOT show a sold count unless the gateway returns one, and SHALL
NOT show a strike-through price, a discount badge or a MALL badge. The first 6 cards of a
page SHALL load their image eagerly; all later cards SHALL use `loading="lazy"`. `ListingGrid` SHALL render
2 columns at 375px, 3 at `sm`, 4 at `md`, 6 at `lg`, and an `Empty` when `listings` is empty. Public props of
both components SHALL remain backward compatible so other phases can keep using them.

#### Scenario: Card reserves its image box

- **WHEN** a `ListingCard` is rendered before its image has loaded
- **THEN** the image container already has a 1:1 box and the card's height does not change when the image loads

#### Scenario: Image fallback keeps the layout

- **WHEN** a listing has no image or the image fails to load
- **THEN** a placeholder is shown inside the same 1:1 box

#### Scenario: Below-the-fold images are lazy

- **WHEN** a grid of 24 cards is rendered
- **THEN** images of cards 1-6 are not `loading="lazy"` and images of cards 7-24 have `loading="lazy"`

#### Scenario: Empty grid offers a way out

- **WHEN** `ListingGrid` receives an empty list with an `empty` message
- **THEN** an `Empty` block shows that message and a suggestion to change keywords or filters

#### Scenario: Listing without reviews shows no rating or sold count

- **WHEN** a `ListingCard` is rendered for a listing with no review aggregate
- **THEN** no rating stars, no rating number and no sold count are rendered

#### Scenario: Card shows only the real price

- **WHEN** a `ListingCard` is rendered for a listing priced 1,250,000, including one above 5,000,000 or with a
  brand keyword in its title
- **THEN** exactly one price is shown, there is no strike-through price, no discount badge and no MALL badge

#### Scenario: Card layout works at 375px

- **WHEN** a grid is rendered in a 375px wide viewport
- **THEN** it shows 2 columns, no horizontal page scroll, and no text smaller than 12px

### Requirement: Discovery routes show only real data

Discovery routes SHALL display only data returned by the gateway. They SHALL NOT render fabricated ratings,
sold counts, strike-through prices computed from the price, discount badges, flash-sale countdowns or "% sold"
bars, Mall badges or rows derived from title or price, or hard-coded hero/hub figures. A block with no real
data SHALL be hidden; `Empty` is used only where the block is the page's purpose (search results, vouchers).
Features without a backend SHALL be hidden, not simulated: the "Bán Chạy" sort option and "Lưu mã" on
`/vouchers`; no `localStorage` voucher wallet SHALL exist.

#### Scenario: No fabricated markers on the home page

- **WHEN** `/` is rendered with listings that have no review, discount or campaign data
- **THEN** the document contains no strike-through price, no "-NN%" badge, no "ĐÃ BÁN" text, no countdown, no
  MALL badge and no star rating

#### Scenario: Mall is never guessed

- **WHEN** a listing costs more than 5,000,000 or its title contains a brand keyword
- **THEN** its card shows no MALL badge

#### Scenario: Unsupported features are hidden

- **WHEN** `/search` and `/vouchers` are rendered
- **THEN** the sort options exclude "Bán Chạy" and no voucher card has a "Lưu mã" button

### Requirement: Discovery pages use the design scale and colour rules

Discovery routes SHALL use only the type sizes 12, 14, 16, 20 and 24px, SHALL contain no arbitrary
`text-[...]` values or raw hex, and SHALL use the brand colour only on primary calls to action, prices and
badges. Service-hub tiles SHALL use neutral surfaces.

#### Scenario: Token lint passes on discovery files

- **WHEN** `scripts/check-tokens.mjs` runs on `src/app/page.tsx`, `src/app/search`, `src/app/vouchers` and
  `src/features/{listing,search,home,voucher}`
- **THEN** it reports no raw hex, `rgb()` or arbitrary values

#### Scenario: Hub tiles are not rainbow

- **WHEN** the eight service hubs are rendered
- **THEN** every tile uses the same neutral surface and only the hub badge uses a badge colour

### Requirement: Flash sale renders only real campaign data

`FlashSaleSection` SHALL be a server component that renders its header and `ListingCard` tiles from real
campaign data (`listings`, a real `endsAt`, and real per-item sold/stock if provided). It SHALL only ship client
code for a `CountdownClock` that ticks every second, mounted only when a real `endsAt` is provided; the clock
SHALL reserve a fixed width (tabular digits). It SHALL render a sold `Progress` only for items with real
sold/stock figures, and SHALL render nothing when there are no listings or no campaign. Until a backend
promotion source exists, the home page SHALL NOT mount it. On 375px the tiles scroll horizontally with snap;
on desktop they form a 6-column grid.

#### Scenario: Countdown ticks without layout shift

- **WHEN** a real `endsAt` is passed and the countdown advances from 00:59:59 to 01:00:00
- **THEN** the clock's bounding box width and height are unchanged
- **VERIFIED BY**: Vitest `team-frontend/src/features/home/FlashSaleSection.test.tsx` › CountdownClock › ticks without changing its box. Not verifiable end to end: FlashSaleSection and CountdownClock are not mounted on any route until a promotion source exists.

#### Scenario: No listings hides the section

- **WHEN** `FlashSaleSection` receives an empty list
- **THEN** it renders nothing
- **VERIFIED BY**: Vitest `team-frontend/src/features/home/FlashSaleSection.test.tsx` › FlashSaleSection › renders nothing for an empty list. Not verifiable end to end: FlashSaleSection is not mounted on any route until a promotion source exists.

#### Scenario: No campaign data means no fake countdown or sold bar

- **WHEN** the home page is rendered and the gateway provides no flash-sale campaign
- **THEN** no flash-sale heading, no countdown, no "% sold" bar and no discount badge are present

### Requirement: Category navigation is URL-driven

`CategoryBar` SHALL render links to `/search?category=<id>` in two variants: `grid` (home) and `pills`
(horizontally scrollable, used above results). The selected category SHALL be marked with `aria-current`.
It SHALL render nothing when there are no categories.

#### Scenario: Selecting a category navigates by URL

- **WHEN** a visitor activates the "Điện thoại" category on the home grid
- **THEN** the browser navigates to `/search?category=<that id>` and the results are limited to that category

#### Scenario: Current category is announced

- **WHEN** `/search?category=c1` is rendered with `CategoryBar variant="pills"`
- **THEN** the pill for `c1` has `aria-current="true"` and the "Tất cả" pill does not

### Requirement: Search page follows the Search List anatomy with URL state

`/search` SHALL render: a header with `Breadcrumb`, `h1` and the result count; a filter column with
`SavedSearches` and `FilterSidebar`; and a results column with `SortBar`, active-filter `Tag`s,
`ListingGrid` (`data-testid="search-results"`) and `Pagination`. All state SHALL live in the URL:
`q, category, seller, rating, minPrice, maxPrice, sort, page`. Changing any filter or sort SHALL reset `page`.
Facet counts SHALL be shown beside each bucket. The page SHALL work for sort and filter links without
client JavaScript.

#### Scenario: Filters are shareable URLs

- **WHEN** a buyer selects the "100000-500000" price bucket and sort "price_asc"
- **THEN** the URL contains `minPrice=100000&maxPrice=500000&sort=price_asc`, and opening that URL in a new
  tab shows the same results and the same selected state

#### Scenario: Changing a filter resets the page

- **WHEN** a buyer on `/search?q=ao&page=3` selects a rating filter
- **THEN** the new URL has no `page` (or `page=1`) and shows the first page of the filtered results

#### Scenario: Active filters can be removed one by one

- **WHEN** `/search?q=ao&category=c1&rating=4` is rendered
- **THEN** three closable `Tag`s are shown, and activating the category tag navigates to
  `/search?q=ao&rating=4`

#### Scenario: Clear all filters

- **WHEN** at least one filter is active and the buyer activates "Xóa tất cả bộ lọc"
- **THEN** the URL becomes `/search` (keeping `q` only if present) and all tags disappear

#### Scenario: Sort options match what the server honours

- **WHEN** the `SortBar` is rendered
- **THEN** it offers only Liên quan, Mới nhất, Giá thấp đến cao and Giá cao đến thấp (no "Bán Chạy"), and the active one has
  `aria-current="true"`

### Requirement: Search results are paginated

`/search` SHALL show at most 24 results per page and a `Pagination` with `hrefFor(page)` links that preserve
every other search param. `?page=N` SHALL be resolved on the server using the existing cursor
`PageRequest` (no contract change). A `page` beyond the last page SHALL redirect to the last page; an invalid
`page` SHALL be treated as 1. On 375px `Pagination` SHALL show previous, current and next only.

#### Scenario: Page links preserve the query

- **WHEN** `/search?q=ao&sort=newest` has 60 results
- **THEN** `Pagination` shows pages 1-3 as links such as `/search?q=ao&sort=newest&page=2`, page 1 marked
  `aria-current="page"`

#### Scenario: Out-of-range page redirects

- **WHEN** a visitor opens `/search?q=ao&page=99` and only 3 pages exist
- **THEN** the response redirects to `/search?q=ao&page=3`

#### Scenario: Single page hides Pagination

- **WHEN** the total is 24 or fewer
- **THEN** no `Pagination` is rendered

### Requirement: Search handles loading, empty and error states

`/search` SHALL have a `loading.tsx` showing the header, filter column skeleton and a grid of
`ListingCardSkeleton` with the same grid, so streaming a result does not shift layout. Zero results SHALL
render `Empty` with a "Xóa bộ lọc" button. A search backend failure SHALL render an `Alert` of type error
with a retry link inside the results column and SHALL NOT be shown as "no results".

#### Scenario: Loading skeleton matches the grid

- **WHEN** a filter change triggers navigation
- **THEN** the results column shows 24 `ListingCardSkeleton`s in the same grid, and the sidebar remains
- **VERIFIED BY**: Vitest `team-frontend/src/app/(shop)/search/loading.test.tsx` › search loading › renders the header, filter column and 24 card skeletons in the real grid. Not verifiable end to end: Next serves search-param navigations from the router prefetch cache and does not show loading.tsx for them, so the skeleton is not observable in a browser (measured with the RSC request delayed).

#### Scenario: Zero results

- **WHEN** `/search?q=zzzzzz` returns no hits
- **THEN** `Empty` is shown with the text "Không tìm thấy sản phẩm" and a "Xóa bộ lọc" button linking to
  `/search`

#### Scenario: Backend failure is not a silent empty

- **WHEN** the search call throws
- **THEN** an error `Alert` with a "Thử lại" link to the same URL is shown, the header and filter column are
  still rendered, and no `Empty` is shown

### Requirement: Filters are usable on mobile

`FilterSidebar` SHALL render inline from 1024px up. Below 1024px it SHALL be hidden behind a "Bộ lọc" `Button`
(with a `Badge` showing the active filter count) that opens a `Drawer` containing the same filters with a
sticky "Áp dụng" button. Price min/max inputs SHALL validate (`FormItem` error) when min is greater than max
and SHALL NOT navigate until valid.

#### Scenario: Filters open in a drawer at 375px

- **WHEN** a visitor at 375px width opens `/search` and taps "Bộ lọc"
- **THEN** a `Drawer` opens with the category, price, rating and seller filters, focus moves into it, and
  Escape closes it and returns focus to the "Bộ lọc" button

#### Scenario: Invalid price range is rejected

- **WHEN** the buyer enters min 500000 and max 100000 and submits
- **THEN** the max field shows an error, the URL does not change, and the submit button is not left loading

#### Scenario: Active count badge

- **WHEN** two filters are active at 375px
- **THEN** the "Bộ lọc" button shows a badge with `2`

### Requirement: Mutations on discovery routes give feedback

Every mutation initiated from the discovery routes (save search, create voucher) SHALL
show a pending state (`isLoading`, width preserved), a disabled state while pending or when the action is
already done, and a success or error toast. Server Actions SHALL return `{ ok, error?, data? }` and call
`revalidatePath` for the route they change.

#### Scenario: Save search shows pending then toast

- **WHEN** a buyer activates "Lưu tìm kiếm" for query "ao"
- **THEN** the button shows a spinner and is disabled until the action settles, then a success toast appears
  and the saved list contains "ao"

#### Scenario: Save search with empty query is rejected

- **WHEN** the buyer activates "Lưu tìm kiếm" with an empty query
- **THEN** the action returns `ok: false` with an `error`, an error toast is shown, and nothing is saved

#### Scenario: Server action failure shows an error toast

- **WHEN** `saveSearchAction` returns `{ ok: false, error: "..." }`
- **THEN** an error toast with that message is shown and the button is enabled again

### Requirement: Vouchers page renders real vouchers with URL tabs

`/vouchers` SHALL render the vouchers returned by `listVouchers()` as `VoucherCard`s (code `Tag`, title,
`Progress` for real used/quota, expiry, "Dùng ngay" link to `/search`, and no "Lưu mã" button) under `Tabs` whose selected tab lives in `?type=`
(`all|shipping|fixed|percent`, default `all`). Tab labels SHALL show counts. An empty tab or a failed
`listVouchers()` SHALL render `Empty` with a "Xem sản phẩm" action. The seller-only `VoucherManager` SHALL
remain visible only to principals with scope `listing.write`. The page SHALL have a `loading.tsx` skeleton.

#### Scenario: Tab state survives reload

- **WHEN** a visitor selects "Freeship" and reloads the page
- **THEN** the URL is `/vouchers?type=shipping` and the Freeship tab is still selected

#### Scenario: Empty tab

- **WHEN** a tab has no vouchers
- **THEN** `Empty` is shown with a "Xem sản phẩm" action linking to `/search`

#### Scenario: No voucher save without a backend

- **WHEN** `/vouchers` renders vouchers
- **THEN** no card has a "Lưu mã" or "Đã lưu" control and nothing is read from or written to `localStorage`

#### Scenario: Seller tools are scope gated

- **WHEN** a principal without `listing.write` opens `/vouchers`
- **THEN** `VoucherManager` is not rendered

#### Scenario: Vouchers grid at 375px

- **WHEN** `/vouchers` is rendered at 375px
- **THEN** voucher cards are one per row and the tabs scroll horizontally without page overflow

### Requirement: Search bar suggests, submits and degrades

`SearchBar` SHALL submit to `/search?q=<term>` on Enter or button, SHALL debounce suggestion requests to
`/api/suggest` by 200ms, SHALL support keyboard navigation of suggestions (ArrowUp/ArrowDown/Enter/Escape)
with `role="combobox"`/`listbox`, and SHALL show trending keywords when the input is empty and focused. A
failed suggestion request SHALL hide suggestions without an error toast. An empty submit SHALL NOT navigate.

#### Scenario: Submit navigates to search

- **WHEN** a visitor types "ao khoac" and presses Enter
- **THEN** the browser navigates to `/search?q=ao%20khoac`

#### Scenario: Empty submit is ignored

- **WHEN** the input is empty and the visitor presses Enter
- **THEN** no navigation happens

#### Scenario: Suggestion failure is silent

- **WHEN** `/api/suggest` returns an error
- **THEN** the suggestion list is hidden and the input keeps working

#### Scenario: Keyboard selects a suggestion

- **WHEN** suggestions are open and the visitor presses ArrowDown then Enter
- **THEN** the first suggestion is submitted as the query

### Requirement: Tracking hooks are unchanged

The rework SHALL NOT change the tracking behaviour of discovery routes: `ListingCard` SHALL still be wrapped
in `TrackImpression` (viewport threshold 0.3, one `view_item_list` per card) and SHALL still link image and
title through `TrackLink` (`select_item`); both SHALL carry `listingId`, `placementId`, `impressionId`,
`modelVersion` and `position = index + 1` as `ListingGrid` passes them. `/search` SHALL still render one
`SearchImpressions` with the rendered listing ids and `q`. `RecommendationsRow` SHALL keep placement ids
`home_feed` and `similar_items`. Existing `data-*` attributes and `data-testid` values SHALL be
preserved.

#### Scenario: Card impression still fires once

- **WHEN** a `ListingCard` with `placementId="home_feed"`, `impressionId="i1"`,
  `modelVersion="m1"`, `position=3` scrolls into view
- **THEN** exactly one `view_item_list` event is sent with `itemId`, `placementId`, `impressionId`,
  `modelVersion` and `index=3`

#### Scenario: Card click still records select_item

- **WHEN** a visitor clicks the image or the title of a card
- **THEN** a `select_item` event with the same attribution fields is sent before navigation to
  `/listing/<id>`, and navigation is not blocked

#### Scenario: Search results still emit a batched impression

- **WHEN** `/search?q=ao` renders 24 results
- **THEN** one `view_item_list` event with `itemListId="search_results"`, `query="ao"` and 24 indexed items
  is sent

#### Scenario: Attribution survives the Suspense rework

- **WHEN** the recommendations row streams in after the hero
- **THEN** its cards still carry `placementId="home_feed"` on both impression and click events
