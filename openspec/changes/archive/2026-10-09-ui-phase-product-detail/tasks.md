## 1. Code — team-frontend: pure logic and actions

- [x] 1.1 Add `src/features/listing/pdp.ts` with `resolveVariant(listing, variantId)`, `parsePdpParams(searchParams)` (`variant`, `rating`, `rpage`, `sort` with defaults and bounds; no `tab`) and `paginate(items, page, size)`; verify a Vitest file covers unknown variant, out-of-stock-first variant, invalid rating/page, and the "write to URL only when price or stock differs" helper, all green
- [x] 1.2 Migrate the listing/review/Q&A actions it owns (`markReviewHelpfulAction`, `createReviewAction`, `askQuestionAction`, `answerQuestionAction`) to the shared `ActionResult<T>` from `src/lib/action-result.ts` (ui-foundation); `addToCartAction` is owned and migrated by ui-phase-cart-checkout and only consumed here and keep their `revalidatePath` calls; verify the existing `actions.test.ts` files are updated, assert `error` on failure and the revalidated path, and pass
- [x] 1.3 Remove fabricated PDP data: rating/count from `ratingSummary`, delete `originalPrice = price * 1.25`, the Mall heuristic, voucher chips, fake shop name/response rate/stats, any sold count and the Unsplash fallback; verify `grep -n "1.25\|isMall\|Official Store Partner\|Đã Bán" src/app/listing` returns nothing and a unit test renders the zero-review, no-sale case with no stars, no sold count, no strike-through and no Mall badge

## 2. Code — team-frontend: gallery, price and variants

- [x] 2.1 Add `ImageGallery` (client): `Image` stage 1:1 eager, 64px lazy thumbnails, click/Enter/Space/arrow selection with `aria-current`, fallback placeholder, variant image override; verify an RTL test covers thumbnail swap with an unchanged bounding box, fallback on empty images, and eager-vs-lazy attributes
- [x] 2.2 Rewrite `VariantSelector` (client) over `Radio` button style: disabled out-of-stock with "Hết hàng" `Tag`, `router.replace(url, { scroll: false })` for `?variant=`, quantity reset on change; verify an RTL test covers URL write, disabled variant, and no write when price and stock equal the base
- [x] 2.3 Render price, stock line and SKU from `resolveVariant` using `PriceTag` and `Descriptions`-style info rows in `page.tsx`, with the strike-through only for a real flash-sale price; verify a render test for a variant with a different price and for a flash-sale listing

## 3. Code — team-frontend: purchase panel and mobile buy bar

- [x] 3.1 Add `PurchasePanel` (client) with `QuantityPicker` (min 1, max stock), `Button isLoading` for `Thêm vào giỏ` and `Mua ngay`, one shared pending state via context, toasts, selected variant id, `trackEcommerce("add_to_cart")` payload unchanged, `Mua ngay` -> `/checkout`; verify RTL tests for pending/disabled/success/failure/quantity-bound/out-of-stock and that the add event fires once with the same payload as before
- [x] 3.2 Add `BuyBar` (client, `<lg` only, fixed bottom, safe-area padding, body bottom padding) sharing the panel context, and hide the inline action row `<lg`; verify an RTL test shows one set of buttons per breakpoint class and the bar reads the selected variant's price
- [x] 3.3 Keep `AddToCartButton` as a thin re-export of `PurchasePanel` for existing imports; verify `npx tsc --noEmit` passes and `grep -rn AddToCartButton src` shows no broken import

## 4. Code — team-frontend: anchor nav, specs, reviews, Q&A

