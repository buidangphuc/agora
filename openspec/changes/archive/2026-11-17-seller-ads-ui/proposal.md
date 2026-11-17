## Why

Initiative `monetization`, story 5: the seller campaign table on `/seller/ads` is client state
(`team-frontend-seller/src/features/seller/AdCampaignForm.tsx:73`, `useState<ViewAdCampaign[]>([])`), so every
campaign disappears on reload, spend and clicks are never shown, and a campaign cannot be paused or resumed.
`promotion-ads-campaign-mgmt` adds `ListMyAdCampaigns`, `PauseAdCampaign` and `ResumeAdCampaign` (routed by
team-gateway); this slice makes the page use them.

initiative: monetization

## What Changes

- `src/lib/gateway/promotion.ts`: `ViewAdCampaign` gains `spent`, `impressions`, `clicks`, `categoryId`; the status
  text covers `PAUSED_BUDGET` ("Hết ngân sách"). New wrappers `listMyAdCampaigns(page)`, `pauseAdCampaign(id)`,
  `resumeAdCampaign(id)` over `SponsoredService`. Existing `createAdCampaign` and `listSponsoredSlots` are unchanged
  (this file is also edited by `seller-plan-ui`; this slice only touches the ads section).
- `src/features/seller/actions.ts`: `pauseAdCampaignAction`, `resumeAdCampaignAction` return the shared
  `{ ok, error?, data? }` and `revalidatePath("/seller/ads")`; a FAILED_PRECONDITION on resume maps to
  "Ngân sách còn lại thấp hơn giá thầu, không thể tiếp tục chạy." `createAdCampaignAction` and the client form reject a
  bid under 500 VND with "Giá thầu tối thiểu là ₫500." (the server stays authoritative: INVALID_ARGUMENT is surfaced).
- `/seller/ads` is a server page that loads the seller's campaigns (`?page=`, 20 per page) and renders a table from
  the service: listing title, bid, budget, spent, remaining, impressions, clicks, status, and a Pause or Resume
  button per row. The "Chiến dịch vừa tạo" client list is removed; after create the page revalidates and the new
  campaign appears from the server.
- Out of scope: budget top-up or edit (no RPC), CTR or ROAS reports, wallet debit for ads (budget is a counter),
  the buyer slots (`buyer-sponsored-slots`).

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `ui-seller`: adds the persistent, manageable campaign list on `/seller/ads` and the 500 VND minimum bid.

## Impact

- `team-frontend-seller/src/lib/gateway/promotion.ts` (+ test), `src/features/seller/actions.ts` (+ test),
  `AdCampaignForm.tsx`, new `AdCampaignTable.tsx`, `src/app/seller/ads/page.tsx` and `loading.tsx`,
  `FEATURES.yaml` (written by the e2e author).
- `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` and its steps.
- Depends on `promotion-ads-campaign-mgmt` (RPCs and gateway routes); no proto change here.

## NFR

- The list adds one server-side `ListMyAdCampaigns` call to the page render, run in parallel with the listings
  read; p95 added latency <= 50 ms locally. A failed or slow (> 2 s) call never blocks the page: the form still
  renders and the list shows an error with "Thử lại".
- Pause and resume are single Server Actions; the button is disabled while pending, so one click issues one RPC.
  They are idempotent server-side, so a double submit is harmless.
- Money is integer VND (int64 converted with `Number`, safe below 2^53), formatted with `formatPrice`; remaining =
  budget - spent is computed for display only and never below 0. No client-side status decisions beyond which
  buttons are enabled.
- Zero layout shift: `loading.tsx` keeps the table skeleton at its final height.

## Data classification

- Seller id, own listing ids and campaign figures (budget, bid, spent, impressions, clicks, status). Purpose:
  let the seller manage their own ads. Basis: contract performance (marketplace terms), same as the existing ads
  and wallet pages. Retention: none added; fetched per request (`force-dynamic`), never stored client-side.
- No buyer data exists in the response (impression counters are aggregates). Never log raw seller, listing or
  campaign ids; the frontend logs only the outcome class.

## Threat model

- Assets: a seller's campaign settings and spend figures; ability to start or stop spend.
- Abuse cases: a seller pausing or resuming another seller's campaign by guessing an id; a guest or a buyer
  reaching the page; tampering with the bid below the minimum; double-click causing repeated actions.
- Authz rule: the page requires a principal with `listing.write` (existing guard); the list returns only the
  caller's campaigns (the request carries no seller id); Pause and Resume return NOT_FOUND for another seller's
  campaign and the UI shows a generic "Không tìm thấy chiến dịch." without confirming existence. The minimum bid
  is enforced by the service; the client check is convenience only.
