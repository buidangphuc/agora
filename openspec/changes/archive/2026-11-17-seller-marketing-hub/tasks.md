## Code track (team-frontend-seller, then team-frontend)

- [x] 1. Gateway module `src/lib/gateway/promotion.ts`: add `listMyShopVouchers(page)`, `deactivateMyShopVoucher(id)`,
  `listMyCampaigns(statuses, page)`, `createCampaign(input)` (with idempotency key), `cancelMyCampaign(id)`; map proto to
  view types, derive voucher state label, map campaign status/sold/cap; map ConnectError codes to typed results. No seller id
  in any request. Scenarios: "Vouchers of every state are listed with used and quota", "Campaigns are listed with status and
  sold over cap", "Other sellers' vouchers are never shown", "Other sellers' campaigns are never shown".
- [x] 2. Server actions in `src/features/seller/actions.ts`: `createShopVoucherAction`, `deactivateShopVoucherAction`,
  `createCampaignAction`, `cancelCampaignAction`; each checks `listing.write`, calls the gateway module, revalidates
  `/seller/marketing`. Scenarios: "Seller without listing.write cannot submit", "Another seller's voucher id is denied",
  "Foreign listing id is denied", "Another seller's campaign id is denied", "Deactivating twice is accepted",
  "Cancelling a campaign that started meanwhile is refused", "Duplicate code shows the server error".
- [x] 3. Route `src/app/seller/marketing/page.tsx` (+ `loading.tsx`) with `?tab=` and `cursor`/`status` URL state through
  `listParams.ts` and `SellerPagination`; per-tab error/retry state. Add "Marketing" to `SELLER_NAV` (`nav.tsx`).
  Scenarios: "Seller opens the hub from the navigation", "Tab is kept in the URL", "Unknown tab value falls back to the default",
  "Buyer account is denied the hub", "Anonymous visitor is redirected to login", "One tab's dependency outage does not blank the
  other", "Unknown cursor shows the retry state", "Paging boundary at 21 vouchers", "Empty list shows an empty state".
- [x] 4. Components `features/seller/marketing/`: `VoucherTab`, `ShopVoucherForm`, `FlashSaleTab`, `FlashSaleForm`,
  `ListingPicker` (debounced, `ListMyListings` with `statuses=[PUBLISHED]` + `query`, one page, via `listingsPage.ts`),
  confirm dialogs; client validation mirrors server rules. Scenarios: "Valid voucher is created and listed", "Empty code is
  rejected before any call", "Window ending before it starts is rejected", "Picker searches server-side", "Valid campaign is
  created and listed", "Double submit creates one campaign", "Missing listing is rejected before any call", "Sale price of zero
  or cap below one is rejected", "Server rejection is shown and nothing is created", "Deactivating an active voucher",
  "Cancelling the confirmation changes nothing", "Cancelling a scheduled campaign", "Active and ended campaigns offer no
  cancel control", "Hub status filter narrows the list", "Listing title lookup failure does not hide the row".
- [x] 5. Unit/component tests (vitest, red first) for tasks 1-4 incl. voucher state derivation at the boundaries
  (now == starts_at, now == ends_at) and the 375 px layout.
- [x] 6. team-frontend: remove `VoucherManager`, `createVoucherAction` (+ tests) and the `canManage` branch from
  `src/app/(shop)/vouchers/page.tsx`; update `page.test.tsx`. Scenarios: "Seller visiting the buyer voucher page sees no create
  form", "Buyer voucher wallet still renders".
- [ ] 7. Run through `gate_digest.py`: lint, typecheck, tests, coverage for both repos; read the JSON verdict only.

## E2E track (platform-e2e)

Seed through the real services (gateway API / `seed-marketplace.sh`); page objects set `app = "seller"`. Entries go to
`team-frontend-seller/FEATURES.yaml` (hub) and `team-frontend/FEATURES.yaml` (`/vouchers`). Needs `promotion-seller-self-service`
deployed in the stack.

- [x] 8. Feature `seller/seller_marketing_hub.feature`, actor seller A (+ seller B for isolation): voucher tab list, create,
  deactivate, foreign id denied. Scenarios: "Vouchers of every state are listed with used and quota", "Other sellers' vouchers
  are never shown", "Valid voucher is created and listed", "Duplicate code shows the server error", "Deactivating an active
  voucher", "Another seller's voucher id is denied".
- [x] 9. Flash sale flow: create via picker, list with status filter, cancel SCHEDULED, ACTIVE row has no cancel, foreign id
  denied. Scenarios: "Valid campaign is created and listed", "Picker searches server-side", "Hub status filter narrows the list",
  "Cancelling a scheduled campaign", "Active and ended campaigns offer no cancel control", "Cancelling a campaign that started
  meanwhile is refused", "Other sellers' campaigns are never shown", "Another seller's campaign id is denied".
- [ ] 10. Access and navigation: buyer denied, anonymous redirected, nav entry, tab URL. Scenarios: "Seller opens the hub from
  the navigation", "Tab is kept in the URL", "Buyer account is denied the hub", "Anonymous visitor is redirected to login".
- [x] 11. Buyer app `/vouchers` for a seller shows no create form and the wallet renders. Scenarios: "Seller visiting the buyer
  voucher page sees no create form", "Buyer voucher wallet still renders".
- [x] 12. Paging boundary (21 vouchers seeded) and empty states. Scenarios: "Paging boundary at 21 vouchers", "Empty list shows
  an empty state".