- [x] 4.1 Build the body as stacked sections `#specs`, `#reviews`, `#qa` (each with its `id`, all rendered, reviews and Q&A lists in their own `Suspense` with a `Skeleton`) and an anchor nav of links to them, sticky on desktop; no `Tabs` and no `?tab=` parsing; verify an RTL test finds the three anchor links with the matching section ids, all three sections in the first server markup, and sticky classes on `lg`
- [x] 4.2 Build the "Chi tiết" section: `Descriptions` specs (category, stock, ship-from, SKU) + description; verify a render test asserts "Kho hàng: 12 sản phẩm" in a `Descriptions`
- [x] 4.3 Rework `ReviewSection` into server markup + client leaves: `Rate`, `Progress` breakdown, `?rating=` filter links, `Avatar`, `Image` photos (64px, lazy), `Tag` verified, `ReviewHelpfulButton` with pending + toast, `Pagination` (`?rpage=`, 10 per page), `Empty` states, "ĐÁNH GIÁ SẢN PHẨM" heading and "Tất cả" filter text kept; verify tests for filter URL, empty filter recovery, pagination page 3 of 23, helpful success and revert-on-failure, and preserved `data-testid`s
- [x] 4.4 Update `ReviewModal` to the `Rate` input, `Button isLoading`, success toast and inline `Alert` on error; verify an RTL test of pending -> success and of the error path
- [x] 4.5 Rework `QASection` with `Avatar`, `Tag` "Shop", `Button isLoading`, disabled-when-empty, toast + `Alert`, `Empty` with ask action, login prompt kept; verify RTL tests for each state and preserved `data-testid`s

## 5. Code — team-frontend: streaming, shop header, exception routes

- [x] 5.1 Wrap `AiReviewSummary` (async fetch inside) and `RecommendationsRow` in `Suspense` with `Skeleton` fallbacks, remove the serial `await summarizeReviews`, keep `null` on failure and `placementId="similar_items"`; verify tests that a rejected fetch renders nothing and that the placement prop is still passed to `ListingGrid`
- [x] 5.2 Add `src/app/listing/[id]/loading.tsx` with Skeletons matching the final footprints; verify a render test and a Playwright CLS measurement of 0 on a seeded listing (measured on a production build, 1.6 Mbit/s, 300 ms, 4x CPU, seeded listings: CLS 0 at 375px and 0 at 1280px, before: 0.1688 / 0.0356, the shifting element was the gallery Image fallback swapping in for the broken `<img src="">`; the skeleton breadcrumb now mirrors the 2-line mobile / 1-line desktop footprint)
- [x] 5.3 Add `src/app/listing/[id]/not-found.tsx` (`Result` 404, "Về trang chủ", "Tìm sản phẩm khác") and call `notFound()` only when `getListing` returns null; add `error.tsx` (`Alert` + `reset()`); verify `curl -s -o /dev/null -w '%{http_code}' :3000/listing/does-not-exist` prints 404 and a Vitest test renders the error boundary with a working retry (Verified on the agora stack: `curl` of `/listing/does-not-exist` prints 404 (fixed by 6693b91), the 'unknown listing' scenario passes, and the Vitest error-boundary test calls `reset()`.)
- [x] 5.4 Extract `ShopHeaderCard` (server; `compact` and `hero` variants) from the PDP block and `ShopStorefrontView`, render the real shop display name from the `shop-display-name` contract (fallback "Shop #<6 chars>" only for an empty name; depends on that change), replace fake stats with `getShopRatingSummary`, keep `shop-rating-summary`; move the storefront sort tabs to `?sort=` leaving `ListingGrid` untouched; verify RTL tests for both variants, the real name, the empty-name fallback, the zero-rating state and the sort URL
- [x] 5.5 Run the token lint and the full gate; verify `npm run check` and `npx next build` pass and `node scripts/check-tokens.mjs` reports 0 violations in the touched files

## 6. Code — team-frontend: tracking regression

- [x] 6.1 Add `src/app/listing/[id]/tracking.test.tsx` asserting one `view_item` per load (not on variant change or anchor navigation), `similar_items` impression/click attribution fields, and the `add_to_cart` payload; verify the test passes and that `TrackView`, `TrackLink`, `TrackImpression` and `AnalyticsProvider` files have an empty `git diff`

## 7. E2E — platform-e2e

