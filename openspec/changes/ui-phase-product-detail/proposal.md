## Why

`/listing/[id]` is the highest-intent screen of the funnel and the weakest one in the UI. A single 431-line
server page inlines the gallery, price box, variant chips, shop card, specs, and perks with raw hex values
(`bg-[#d0011b]`), 9-11px micro-text, and emoji icons, which violates `UI_SYSTEM_DESIGN.md` principles 1 and 3.
It also has product-detail defects that mislead or lose the buyer:

- Fabricated data is shown as fact: rating "5.0", "125 Đánh Giá", "1.2k Đã Bán", a fake "-20%" with a
  `price * 1.25` strike-through price, a "Mall" badge guessed from the title, a hardcoded shop name
  "Official Store Partner", fixed vouchers and a fixed 99% response rate.
- The variant chips on the page are inert `<button>`s (no state, no URL, no effect on price or stock), while a
  working `VariantSelector` client component exists but is unused. `AddToCartButton` always buys
  `variants[0]`.
- The thumbnails are not clickable, the gallery has no fixed-aspect `Image` fallback, and there is no mobile
  buy bar.
- Reviews filter and Q&A state are local `useState`, so they are not shareable or back-button friendly.
- `summarizeReviews` is awaited serially after the parallel fetch, blocking first byte on team-ai.
- An unknown listing falls through to the generic `notFound()` page with no recovery path specific to a product.

This change rebuilds the PDP on the core components (`ui-core-components`) following the Ant Design Pro
**Profile › Basic Profile** template: a header card (gallery + title + price + actions), a Descriptions
specs block, and Tabs for the long-tail content.

Repos: `team-frontend` (UI) and `platform-e2e` (extends existing features). No other repo is touched.

## What Changes

- New `ImageGallery` (client leaf): `Image` main stage with a fixed 1:1 aspect, thumbnail strip, keyboard and
  swipe navigation. Replaces the inline `<img>` + dead thumbnails.
- `VariantSelector` rewritten as a client leaf island that writes the selected variant to the URL
  (`?variant=<id>`); price, stock, gallery image and SKU are derived on the server from that param.
- `PriceTag` is the only price renderer; discount and strike-through are shown only when real data exists.
- `ShopHeaderCard` extracted (server) and shared by the PDP shop block and the `/shop/[id]` storefront
  header, built from `Card`, `Avatar`, `Tag`, `Statistic`, `Rate`.
- Specs block (`Descriptions`): category, stock, ship-from, variant SKU, from real fields only.
- Body content as stacked, always-visible sections (no content `Tabs`): specs and description (`#specs`),
  reviews (`#reviews`, with `Rate`, rating filter in `?rating=`, `Pagination` in `?rpage=`) and Q&A (`#qa`),
  with an anchor nav (links to `#specs`, `#reviews`, `#qa`, sticky on desktop) at the top of the body.
- `AiReviewSummary` streamed in `Suspense` with a `Skeleton`, and hidden entirely when team-ai is unavailable.
- Similar-items row (`RecommendationsRow`) wrapped in `Suspense` with a `Skeleton` grid, hidden when empty.
- `AddToCart` / `Buy now` rebuilt with `Button isLoading`, `QuantityPicker` (min 1, max = stock), toast feedback,
  and one shared pending state; `Buy now` = add then navigate.
- Mobile (375px): sticky bottom buy bar (price, "Thêm vào giỏ", "Mua ngay"); desktop (>=1024px):
  actions inline in the header card.
- `src/app/listing/[id]/loading.tsx` (Skeleton of the full anatomy), `not-found.tsx` (`Result` 404),
  `error.tsx` (`Alert` + retry).
- Remove all fabricated PDP data (see Why); a value with no data source is omitted, not invented: no fake
  rating or sold count, no computed strike-through price, no fake discount badge, no Mall heuristic, no fake
  shop stats or response rate, no hard-coded voucher chips. A listing with no reviews shows no rating stars and
  no sold count.
- Shop name: the real shop display name is rendered in `ShopHeaderCard` (PDP and storefront); "Shop #<6 chars>"
  remains only as a fallback for an empty name.

## Dependencies

- Requires the backend change `shop-display-name` (adds a shop/seller display name to the contract). The
  `ShopHeaderCard` real-name requirement cannot be implemented or verified until that change lands.

## Capabilities

### New Capabilities
- `ui-product-detail`: anatomy, states, URL state and mobile behaviour of the product detail page and the
  shared shop header card.

### Modified Capabilities
None. `tracking.emit-view`, `recommendations.for-you-row` and `shop.storefront` keep their behaviour; their
existing e2e scenarios are extended, not rewritten.

## Non-goals

- No backend, gateway or proto changes in this change. The shop display name is delivered by the separate
  change `shop-display-name`. Sold count, original/compare-at price and response rate are NOT added to the
  contract: where the contract has no field, the UI hides the element (see design Decision 7).
- Follow-up backend change: sold count, an "official store" (Mall) flag and shop response rate; until then
  those elements are hidden.
- No `antd` dependency.
- No routes owned by another phase: `/cart`, `/checkout` (cart-checkout), `/search`, home (discovery),
  `/account/*`. Buy now only navigates to them.
- `ListingCard`, `ListingGrid`, `SearchBar`, `FilterSidebar`, `SortBar` belong to `ui-phase-discovery`; the
  similar-items row reuses `ListingGrid` unchanged.
- No change to tracking hooks: `TrackView`, `TrackLink`, `TrackImpression`, `AnalyticsProvider`,
  `placementId="pdp_similar_items"` attribution, `trackEcommerce("add_to_cart"|"view_item"|"select_item")`
  and every `data-testid` used by existing e2e.
- Seller-side PDP management, image zoom/lightbox video, and share-link redesign (`ShareButton` keeps its
  behaviour, only restyled by tokens).

## Impact

- `team-frontend/src/app/listing/[id]/{page,loading,not-found,error}.tsx`, `src/app/shop/[id]/page.tsx`,
  `src/features/listing/{ImageGallery,VariantSelector,qa/QASection}.tsx`,
  `src/features/shop/{ShopHeaderCard,ShopStorefrontView}.tsx`, `src/features/cart/AddToCartButton.tsx`
  (+ `BuyBar`), `src/features/review/{ReviewSection,AiReviewSummary,ReviewModal}.tsx`,
  `src/features/recommendations/RecommendationsRow.tsx` (wrapper only).
- `team-frontend/FEATURES.yaml` (new `listing.pdp-*` entries), `platform-e2e/tests/e2e/features/` (extends
  `buyer/review_ratings_filter`, `engagement/{qa,rich_reviews}`, `recommendations/recommendations`,
  `shop/shop_storefront`, `buyer/purchase`; adds `frontend/product_detail.feature`).
- Depends on `ui-foundation` (tokens, `not-found`/`error`/`loading` shells) and `ui-core-components`.
  Independent of the other phases.
