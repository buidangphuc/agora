## Purpose

Defines the product detail page `/listing/[id]` and the shop header card shared with `/shop/[id]` in
`team-frontend`, following the Ant Design Pro Profile › Basic Profile template and composed only from the core
UI components (`ui-components`).

## ADDED Requirements

### Requirement: Page anatomy follows Basic Profile

`/listing/[id]` SHALL render, in order: a `Breadcrumb` (Trang chủ › category › listing title) and a header
`Card` containing the `ImageGallery` and the info column (title as `<h1>`, rating row, `PriceTag`, variants,
quantity, purchase actions, collection/alert/share actions); a `ShopHeaderCard`; a body with an anchor nav and
three stacked, always-visible sections "Chi tiết" (`#specs`), "Đánh giá" (`#reviews`) and "Hỏi đáp" (`#qa`);
and the similar-items row. The page SHALL NOT use content `Tabs`. The "Chi tiết" section SHALL contain a
`Descriptions` specs block (category, stock, ship-from, selected-variant SKU when present) followed by the
description text. The page SHALL be composed only from core components, with a type scale of 12/14/16/20/24px
and brand colour used only for `Mua ngay`, `PriceTag`, real discount badges and the active variant accent.

#### Scenario: Desktop layout shows the header card above the sections

- **WHEN** a buyer opens a published listing at a 1280px-wide viewport
- **THEN** the breadcrumb, the gallery and the info column side by side, the shop header card, the anchor nav
  with the specs, reviews and Q&A sections, and the similar-items row appear in that vertical order, and exactly one `<h1>` contains the listing title

#### Scenario: Specs render in a Descriptions block

- **WHEN** the buyer views the "Chi tiết" section of a listing with stock 12 and a category
- **THEN** a `Descriptions` block lists "Kho hàng: 12 sản phẩm" and the category, followed by the description

#### Scenario: No raw values or off-scale type

- **WHEN** the token lint (`scripts/check-tokens.mjs`) runs over the files of this change
- **THEN** it reports no raw hex value, no arbitrary pixel value and no `text-[9px]`/`text-[10px]`/`text-[11px]`

### Requirement: The page shows only real data

The PDP SHALL NOT display values that have no data source. The rating and review count SHALL come from the
listing rating summary; with zero reviews it SHALL show "Chưa có đánh giá" without stars, and no sold count
SHALL be shown anywhere. A strike-through price and a discount badge SHALL appear only when an active
flash-sale campaign provides a sale price (never computed from the price alone). The "Mall" badge (no title-
or price-based heuristic), sold count, voucher chips, shop stats, response rate and "Online" time SHALL NOT be
rendered. A block with no real data SHALL be hidden.

#### Scenario: A listing with no reviews and no sale shows no invented numbers

- **WHEN** the buyer opens a listing with zero reviews that is not on flash sale
- **THEN** the rating row reads "Chưa có đánh giá", no strike-through price and no "% GIẢM" badge exist, and
  the text "Đã Bán", "Mall" and "Official Store Partner" do not appear

#### Scenario: A listing with no reviews shows no rating stars and no sold count

- **WHEN** the buyer opens a listing with zero reviews
- **THEN** no star `Rate` is rendered for the listing, the text "Chưa có đánh giá" is shown, and no sold count
  ("Đã bán") appears in the header or the shop card

#### Scenario: Mall is never guessed

- **WHEN** the buyer opens a listing priced above 5,000,000 or whose title contains a brand keyword
- **THEN** no Mall badge is rendered

#### Scenario: A flash-sale listing shows the real discount

- **WHEN** the buyer opens a listing with an active flash-sale campaign
- **THEN** the strike-through shows the listing's regular price, the `PriceTag` shows the campaign sale price,
  and the discount badge percentage is computed from those two values

### Requirement: ImageGallery has a fixed aspect and no layout shift

`ImageGallery` SHALL render the stage with `Image` in a fixed 1:1 box and a thumbnail strip of fixed 64px
`Image` boxes. Selecting a thumbnail (click, tap, or Enter/Space) SHALL swap the stage image without changing the
layout size; Left/Right arrow keys on a focused thumbnail SHALL move the selection; the selected thumbnail SHALL
carry `aria-current="true"`. The stage image SHALL load eagerly and thumbnails SHALL load lazily. A failed or
missing image SHALL render the `Image` fallback placeholder in the same box. When the selected variant has an
image, the gallery SHALL show it.