- [x] 7.1 Add `team-frontend/FEATURES.yaml` entries (`status: planned`) `listing.pdp-anatomy`, `listing.pdp-variant-url`, `listing.pdp-purchase-feedback`, `listing.pdp-mobile-buy-bar`, `listing.pdp-reviews-url`, `listing.pdp-anchor-nav`, `listing.pdp-no-fabricated-data`, `listing.pdp-not-found`, `listing.pdp-ai-recs-hidden`, one `acceptance` line per spec scenario; verify `make -C platform-e2e features-check`
- [x] 7.2 Add `tests/e2e/features/frontend/product_detail.feature` with steps in a new `step_definitions/product_detail_steps.py` and `ListingDetailPage` page-object methods (variant radio, quantity, buy bar, anchor nav, not-found result) covering: variant URL + price, shared variant URL, add-to-cart pending/toast, buy now -> `/checkout`, 375px sticky bar, `Result` 404 for an unknown id; verify the scenarios pass on the local stack and flip them to `automated` (Ran green against the agora stack (10 scenarios). Variant order now follows the seller's list (team-domain fix); `listing.pdp-variant-url`, `purchase-feedback`, `mobile-buy-bar`, `anchor-nav`, `no-fabricated-data`, `not-found` flipped to `automated`; `pdp-anatomy` has no scenario and stays `planned`.)
- [x] 7.3 Extend `buyer/review_ratings_filter.feature` (assert `?rating=` in the URL and a shareable reload); existing steps for reviews and Q&A (`reviews_breakdown_visible`, `qa.feature`, `rich_reviews.feature`) are NOT modified because the sections are always visible; verify `buyer/review_ratings_filter`, `engagement/qa` and `engagement/rich_reviews` stay green (review_ratings_filter, engagement/qa and rich_reviews run green against the agora stack; `listing.pdp-reviews-url` flipped to `automated`. The shop rating card is asserted against GetShopRatingSummary (the card shows 'Chưa có đánh giá' for a shop with no attributed reviews).)
- [x] 7.4 Extend `recommendations/recommendations.feature` (PDP scenarios): assert the Skeleton gives way to cards with `similar_items` impressions, and that an UNAVAILABLE service leaves the row and its skeleton absent; extend `tracking/emit_tracking.feature` to assert exactly one VIEW event after a variant change and an anchor-nav click; verify both features stay green (NOT TICKED: emit_tracking.feature (7 scenarios, including the variant/anchor-nav view-beacon one) runs green against the agora stack. recommendations/recommendations.feature cannot be collected in the e2e venv (needs pandas and platform-recsys), so the PDP recommendation scenarios were not run; `listing.pdp-ai-recs-hidden` stays `planned`.)
- [x] 7.5 Extend `shop/shop_storefront.feature` to assert the shared header card with the real shop display name, `?sort=price_asc` ordering and the follow toggle; verify the scenario stays green (shop_storefront.feature runs green against the agora stack (the storefront now queries the seller-filtered search instead of the marketplace's first page).)
- [x] 7.6 Extend `buyer/purchase.feature` / `cart_management.feature` steps only where they click `Mua Ngay`/`Thêm Vào Giỏ Hàng` (button names become `Mua ngay`/`Thêm vào giỏ`, match case-insensitively); verify both features stay green and no step still depends on the old labels (buyer/purchase.feature and cart_management.feature run green against the agora stack; no step depends on the old button labels.)
- [x] 7.7 Run `openspec validate ui-phase-product-detail --strict`; verify it is valid

## Evidence (2026-10-09)

- Gate on feat/ui-system after the UI fixes and a frontend rebuild:
  - parallel lane: 714 passed, run twice (w3-par-1, w3-par-2);
  - destructive lane: 97 passed (w3-destr);
  - the stack was READY afterwards.
- spec_sync --strict reports e2e-ready. Unit-verified scenarios carry a VERIFIED BY line and a not-testable FEATURES
  entry (platform-e2e README, spec-check).
- Defects found by e2e and fixed in this wave:
  - voucher layout shift;
  - checkout Stepper shown under the kill-switch;
  - PDP footer and chat bubble under the buy bar;
  - category pills missing on /search;
  - card-title attribution;
  - review paging;
  - route-error retry that did not re-fetch;
  - outages shown as empty (cart, sessions, notifications);
  - home page crash when team-domain is down;
  - orders retry served from the router cache.
- Task 7.4: `recommendations.feature` collects, and its 7 PDP/home scenarios pass. The recs-unavailable path is covered by the uif_product_detail destructive scenarios.
