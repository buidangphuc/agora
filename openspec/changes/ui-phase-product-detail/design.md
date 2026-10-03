## Context

Reference template: Ant Design Pro **Profile › Basic Profile**: a page header (breadcrumb + title + actions),
a header Card with a media block and a `Descriptions` summary, then Cards/Tabs for the long-tail content.
agora implements it natively on the core components from `ui-core-components`. Inputs read from disk:
`UI_SYSTEM_DESIGN.md` §1/§4/§5/§6, the current `src/app/listing/[id]/page.tsx` (RSC, `force-dynamic`),
`src/app/shop/[id]/page.tsx`, and the feature components listed in the proposal.

Current shape (verified in code):

| Part | Today | Problem |
|---|---|---|
| `page.tsx` | RSC; `Promise.all` of 7 fetches, then a serial `await summarizeReviews` | AI summary blocks TTFB; no `loading.tsx` |
| Gallery | inline `<img>` + inert thumbnails | no click, no fallback, no aspect-safe `Image` |
| Variants | inert `<button>`s in page; unused client `VariantSelector` | no state, no URL, price/stock never change |
| `AddToCartButton` | client; `Thêm`/`Mua Ngay`, local `quantity` | always `variants[0]`; raw `<button>`s; text-only pending |
| `ReviewSection` | client; filter in `useState`; stars as `★` text | not shareable; no `Rate`; no pagination |
| `QASection` | client; ask/answer via `useTransition` | inline error text, no toast |
| `AiReviewSummary` | server-pure | hidden when null (kept), but not streamed |
| `RecommendationsRow` | async RSC, hides on empty | not in `Suspense` |
| Shop block | inline card with fake name/response rate; storefront header duplicated in `ShopStorefrontView` | two diverging copies |

## Goals / Non-Goals

Goals: one coherent PDP on core components; every commerce action has pending/disabled/feedback; CLS = 0;
URL-addressable variant/rating/page; sticky mobile buy bar; graceful hide of AI and recommendations;
tracking untouched.
Non-goals: new data in the contract, lightbox/zoom, seller management, anything in other phases.

## Decisions

1. **Ant Design / Ant Pro mapping**

   | Ant Design / Pro | agora component | Route / location |
   |---|---|---|
   | Pro Basic Profile: page header | `Breadcrumb` (Trang chủ › category › title) + `<h1>` | `/listing/[id]` top |
   | Pro Basic Profile: header card | `Card` with 2-col grid: `ImageGallery` / info column | `/listing/[id]` |
   | `Image` (no preview) | `Image` (fixed 1:1, fallback, lazy below fold) | gallery stage, thumbs, review photos |
   | `Carousel` thumbnails (custom) | `ImageGallery` thumbnail strip (buttons, `aria-current`) | gallery |
   | `Radio.Group` button style | `VariantSelector` over `Radio` (button variant), writes `?variant=` | info column |
   | `InputNumber` | `QuantityPicker` (min 1, max = variant stock) | purchase panel |
   | `Statistic` (price) / `Tag` | `PriceTag` + `Tag` (discount only when data exists) | info column |
   | `Descriptions` | `Descriptions` specs block (category, stock, ship-from, SKU) | `#specs` section |
   | Anchor nav (`Anchor`) | anchor nav of links `#specs`, `#reviews`, `#qa`, sticky on desktop (no `Tabs`) | body |
   | `Rate` | `Rate` read-only (summary, review items) + input (ReviewModal) | reviews |
   | `Progress` | `Progress` for the 5-star breakdown bars | reviews summary |
   | `Pagination` | `Pagination` (link-based, `?rpage=`) | reviews section |
   | `Avatar` | `Avatar` (reviewer, shop, answerer) | reviews, Q&A, shop card |
   | `Card` + `Descriptions`/`Statistic` row | `ShopHeaderCard` | PDP shop block + `/shop/[id]` header |
   | `Alert` | `Alert` (action errors, out-of-stock warning) | in context |
   | `Result` 404 | `Result` status 404 | `not-found.tsx` |
   | `Skeleton` | `Skeleton` footprints | `loading.tsx`, Suspense fallbacks |
   | `Empty` | `Empty` (no reviews, no reviews for filter, no questions) | sections |
   | `message` | `ToastProvider` | cart / review / Q&A actions |
   | `Affix` (bottom) | `BuyBar` (fixed bottom, mobile only) | below `lg` |