#### Scenario: Clicking a thumbnail changes the stage without shifting layout

- **WHEN** the buyer clicks the third thumbnail of a listing with 4 images
- **THEN** the stage shows the third image and the stage's bounding box is identical before and after

#### Scenario: A missing image uses the fallback

- **WHEN** a listing has no `imageKeys` and no `imageUrl`
- **THEN** the stage renders the `Image` fallback placeholder in a 1:1 box and no external stock-photo URL is
  requested

#### Scenario: Thumbnails are lazy and the stage is eager

- **WHEN** the page markup is rendered for a listing with 4 images
- **THEN** the stage `img` has no `loading="lazy"` and each thumbnail `img` has `loading="lazy"`

### Requirement: Variant selection lives in the URL and drives price and stock

`VariantSelector` SHALL render one `Radio` (button style) per variant, show out-of-stock variants as disabled
with an "Hết hàng" `Tag`, and on selection write `?variant=<id>` with `router.replace(..., { scroll: false })`
when the variant changes price or stock relative to the base listing. The server SHALL resolve the variant from
`searchParams` and derive the displayed price, stock line, SKU and gallery image from it. An unknown `variant`
id SHALL fall back to the first in-stock variant, else the first variant. Changing the variant SHALL reset the
quantity to 1.

#### Scenario: Selecting a variant with a different price updates the URL and the price

- **WHEN** the buyer selects variant "256GB" whose price differs from the base listing
- **THEN** the URL gains `?variant=<id of 256GB>` without a scroll jump or an extra history entry, the
  `PriceTag` shows the 256GB price, and the stock line shows that variant's stock

#### Scenario: A shared variant URL restores the selection

- **WHEN** a buyer opens `/listing/<id>?variant=<id of 256GB>` directly
- **THEN** the 256GB radio is checked and the price, stock and SKU already show the 256GB values in the first
  server-rendered markup

#### Scenario: An unknown variant id falls back safely

- **WHEN** a buyer opens `/listing/<id>?variant=does-not-exist`
- **THEN** the first in-stock variant is selected and the page renders without an error

#### Scenario: An out-of-stock variant cannot be chosen

- **WHEN** a variant has stock 0
- **THEN** its radio is disabled with `aria-disabled="true"`, shows an "Hết hàng" tag, and cannot be selected by
  click or keyboard

### Requirement: Purchase actions give pending, disabled and toast feedback

`PurchasePanel` SHALL render a `QuantityPicker` (min 1, max = selected variant stock), `Thêm vào giỏ` and
`Mua ngay` as `Button`s. Both buttons SHALL be disabled when the selected stock is 0; while an add request is
pending both SHALL be disabled and the clicked one SHALL show `isLoading` with its width preserved. The add SHALL
be a Server Action returning `{ ok, error?, data? }` that calls `revalidatePath`; success SHALL show a success
toast and fire `trackEcommerce("add_to_cart")` once, failure SHALL show an error toast and re-enable the
buttons. `Mua ngay` SHALL add the selected variant and quantity and then navigate to `/checkout`. The add SHALL
use the selected variant id, not the first variant.

#### Scenario: Add to cart shows pending then success

- **WHEN** the buyer clicks `Thêm vào giỏ` with quantity 2 on an in-stock variant
- **THEN** both buttons are disabled with `aria-busy="true"` on the clicked one while the action runs, then a
  success toast "Đã thêm 2 sản phẩm vào giỏ hàng" appears, the buttons re-enable and the cart counter increases

#### Scenario: Add to cart failure is reported and recoverable

- **WHEN** the add action returns `{ ok: false, error: "..." }`
- **THEN** an error toast with that message appears, no `add_to_cart` event is fired, and both buttons are
  enabled again

#### Scenario: Quantity cannot exceed stock

- **WHEN** the selected variant has stock 3 and the buyer presses "+" four times
- **THEN** the `QuantityPicker` value is 3 and the "+" control is disabled

#### Scenario: Buy now adds the selected variant then goes to checkout

- **WHEN** the buyer selects variant "256GB", sets quantity 1 and clicks `Mua ngay`
- **THEN** the cart contains that listing with the 256GB variant id and the browser navigates to `/checkout`

#### Scenario: Out of stock disables both actions

