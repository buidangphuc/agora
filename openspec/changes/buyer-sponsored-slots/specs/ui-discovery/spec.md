## ADDED Requirements

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
