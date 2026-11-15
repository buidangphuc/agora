## 1. Code track (team-frontend-seller)

- [x] 1.1 Tests first: `src/lib/gateway/promotion.test.ts` for `listMyAdCampaigns` / `pauseAdCampaign` / `resumeAdCampaign` mapping (spent, impressions, clicks, PAUSED_BUDGET text, int64 precision); confirm red (Campaigns persist after reload, Spend and counters come from the service)
- [x] 1.2 Extend `ViewAdCampaign` and `adCampaignStatusText` in `src/lib/gateway/promotion.ts`; add the three wrappers with a 2 s timeout (Campaigns persist after reload, List failure degrades only the list)
- [x] 1.3 Tests first in `actions.test.ts`: pause/resume success, NOT_FOUND to "Không tìm thấy chiến dịch.", FAILED_PRECONDITION to the budget message, create with bid < 500 (Pause an active campaign, Resume a paused campaign, Budget-exhausted campaign cannot resume, Another seller's campaign is not found)
- [x] 1.4 Add `pauseAdCampaignAction`, `resumeAdCampaignAction` and the 500 VND rule in `actions.ts` with `revalidatePath("/seller/ads")` (Pause an active campaign, Resume a paused campaign, Bid below minimum is rejected)
- [x] 1.5 Tests first: `AdCampaignTable.test.tsx` for empty, ended, budget-exhausted, pending, double click (Seller with no campaign sees an empty state, Ended campaign has no action, Budget-exhausted campaign cannot resume, Double click issues one call)
- [x] 1.6 New `src/features/seller/AdCampaignTable.tsx` (Table, Tag, Button with pending and toast, `formatPrice`, remaining = max(0, budget - spent)); remove the client-only "Chiến dịch vừa tạo" list from `AdCampaignForm.tsx` (Campaigns persist after reload, Pause an active campaign, Resume a paused campaign)
- [x] 1.7 Update `validateAdCampaign` (bid >= 500, budget >= bid) and `SellerForms.test.tsx` (Bid below minimum is rejected, Bid of exactly 500 is accepted, Ad form: budget below the bid is rejected)
- [x] 1.8 `src/app/seller/ads/page.tsx`: fetch campaigns with `?page=` and listings in parallel, join listing titles, keep the existing guest redirect and `listing.write` guard, error state with "Thử lại"; `page.test.tsx`; update `loading.tsx` (Campaigns are paged at 20, List failure degrades only the list, Guest and non-seller are not authorized to open the page)
- [x] 1.9 `npm run check` in team-frontend-seller green; no new dependency

## 2. e2e track (platform-e2e)

- [x] 2.1 `tests/e2e/features/frontend/seller_ads_ui.feature` with scenarios Campaigns persist after reload, Spend and counters come from the service, Pause an active campaign, Resume a paused campaign, Another seller's campaign is not found, Bid of exactly 500 is accepted, Guest and non-seller are not authorized to open the page; data from real create, click and pause flows, no route fulfils
- [x] 2.2 Step definitions for the feature, reusing seller login, listing creation and the buyer click step from `buyer-sponsored-slots` if present
- [x] 2.3 FEATURES.yaml entry for team-frontend-seller (written by the e2e author)
- [x] 2.4 `openspec validate seller-ads-ui --strict`; run the feature on a dev env after `promotion-ads-campaign-mgmt` is deployed
