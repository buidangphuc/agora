## Why

Initiative `monetization`: sponsored listings are the third revenue lever. `promotion-ads-serving` makes
`SponsoredService.ListSponsoredSlots` return category-matched, bid-ranked slots with single-use impression tokens and
makes `RecordAdClick` charge a click. Nothing on the buyer side calls them: `listSponsoredSlots` in
`team-frontend/src/lib/gateway/promotion.ts` is a wrapper that no page uses, and it reads only the old `listing_ids`
field. Without this slice no buyer ever sees a "Tài trợ" slot and no click is ever billed.

initiative: monetization

## What Changes

- `src/lib/gateway/promotion.ts` gains `listSponsoredSlotSet({ placement, categoryId, limit })` returning
  `{ listingId, campaignId, impressionToken }[]` (fail-soft: `[]` on any error) and `recordAdClick(token)`.
  The old `listSponsoredSlots(contextStr)` wrapper is left untouched.
- Search page 1 (`/search`, `SearchResultsBlock`) and the home feed (`FeedBlock`) each request 2 slots
  (placements `search` and `home`), load the sponsored listings, and render them first in the grid with a "Tài trợ"
  label. A sponsored listing that also occurs in the organic items of the same page is removed from the organic list.
- Clicking a sponsored card navigates to `/listing/[id]` and, from a client island, posts the impression token to a new
  route handler `POST /api/ads/click`, which calls `RecordAdClick` server-side. The charge never blocks navigation and
  the page never learns whether it was charged.
- The slot lookup is capped at 150 ms; on error or timeout the page renders organic results only.
- Out of scope: any ranking, charging or token logic (owned by `promotion-ads-serving`), the seller ads page
  (`seller-ads-ui`), sponsored items on page 2+, product detail, cart or the AI assistant, organic ranking.

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `ui-discovery`: adds sponsored slots on search page 1 and the home feed, and the click-charge route.

## Impact

- `team-frontend/src/lib/gateway/promotion.ts` (+ test), new `src/features/ads/` (`data.ts`, `SponsoredSlots.tsx`,
  `SponsoredClickGuard.tsx`, `dedupe.ts`, tests), `src/features/search/SearchBlocks.tsx`,
  `src/features/home/FeedBlock.tsx`, new `src/app/api/ads/click/route.ts` (+ test); `FEATURES.yaml` (written by the
  e2e author).
- `platform-e2e/tests/e2e/features/frontend/buyer_sponsored_slots.feature` and its steps.
- Depends on `promotion-ads-serving` (RPC behaviour, gateway routes for both RPCs, `RecordAdClick` anonymous); no
  proto change here and `/api/` stays disallowed in `robots.ts`.

## NFR

- Slot lookup plus listing loads add at most 150 ms to the render of search page 1 and home: the combined call is
  raced against a 150 ms timeout and the slots are dropped on expiry. Slots load in parallel with existing reads, so
  there is no extra Suspense boundary and no layout shift when slots arrive or are dropped.
- Availability: a failing, slow or empty sponsored lookup never changes the organic results, the count, the filters
  or the HTTP status (200).
- `POST /api/ads/click` answers within 1 s, always 204 for a well-formed body (charged or not) and 400 for a malformed
  body; the browser fires it with `navigator.sendBeacon`, so navigation is never delayed.
- Money and idempotency are server-side: the frontend sends a token once per click gesture and never computes a
  charge. A double click may send two beacons; `RecordAdClick` charges at most once per token.

## Data classification

- Impression token: opaque, single-use, expires after 30 minutes, carries no buyer id (set by `promotion-ads-serving`).
  Purpose: bill one click once. Basis: legitimate interest in running paid placements, no personal data. Retention:
  none in the frontend; it exists in the rendered HTML and one beacon body, never stored, cached (`force-dynamic`)
  or put in a URL or cookie.
- Buyer session: the route forwards the existing session token, if any, so the server can detect the seller's own click;
  the frontend stores nothing new and no buyer id, IP or session is written by this slice.
- Logs: the route logs only the outcome class (`ok`, `rejected`, `error`), never the token, listing, campaign or
  user id (sloglint rule / `no raw ids in logs`).

## Threat model

- Assets: the sellers' campaign budgets; buyer trust in labelled ads.
- Abuse cases: click fraud by replaying a token or scripting `POST /api/ads/click` (bounded by single-use tokens,
  30 min expiry and gateway rate limit; no extra charge is possible from the frontend); a crawler or link prefetch
  charging a campaign (the charge is a POST fired by a click, never a GET or a prefetch); a seller clicking their own ad
  (the forwarded session lets the server charge nothing); an open redirect through the click route (the route never
  redirects: it returns 204 and navigation is the plain `/listing/[id]` link); unlabelled ads (every sponsored card
  carries the "Tài trợ" label); a forged token with oversized input (length and charset checked, 400).
- Authz rule: browsing slots and clicking are open to guests and buyers; there is no resource ownership on the buyer
  side. The self-click rule and the charge are enforced by team-promotion, which stays authoritative.
