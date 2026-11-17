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
Features without a backend SHALL be hidden, not simulated: "Lưu mã" on `/vouchers`; no `localStorage` voucher wallet
SHALL exist. The "Bán chạy" sort and the rating filter on `/search` are backed by indexed `sold_count` and `rating_avg`
and are offered.

#### Scenario: No fabricated markers on the home page

- **WHEN** `/` is rendered with listings that have no review, discount or campaign data
- **THEN** the document contains no strike-through price, no "-NN%" badge, no "ĐÃ BÁN" text, no countdown, no
  MALL badge and no star rating

#### Scenario: Mall is never guessed

- **WHEN** a listing costs more than 5,000,000 or its title contains a brand keyword
- **THEN** its card shows no MALL badge

#### Scenario: Unsupported features are hidden

- **WHEN** `/search` and `/vouchers` are rendered
- **THEN** the sort options include "Bán chạy" and no voucher card has a "Lưu mã" button

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
`q, category, seller, minRating, minPrice, maxPrice, sort, page`. Changing any filter or sort SHALL reset `page`.
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

- **WHEN** `/search?q=ao&category=c1&minRating=4` is rendered
- **THEN** three closable `Tag`s are shown, and activating the category tag navigates to
  `/search?q=ao&minRating=4`

#### Scenario: Clear all filters

- **WHEN** at least one filter is active and the buyer activates "Xóa tất cả bộ lọc"
- **THEN** the URL becomes `/search` (keeping `q` only if present) and all tags disappear

#### Scenario: Sort options match what the server honours

- **WHEN** the `SortBar` is rendered
- **THEN** it offers Liên quan, Mới nhất, Bán chạy, Giá thấp đến cao and Giá cao đến thấp, selecting "Bán chạy" navigates to a URL
  with `sort=best_selling`, and the active link has `aria-current="page"`

#### Scenario: Best-selling sort orders results by sold count

- **GIVEN** listings with `sold_count` 7, 3 and 0 match the keyword
- **WHEN** a buyer selects "Bán chạy"
- **THEN** the results appear in the order 7, 3, 0 and opening `/search?q=<keyword>&sort=best_selling` in a new tab shows the same order

#### Scenario: An unknown sort value falls back to relevance

- **WHEN** a buyer opens `/search?q=<keyword>&sort=top_rated`
- **THEN** the page renders the relevance order with "Liên quan" marked `aria-current="page"` and no error

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

The rework SHALL NOT change the tracking behaviour of discovery routes except that impressions are coalesced per list: every rendered `ListingGrid` SHALL announce its visible cards as one `view_item_list` per list instance (viewport threshold 0.3), with items in grid order, `index` 1..N, a shared `item_list_id` equal to the list's placement id and one shared event group id. `ListingCard` SHALL still link image and title through `TrackLink` (`select_item`); both the list impression items and the click SHALL carry `listingId`, `placementId`, `impressionId`, `modelVersion` and `position = index + 1` as `ListingGrid` passes them. `/search` SHALL announce its results through `ListingGrid` with `item_list_id = "search_results"` and `q` as the search term; `SearchImpressions` SHALL NOT be used. `RecommendationsRow` SHALL keep placement ids `home_feed` and `similar_items`. Existing `data-*` attributes and `data-testid` values SHALL be preserved.

#### Scenario: Card impression still fires once

- **WHEN** a `ListingGrid` with `placementId="home_feed"` and 3 visible cards (`impressionId` and `modelVersion` set per card) scrolls into view
- **THEN** exactly one `view_item_list` event is sent with 3 items in grid order, `index` 1, 2, 3, each carrying `itemId`, `impressionId` and `modelVersion`, all with `item_list_id="home_feed"` and one shared event group id

#### Scenario: Card click still records select_item

- **WHEN** a visitor clicks the image or the title of a card
- **THEN** a `select_item` event with the same attribution fields is sent before navigation to `/listing/<id>`, and navigation is not blocked

#### Scenario: Search results still emit a batched impression

- **WHEN** `/search?q=ao` renders 24 results
- **THEN** one `view_item_list` event with `item_list_id="search_results"`, `search_term="ao"` and 24 items indexed 1..24 in grid order is sent, and no `SearchImpressions` component is rendered

#### Scenario: Attribution survives the Suspense rework

- **WHEN** the recommendations row streams in after the hero
- **THEN** its list impression carries `item_list_id="home_feed"` and its card click events carry `placementId="home_feed"`

### Requirement: Search page 1 shows labelled sponsored slots