- **WHEN** the selected variant has stock 0
- **THEN** `Thêm vào giỏ` and `Mua ngay` are disabled, a warning `Alert` "Phân loại này đã hết hàng" is shown,
  and the `QuantityPicker` is disabled

### Requirement: Mobile uses a sticky bottom buy bar

At viewport widths below 1024px the page SHALL render a `BuyBar` fixed to the bottom of the viewport (respecting
the safe-area inset) showing the current `PriceTag`, `Thêm vào giỏ` and `Mua ngay`, sharing the pending,
disabled and selected-variant state of the inline panel. The inline purchase action row SHALL be hidden at those
widths so only one set of purchase buttons is visible, and the page body SHALL reserve bottom padding equal to
the bar height so no content is covered. At 1024px and above the `BuyBar` SHALL NOT render.

#### Scenario: The buy bar is visible at 375px and tracks the variant

- **WHEN** a buyer opens a listing at a 375px-wide viewport and scrolls to the reviews section
- **THEN** the bottom bar remains visible with the selected variant's price, and clicking `Thêm vào giỏ` in the
  bar adds the item with the same pending and toast behaviour as the desktop button

#### Scenario: Only one set of purchase buttons is visible per breakpoint

- **WHEN** the viewport is 375px wide
- **THEN** exactly one visible `Thêm vào giỏ` button exists (in the bar); at 1280px exactly one exists (inline)
  and no bottom bar is rendered

#### Scenario: The bar covers no content

- **WHEN** the buyer scrolls to the bottom of the page at 375px
- **THEN** the last element of the page is fully visible above the buy bar

### Requirement: Detail content is stacked sections with an anchor nav

The body SHALL render the specs, reviews and Q&A as stacked, always-visible sections with the ids `specs`,
`reviews` and `qa`, preceded by an anchor nav containing links to `#specs`, `#reviews` and `#qa`. The anchor nav
SHALL be sticky on desktop (>=1024px) and SHALL scroll horizontally at 375px without wrapping; highlighting the
current section is optional. The page SHALL NOT use content `Tabs` and SHALL NOT keep tab state in the URL.
All three sections SHALL be present in the server markup; the reviews and Q&A lists SHALL stream in their own
`Suspense` boundaries.

#### Scenario: Anchor links point at the sections

- **WHEN** a buyer opens `/listing/<id>`
- **THEN** the anchor nav has links `#specs`, `#reviews` and `#qa`, each matching a section id present in the
  first server-rendered markup, and activating "Đánh giá" scrolls to `#reviews`

#### Scenario: The anchor nav is sticky on desktop only

- **WHEN** the page is rendered at 1280px and at 375px
- **THEN** the anchor nav is sticky at 1280px, and at 375px it is not sticky and scrolls horizontally without
  page overflow

### Requirement: Reviews use Rate, a URL filter and pagination

The "Đánh giá" section SHALL show the average with a read-only `Rate`, the count, the star breakdown as `Progress`
bars, and filter controls for All and 5..1 stars that write `?rating=`. The list SHALL show each review with an
`Avatar`, a read-only `Rate`, the comment, photos as `Image` (fixed 64px box, lazy), the "Đã mua hàng" `Tag` when
verified, and a helpful button. The list SHALL be paginated at 10 per page with a link-based `Pagination`
(`?rpage=`). Marking a review helpful SHALL be a Server Action with pending state and an error toast on failure.
"Viết đánh giá" SHALL open the review `Modal` whose rating input is `Rate`; submit shows pending, then a success
toast and the new review after `revalidatePath`, or an inline `Alert` on error.

#### Scenario: Filtering by stars is shareable

- **WHEN** the buyer clicks "4 Sao" in the reviews section
- **THEN** the URL contains `rating=4`, only 4-star reviews are listed, and reloading the URL shows the same list

#### Scenario: A filter with no matches shows Empty with a way out

- **WHEN** no 2-star review exists and the buyer opens `?rating=2`
- **THEN** an `Empty` "Không có đánh giá 2 sao" with a `Xem tất cả` action is shown, and that action removes
  `rating` from the URL

#### Scenario: Marking helpful is single-use and reports failure

- **WHEN** the buyer clicks `review-helpful` on a review and the action succeeds
- **THEN** the count increases by one and the button is disabled; if the action fails the count reverts and an
  error toast is shown

#### Scenario: Reviews paginate at 10

