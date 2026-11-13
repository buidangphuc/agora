## 1. Code track (team-frontend)

- [ ] 1.1 Tests first in `src/lib/gateway/promotion.test.ts` for `listSponsoredSlotSet` mapping and error to empty; confirm red (Slots are mapped from the response, A failing call returns no slots)
- [ ] 1.2 Add `listSponsoredSlotSet` and `recordAdClick` to `src/lib/gateway/promotion.ts`; leave `listSponsoredSlots` as is (Slots are mapped from the response, A failing call returns no slots)
- [ ] 1.3 Tests first: `src/features/ads/data.test.ts` for limit 2, listing load, unpublished drop, 150 ms timeout (At most two slots are rendered, Unpublished sponsored listing is dropped, Slot service failure leaves organic results intact)
- [ ] 1.4 `src/features/ads/{constants,data,dedupe}.ts`: `loadSponsored(placement, categoryId)` with the 150 ms race and `dedupe` (At most two slots are rendered, Unpublished sponsored listing is dropped, Sponsored listing is not duplicated in organic results, Home dedupes the sponsored listing)
- [ ] 1.5 Tests first: `SponsoredClickGuard.test.tsx` (Click beacon fires once per gesture and does not prevent navigation, Click on an organic card sends no beacon); then implement `SponsoredSlots.tsx` with the "Tài trợ" Badge and `SponsoredClickGuard.tsx`
- [ ] 1.6 Tests first: `route.test.ts` for `POST /api/ads/click`; then implement `src/app/api/ads/click/route.ts` (Click route rejects a malformed body, Click route hides the billing outcome, Click route forwards the session and does not log ids, GET on the click route is not allowed)
- [ ] 1.7 Wire `SearchResultsBlock`: page 1 and non-empty results only, parallel load, dedupe, render before grid; extend `SearchBlocks.test.tsx` (Sponsored slot is shown labelled in its category, Page 2 has no sponsored slots, Slot service failure leaves organic results intact, No slots when the organic search has no results, Sponsored listing is not duplicated in organic results)
- [ ] 1.8 Wire `FeedBlock` with placement `home`; extend `blocks.test.tsx` (Sponsored slot is shown on the home feed, Home dedupes the sponsored listing, Home slot failure does not break the feed)
- [ ] 1.9 `npm run check` in team-frontend green; no new dependency; no change to `robots.ts`

## 2. e2e track (platform-e2e)

- [ ] 2.1 `tests/e2e/features/frontend/buyer_sponsored_slots.feature` with scenarios Search shows a sponsored slot in its category, Sponsored listing is not duplicated in organic results, Home feed shows a sponsored slot, Click charges the campaign once and opens the listing, Second click on the same slot charges nothing, Rendering the page charges nothing, Seller clicking their own ad is not charged; campaigns are created through the real `CreateAdCampaign` API, no route fulfils
- [ ] 2.2 Step definitions, reusing seller login, listing seeding and campaign creation steps; read spent back through `ListMyAdCampaigns`
- [ ] 2.3 FEATURES.yaml entry for team-frontend (written by the e2e author)
- [ ] 2.4 `openspec validate buyer-sponsored-slots --strict`; run the feature on a dev env after `promotion-ads-serving` is deployed