`/search` SHALL, on page 1 when the organic search succeeded with at least one item, request up to 2 sponsored slots
from `SponsoredService.ListSponsoredSlots` with placement `search` and the selected category, and render each as a
listing card labelled "Tài trợ" before the organic grid. A sponsored listing that also appears in the organic items of
that page SHALL be shown only as the sponsored slot. The page SHALL NOT request slots for page 2 or later.

#### Scenario: Sponsored slot is shown labelled in its category

- **WHEN** a guest opens `/search?category=<c>` where an ACTIVE campaign exists for a published listing L in category c
- **THEN** the page shows L in an element `sponsored-slot` that contains the text "Tài trợ", positioned before the first organic card
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature` `Sponsored slot is shown labelled in its category`

#### Scenario: Sponsored listing is not duplicated in organic results

- **WHEN** the sponsored listing L also matches the organic search of page 1
- **THEN** L appears exactly once on the page, inside `sponsored-slot`, and zero times in `search-results`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature` `Sponsored listing is not duplicated in organic results`

#### Scenario: Page 2 has no sponsored slots

- **WHEN** a visitor opens `/search?page=2`
- **THEN** zero `sponsored-slot` elements are rendered and `ListSponsoredSlots` is not called
- **VERIFIED BY**: `team-frontend/src/features/search/SearchBlocks.test.tsx` `Page 2 has no sponsored slots`

#### Scenario: Slot service failure leaves organic results intact

- **WHEN** `ListSponsoredSlots` fails with UNAVAILABLE or does not answer within 150 ms while the organic search succeeds
- **THEN** the page responds 200 with the organic grid, count and pagination unchanged and zero `sponsored-slot` elements
- **VERIFIED BY**: `team-frontend/src/features/search/SearchBlocks.test.tsx` `Slot service failure leaves organic results intact`

#### Scenario: No slots when the organic search has no results

- **WHEN** the organic search returns zero items for the query
- **THEN** the page shows "Không tìm thấy sản phẩm" and zero `sponsored-slot` elements
- **VERIFIED BY**: `team-frontend/src/features/search/SearchBlocks.test.tsx` `No slots when the organic search has no results`

#### Scenario: Unpublished sponsored listing is dropped

- **WHEN** a returned slot points to a listing that `GetListing` reports as missing or not published
- **THEN** that slot is not rendered and the other slots still render
- **VERIFIED BY**: `team-frontend/src/features/ads/data.test.ts` `drops slots whose listing is missing or unpublished`

#### Scenario: At most two slots are rendered

- **WHEN** the service returns 4 slots for the request
- **THEN** exactly 2 `sponsored-slot` elements are rendered, the first 2 in response order, and the request carried limit 2
- **VERIFIED BY**: `team-frontend/src/features/ads/data.test.ts` `requests and renders at most two slots`

### Requirement: Home feed shows labelled sponsored slots

The home feed block SHALL request up to 2 sponsored slots with placement `home` and no category, render each as a
listing card labelled "Tài trợ" before the feed items, and remove from the feed items any listing shown as a
sponsored slot. A sponsored lookup failure SHALL NOT change the rest of the home page.

#### Scenario: Sponsored slot is shown on the home feed

- **WHEN** a guest opens `/` while an ACTIVE campaign exists whose slot is served for placement `home`
- **THEN** the feed section "Gợi ý hôm nay" contains a `sponsored-slot` with the text "Tài trợ" before its first organic card
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature` `Sponsored slot is shown on the home feed`

#### Scenario: Home dedupes the sponsored listing

- **WHEN** the sponsored listing also occurs in the feed items
- **THEN** it appears once on the page, inside `sponsored-slot`
- **VERIFIED BY**: `team-frontend/src/features/home/blocks.test.tsx` `Home dedupes the sponsored listing`

#### Scenario: Home slot failure does not break the feed

- **WHEN** the sponsored lookup fails or exceeds 150 ms
- **THEN** the feed renders its organic items and zero `sponsored-slot` elements, and the other home blocks still render
- **VERIFIED BY**: `team-frontend/src/features/home/blocks.test.tsx` `Home slot failure does not break the feed`

### Requirement: A click on a sponsored slot is charged once and never blocks navigation

A click or auxiliary click on an anchor inside a sponsored slot SHALL send its impression token once to
`POST /api/ads/click`, which SHALL call `SponsoredService.RecordAdClick` and answer 204 whether or not a charge was
booked. The anchor SHALL navigate to `/listing/[id]` through a plain link; the frontend SHALL NOT charge on render,
prefetch or any GET request.

#### Scenario: Click charges the campaign once and opens the listing

- **WHEN** a buyer clicks the sponsored slot of campaign C (bid 1 000, budget remaining 5 000) on `/search`
- **THEN** the browser lands on `/listing/<L>` and C's spent equals 1 000 (remaining 4 000) when read back through `ListMyAdCampaigns`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature` `Click charges the campaign once and opens the listing`

