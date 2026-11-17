## Decisions

- Where slots merge: server components. `SearchResultsBlock` and `FeedBlock` already await their data; each adds a
  parallel `loadSponsored(placement, categoryId)` (module `features/ads/data.ts`, `server-only`). It calls
  `listSponsoredSlotSet(limit 2)`, then `getListing` for each slot, drops a slot whose listing is missing or not
  published, and returns at most 2 `{ listing, impressionToken }`. The whole function is raced against 150 ms.
  No new Suspense boundary, so no layout shift.
- Search conditions: slots only on `state.page === 1`, only when the organic search succeeded and returned items.
  `category_id` is `state.category` (empty string when only a text query is set; team-promotion decides what an empty
  category returns). Page 2+ makes no sponsored call.
- Dedupe: `dedupe.ts` removes from organic `items` every id present in the sponsored slots. The page total and
  pagination are unchanged (organic count stays from search).
- Rendering: slots reuse `ListingCard` inside `SponsoredSlots`, a grid with the same `LISTING_GRID_CLASS`, placed
  before the organic grid. Each is wrapped in a `data-testid="sponsored-slot"` container with a "Tài trợ" `Badge`
  (text visible, not colour only). Tracking placement ids are `search_sponsored` and `home_sponsored`.
- Click charge: the sponsored container is a small client island (`SponsoredClickGuard`) listening to `click` and
  `auxclick` on its anchors; it calls `navigator.sendBeacon("/api/ads/click", JSON {token})` once per gesture and does
  not preventDefault, so the browser navigates through the normal link. A GET route or a Next `Link` prefetch is
  avoided on purpose: prefetch and crawlers must never charge. `fetch(..., { keepalive: true })` is the fallback when
  `sendBeacon` is missing.
- Route handler `POST /api/ads/click`: validates `{ token: string }` (1..256 chars, `[A-Za-z0-9_-.~]`), calls
  `recordAdClick(token)` with the session token if present (for the self-click check), returns 204 for any upstream
  outcome and error, so it leaks nothing about charging. `robots.ts` already disallows `/api/`.
- Placement strings `search` and `home` and limit 2 are fixed constants in `features/ads/constants.ts`.

## Alternatives rejected

- Charge on a GET redirect route (`/api/ads/click?t=` then 307): also charges on prefetch, crawlers and link previews.
- Client-side fetch of slots: delays render, loses SSR, shifts layout.
- Showing slots on empty organic results: a sponsored row under "Không tìm thấy sản phẩm" contradicts the message.