- **WHEN** a listing has 23 reviews and the buyer opens `?rpage=3`
- **THEN** 3 reviews are listed and the `Pagination` marks page 3 as current

#### Scenario: Submitting a review shows pending then success

- **WHEN** the buyer picks 5 stars in the `Rate` input, types a comment and submits the review modal
- **THEN** the submit button shows `isLoading` and the modal controls are disabled, then a success toast shows,
  the modal closes and the new review appears in the list

### Requirement: Q&A uses core components with action feedback

The "Hỏi đáp" section SHALL list questions and answers (`Avatar`, "Shop" `Tag` on shop replies) and keep the
`data-testid` values `qa-item`, `qa-answer`, `qa-ask-form`, `qa-login-prompt`, `qa-empty` and `qa-answer-toggle`.
Asking and answering SHALL be Server Actions returning `{ ok, error?, data? }` with `revalidatePath`, a pending
`Button isLoading`, a disabled submit while the text is empty or pending, a success toast, and on failure an
error toast plus an inline `Alert`. A logged-out visitor SHALL see a login prompt linking to
`/login?returnUrl=/listing/<id>`. With no questions the section SHALL show an `Empty` with an ask action.

#### Scenario: Asking a question shows pending and a toast

- **WHEN** a logged-in buyer types a question and submits
- **THEN** the submit button shows `isLoading`, then a success toast appears, the textarea clears and the
  question is listed

#### Scenario: Empty submit is blocked

- **WHEN** the question textarea is empty
- **THEN** the submit button is disabled and no request is made

#### Scenario: A guest is prompted to log in

- **WHEN** a logged-out visitor views the Q&A section
- **THEN** `qa-login-prompt` is shown with a link to `/login?returnUrl=/listing/<id>` and no ask form renders

### Requirement: AI review summary streams and hides gracefully

`AiReviewSummary` SHALL be rendered inside the "Đánh giá" section in a `Suspense` boundary with a `Skeleton` of the
card's height, and SHALL NOT delay the first byte of the page. When team-ai is unavailable, returns no summary,
or the listing has no reviews, the block SHALL render nothing (no error, no empty card, no layout gap).

#### Scenario: The summary does not block the page

- **WHEN** team-ai responds slowly (>2s) to SummarizeReviews
- **THEN** the page's header, price and actions are visible before the summary, and a `Skeleton` holds the
  summary's space until it resolves

#### Scenario: An unavailable AI service hides the block

- **WHEN** SummarizeReviews fails with UNAVAILABLE
- **THEN** the reviews section renders normally with no AI block, no error text and no leftover skeleton

### Requirement: The similar-items row streams, hides gracefully and keeps attribution

`RecommendationsRow` seeded with the current listing SHALL be rendered in a `Suspense` boundary with a
`Skeleton` row, SHALL render nothing when the recommendation call fails or returns no items, and SHALL continue
to render `ListingGrid` with `placementId="similar_items"` so impression and click events carry the same
placement attribution as before.

#### Scenario: A slow recommendation call does not block the page

- **WHEN** the recommendation service is slow
- **THEN** the page renders fully with a one-row `Skeleton` in the row's place and no layout shift when the
  cards arrive

#### Scenario: Unavailable recommendations are hidden

- **WHEN** the recommendation service returns UNAVAILABLE
- **THEN** the page renders without the "Gợi ý cho bạn" row and without an error state

### Requirement: ShopHeaderCard is shared by the PDP and the storefront

`ShopHeaderCard` SHALL render an `Avatar`, the real shop display name (from the `shop-display-name` contract; "Shop #" followed by the first 6 characters of the seller id only when the name is empty), a `Statistic` row with the shop rating from the shop
rating summary (keeping `data-testid="shop-rating-summary"`), and actions (`ChatWithSellerButton`, `Xem Shop` on
the PDP; `FollowSellerButton`, `ChatWithSellerButton` on the storefront). The PDP SHALL use `variant="compact"`;
`/shop/[id]` SHALL use `variant="hero"` with the storefront banner and tagline when present. The storefront price
sort tabs SHALL be driven by `?sort=price_asc|price_desc` (default: all). No fabricated rating, response-rate or
tenure value SHALL be rendered in either place.

#### Scenario: The shop card shows the real shop name

- **WHEN** the seller's shop display name is "Cửa hàng Hoa Mai"
- **THEN** the PDP shop card and the storefront header show "Cửa hàng Hoa Mai" and not "Shop #"