2. **Page anatomy** (desktop >=1024px; one `max-w` container, 4px spacing scale):
   - Header: `Breadcrumb`, then the header `Card` (`grid-cols-12`: gallery `col-span-5`, info `col-span-7`).
   - Gallery: stage `Image` (aspect 1/1) with the `FavoriteButton` overlay bottom-right (unchanged component);
     thumbnails below (`Image` 64x64 fixed). A `Tag` (Mall) appears top-left only when a real flag exists
     (Decision 7).
   - Info: `<h1>` (20px), rating row (`Rate` read-only + count, from `getListingRatingSummary`), `PriceTag`
     (24px, brand colour), `LiveFlashSaleStock` (unchanged), perks list (shipping/insurance static policy text,
     14px), `VariantSelector`, `QuantityPicker`, stock line, actions (`Thêm vào giỏ` outline-brand, `Mua ngay`
     primary), then `AddToCollectionButton`, `AlertToggle`, `ShareButton` (behaviour unchanged).
   - `ShopHeaderCard`: `Avatar` + shop name (from `getStorefront(sellerId)` when available, else
     "Shop #<id6>") + `Statistic` row (rating from `getShopRatingSummary`) + actions `ChatWithSellerButton` /
     `Xem Shop`.
   - Body: an anchor nav (links to `#specs`, `#reviews`, `#qa`; sticky on desktop; highlighting the current
     section is optional) above three stacked, always-visible sections: `#specs` Chi tiết (specs `Descriptions` +
     description), `#reviews` Đánh giá (AI summary, summary box, filter, list, pagination, write-review),
     `#qa` Hỏi đáp. There are no content `Tabs`.
   - Footer row: similar items (`RecommendationsRow` in `Suspense`).
   - Mobile (375px): single column; gallery full-width 1:1 with dot indicator; info stacked; `ShopHeaderCard`
     stacks; the anchor nav scrolls horizontally; `BuyBar` fixed to the bottom (safe-area padding): price,
     `Thêm vào giỏ` (outline), `Mua ngay` (primary). The inline action row is hidden below `lg`, so there is
     exactly one visible set of purchase actions per breakpoint.

3. **Server/client split (RSC by default, leaf islands).** Server: `page.tsx`, `Breadcrumb`, `Descriptions`,
   `PriceTag`, `ShopHeaderCard`, review summary box, read-only review list, `Pagination` (links),
   `AiReviewSummary`, `RecommendationsRow`. Client islands only: `ImageGallery` (selected index, swipe),
   `VariantSelector` (`router.replace`), `PurchasePanel` (`QuantityPicker` + `Thêm`/`Mua ngay`, one shared pending
   state exposed to `BuyBar` through a small context), `BuyBar`, `ReviewHelpfulButton`, review form/modal,
   `QAAskForm`, `QAAnswerForm`. The anchor nav is plain links (server), with an optional client
   scroll-spy leaf for highlighting.

4. **URL state (searchParams).** `?variant=<variantId>`, `?rating=1..5` (default all),
   `?rpage=<n>` (default 1). There is no tab parameter. `quantity` stays local (transient, not shareable). Island
   writes use `router.replace(url, { scroll: false })` for variant and rating (no history spam, no scroll
   jump); review filter/page links keep the `#reviews` hash so the reload lands on the reviews section. `page.tsx` reads `searchParams` and calls the pure
   function `resolveVariant(listing, variantId)`: unknown id falls back to the first in-stock variant, else the
   first variant, else the base listing. Price, stock, SKU and the gallery's active image are all derived from
   that one result on the server. The variant is written to the URL only when it changes price or stock
   relative to the base (otherwise chips render but selection stays local), per the product requirement.

5. **Streaming / CLS.** `loading.tsx` renders `Skeleton` blocks with the exact footprints (breadcrumb, gallery
   aspect-square, title/price/actions heights, shop card, anchor nav). `page.tsx` awaits only the critical set
   (`getListing`, rating summary, flash sale, shop summary, principal). Non-critical blocks are async RSCs in
   `Suspense`: `AiReviewSummary` (Skeleton of the card height; renders `null` when team-ai returns null),
   `RecommendationsRow` (Skeleton of one grid row), and the reviews and Q&A list fetchers, each in its own `Suspense` boundary with a `Skeleton` of the section
   footprint (all sections are always rendered). Every `Image` declares its aspect (`aspect-square` for gallery, thumbs, review
   photos); the gallery stage is eager (above the fold), thumbs and below-the-fold images are `loading="lazy"`.

6. **Mutation contract.** PDP actions return `{ ok: boolean; error?: string; data?: T }` and call
   `revalidatePath("/listing/<id>")` (cart also revalidates `/cart`). Today they return `{ ok, message? }`. The
   change adds `error` and keeps `message` as a deprecated alias for this phase (additive), so callers outside the
   PDP (the cart phase) keep working. Touched actions: `addToCartAction` (`features/cart/actions.ts`, shared;
   additive only), `markReviewHelpfulAction`, `createReviewAction` (`features/review/actions.ts`),
   `askQuestionAction`, `answerQuestionAction` (`features/listing/qa/actions.ts`). Each trigger has pending
   (`isLoading`), disabled (out of stock, quantity bounds, pending, empty input) and a toast on success and error;
   errors that need context (rejected review, rejected question) also render an inline `Alert`.