#### Scenario: Second click on the same slot charges nothing

- **WHEN** the buyer returns to the same results page without reloading and clicks the same slot again
- **THEN** C's spent is still 1 000 and the navigation to `/listing/<L>` still works
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature` `Second click on the same slot charges nothing`

#### Scenario: Rendering the page charges nothing

- **WHEN** a visitor loads `/search?category=<c>` with a sponsored slot and never clicks
- **THEN** C's spent stays 0 and `RecordAdClick` is not called
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature` `Rendering the page charges nothing`

#### Scenario: Sponsored slot: a seller clicking their own ad is not charged

- **WHEN** the seller who owns C is logged in, opens `/search?category=<c>` and clicks the sponsored slot
- **THEN** the navigation to `/listing/<L>` works and C's spent stays 0
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature` `Seller clicking their own ad is not charged`

#### Scenario: Click beacon fires once per gesture and does not prevent navigation

- **WHEN** a user clicks, or middle-clicks, an anchor inside a `sponsored-slot` once
- **THEN** exactly one beacon with that slot's token is sent and the click event is not default-prevented
- **VERIFIED BY**: `team-frontend/src/features/ads/SponsoredClickGuard.test.tsx` `sends one beacon and keeps default navigation`

#### Scenario: Click on an organic card sends no beacon

- **WHEN** a user clicks an anchor in `search-results` that is not inside a `sponsored-slot`
- **THEN** no request is sent to `/api/ads/click`
- **VERIFIED BY**: `team-frontend/src/features/ads/SponsoredClickGuard.test.tsx` `organic clicks send no beacon`

#### Scenario: Click route rejects a malformed body

- **WHEN** `POST /api/ads/click` receives an empty body, a missing token, a token over 256 characters or a token with characters outside the allowed set
- **THEN** it responds 400 and `RecordAdClick` is not called
- **VERIFIED BY**: `team-frontend/src/app/api/ads/click/route.test.ts` `rejects malformed bodies with 400`

#### Scenario: Click route hides the billing outcome

- **WHEN** `RecordAdClick` returns `charged = false` or fails with an error for a well-formed token
- **THEN** `POST /api/ads/click` still responds 204 with an empty body
- **VERIFIED BY**: `team-frontend/src/app/api/ads/click/route.test.ts` `answers 204 for charged, uncharged and failed calls`

#### Scenario: Click route forwards the session and does not log ids

- **WHEN** a logged-in buyer's click reaches `POST /api/ads/click`
- **THEN** `RecordAdClick` is called with the session bearer token, and the log line contains only the outcome class and none of the token, listing id or user id
- **VERIFIED BY**: `team-frontend/src/app/api/ads/click/route.test.ts` `forwards session and logs outcome class only`

#### Scenario: GET on the click route is not allowed

- **WHEN** a client sends `GET /api/ads/click?token=x`
- **THEN** the response is 405 and `RecordAdClick` is not called
- **VERIFIED BY**: `team-frontend/src/app/api/ads/click/route.test.ts` `GET is rejected with 405`

### Requirement: Gateway wrapper maps slots and swallows failures

`src/lib/gateway/promotion.ts` SHALL expose `listSponsoredSlotSet` mapping `ListSponsoredSlotsResponse.slots` to
`{ listingId, campaignId, impressionToken }` with the request fields `placement`, `categoryId` and `limit`, and
SHALL return an empty list on any error; and `recordAdClick` SHALL call `RecordAdClick` with the token.

#### Scenario: Slots are mapped from the response

- **WHEN** the response holds 2 slots with listing, campaign and token values
- **THEN** the wrapper returns 2 entries with the same values in the same order and sent placement, category and limit as given
- **VERIFIED BY**: `team-frontend/src/lib/gateway/promotion.test.ts` `listSponsoredSlotSet maps slots`

#### Scenario: A failing call returns no slots

- **WHEN** the underlying RPC rejects with UNAVAILABLE
- **THEN** the wrapper resolves to an empty list and does not throw
- **VERIFIED BY**: `team-frontend/src/lib/gateway/promotion.test.ts` `listSponsoredSlotSet returns empty on error`