#### Scenario: An empty shop name falls back

- **WHEN** the shop display name is empty
- **THEN** the shop card shows "Shop #" followed by the first 6 characters of the seller id

#### Scenario: The PDP shop card links to the storefront

- **WHEN** a buyer clicks `Xem Shop` in the PDP shop card
- **THEN** the browser navigates to `/shop/<sellerId>` whose header is the same component in hero variant

#### Scenario: Storefront sort is in the URL

- **WHEN** a buyer selects "Giá: Thấp đến Cao" on `/shop/<id>`
- **THEN** the URL contains `sort=price_asc` and the grid is ordered by ascending price; reloading keeps it

#### Scenario: A shop with no ratings shows an empty rating

- **WHEN** the seller has zero reviews
- **THEN** the shop card shows "Chưa có đánh giá" instead of "0.0 / 5.0"

### Requirement: Loading, not-found and error states

`/listing/[id]` SHALL have a `loading.tsx` rendering `Skeleton`s with the footprints of the breadcrumb, header
card, shop card and anchor nav. An unknown listing id (gateway NotFound) SHALL render a `Result` with status 404,
title "Không tìm thấy sản phẩm", a primary action "Về trang chủ" (`/`) and a secondary action "Tìm sản phẩm khác"
(`/search`), with HTTP status 404. Any other fetch failure SHALL render an error `Alert` with a "Thử lại" action
calling `reset()`.

#### Scenario: An unknown listing renders Result 404

- **WHEN** a buyer opens `/listing/does-not-exist`
- **THEN** the response status is 404 and the page shows the `Result` 404 with "Về trang chủ" and "Tìm sản phẩm
  khác" actions

#### Scenario: A gateway failure shows a recoverable error

- **WHEN** the listing fetch fails with a non-NotFound error
- **THEN** an error `Alert` with a "Thử lại" button is shown, and clicking it re-renders the route

#### Scenario: The loading skeleton matches the final layout

- **WHEN** the route is navigated to and `loading.tsx` is shown, then the page resolves
- **THEN** the gallery stage, title block and anchor nav keep the same top offsets and the measured layout shift is 0

### Requirement: Mutation actions follow the Server Action contract

Every mutation on the PDP (add to cart, mark helpful, create review, ask question, answer question) SHALL be a
Server Action returning `{ ok: boolean; error?: string; data?: T }` and calling `revalidatePath` for the affected
route. The existing `message` field MAY remain as a deprecated alias in this phase. Components SHALL read
`error` for failure text.

#### Scenario: A failed action returns a structured error

- **WHEN** `askQuestionAction` is called and the gateway rejects it
- **THEN** it resolves to `{ ok: false, error: <non-empty string> }` and does not throw

#### Scenario: A successful mutation revalidates the page

- **WHEN** `markReviewHelpfulAction` succeeds for listing `L`
- **THEN** it resolves to `{ ok: true }` and `revalidatePath` was called with `/listing/L`

### Requirement: Existing tracking hooks keep firing

The redesign SHALL NOT alter `TrackView`, `TrackLink`, `TrackImpression`, `AnalyticsProvider`, the
`placementId` attribution on the similar-items row, or the `trackEcommerce` events. Opening a PDP SHALL still
fire exactly one `view_item` event with the listing id and path; cards in the similar-items row SHALL still fire
an impression with `placementId="similar_items"`, `position` and any `impressionId`/`modelVersion`, and a
click on one SHALL still fire `select_item` with the same attribution; a successful add to cart SHALL still fire
one `add_to_cart` with the same payload shape.

#### Scenario: One view event per PDP load

- **WHEN** a buyer opens a listing and then switches variants and uses the anchor nav
- **THEN** exactly one `view_item` event for that listing was sent, and variant changes and anchor navigation sent none

#### Scenario: Similar-item impression and click keep attribution

- **WHEN** the similar-items row scrolls into view and the buyer clicks the second card
- **THEN** impressions with `placementId="similar_items"` and positions 1..n were sent, and a `select_item`
  with `placementId="similar_items"` and `position=2` was sent before navigation

#### Scenario: Add to cart event is unchanged

- **WHEN** the buyer adds quantity 2 of a listing priced 100000
- **THEN** one `add_to_cart` event is sent with `value=200000`, `currency="VND"` and the item's id, name, price,
  quantity and category