7. **No fabricated data.** Removed or conditioned on real data:
   - rating and count come from `ratingSummary`; with `reviewCount = 0` show "Chưa có đánh giá" (no stars);
   - "Đã bán" (sold count) is hidden (no field in the contract); with no reviews there are no stars and no
     sold count anywhere on the page;
   - strike-through price and "% GIẢM" only when the flash-sale campaign carries a sale price, never
     `price * 1.25`;
   - Mall badge hidden (no field); the title-substring and `price > 5,000,000` heuristics are deleted from the
     PDP (ListingCard's copy belongs to discovery and is out of scope);
   - shop name: the real display name from the `shop-display-name` contract; "Shop #<6 chars of id>" only when
     the name is empty; response rate and "Online n phút trước" hidden;
   - voucher chips hidden on the PDP (`/vouchers` and checkout own vouchers);
   - the "Thương hiệu: Chính Hãng 100%" spec row removed; the guarantee strip stays as static policy text;
   - the Unsplash placeholder URL replaced by the `Image` fallback.
   Principle 1 is enforced by the `ui-foundation` token lint: no `text-[10px]`, no `bg-[#d0011b]`; type scale
   12/14/16/20/24 only; brand colour only on `Mua ngay`, `PriceTag`, discount badge, and the active
   `Tabs`/`Radio` accent.

8. **Shop header sharing.** `ShopHeaderCard({ sellerId, storefront, summary, productCount?, actions, variant })`
   with `variant="compact"` (PDP) and `variant="hero"` (storefront banner + tagline). It is a server component;
   follow/chat remain client leaf buttons passed through `actions`. On `/shop/[id]` only the header card and the
   price-sort tabs are in scope: the tab state moves from `useState` to `?sort=` and `ListingGrid` is reused
   unchanged. The fabricated "4.9 / 5.0", "99%", "1 năm" header stats are replaced by `getShopRatingSummary` and
   omitted fields. Bundles, listing fetch and follow logic on that route are untouched.

9. **Not-found, error, empty.** `getListing` returns `null` only for gRPC `NotFound`; any other error throws.
   `not-found.tsx` (route level) renders `Result status="404"` titled "Không tìm thấy sản phẩm" with primary
   action `Về trang chủ` and secondary `Tìm sản phẩm khác` (`/search`). `error.tsx` (client) renders an `Alert`
   (error) with `Thử lại` calling `reset()`. Empty states use `Empty`: "Chưa có đánh giá nào" (+ `Viết đánh giá`
   when logged in), "Không có đánh giá {n} sao" with `Xem tất cả` (clears `?rating`), "Chưa có câu hỏi nào" with
   an ask action. Out of stock: `Alert` (warning) "Phân loại này đã hết hàng" plus a hint to choose another
   variant; both purchase buttons disabled.

10. **Tracking invariants (unchanged, asserted in tests).** `TrackView` stays rendered once, outside the sections,
    with unchanged props. `RecommendationsRow` keeps `placementId="pdp_similar_items"` and renders `ListingGrid`
    as today, so `TrackImpression`/`TrackLink` fire with `placementId`, `position`, `impressionId`,
    `modelVersion`. `PurchasePanel` keeps `trackEcommerce("add_to_cart", ...)` with the same payload (Buy now fires
    the same single add event, no extra one). These `data-testid` values are preserved: `review-item`,
    `review-helpful`, `verified-purchase`, `shop-rating-summary`, `qa-item`, `qa-answer`, `qa-ask-form`,
    `qa-login-prompt`, `qa-empty`, `qa-answer-toggle`.

## Risks / Trade-offs

- Moving the rating filter to the URL makes filter changes server round-trips (RSC payload) instead of
  instant client filtering. Acceptable: lists are small and `router.replace` keeps scroll.
- Stacked sections make the page longer and fetch reviews and Q&A on every load (streamed in `Suspense`, so
  first byte is not blocked). In return the existing e2e step bindings for reviews and Q&A keep working
  unchanged.
- Hiding sold count, Mall and voucher chips lowers visual density versus the Shopee-like WIP; chosen over
  showing invented numbers.
- The additive `error` alias leaves two fields on shared action results until the cart phase removes `message`.

## Migration Plan

No feature flag. One commit group per component in task order. `VariantSelector` and `AddToCartButton` keep
their export names so any other importer still compiles; the inline variant chips, dead thumbnails and fake
blocks are deleted with the page rewrite.

## Open Questions

1. Decided: no content `Tabs`. Specs, reviews and Q&A are stacked visible sections with a sticky (desktop)
   anchor nav linking to `#specs`, `#reviews`, `#qa`; no tab URL parameter; `rpage` and `rating` stay in the
   URL; existing e2e step bindings are unchanged.
2. **Buy now destination.** Today `AddToCartButton` goes to `/cart` and the unused `VariantSelector` to
   `/checkout`. This draft uses `/checkout`; `ui-phase-cart-checkout` must confirm `/checkout` works for a freshly
   added single item, otherwise fall back to `/cart`.
3. **Shared action shape.** `addToCartAction` is also used by the cart phase. Is the additive `error` alias
   enough, or should the cart phase own the full `{ ok, error?, data? }` migration of `cart/actions.ts`?
4. Decided: the shop name comes from the separate backend change `shop-display-name`; "Shop #<6 chars>" is only
   the fallback for an empty name. This change depends on it.
5. **Reviews pagination.** `listReviews` returns all reviews; this draft slices 10 per page on the server.
   Confirm the page size, or defer until the API paginates.
6. Decided: the Mall badge is hidden; the heuristic is deleted. A real "official store" flag is a follow-up
   backend change (listed in Non-goals).
