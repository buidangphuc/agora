## 1. Code track: team-frontend-seller

- [ ] 1.1 Failing tests then pure `deriveGettingStarted` in `src/features/seller/gettingStarted.ts`: four steps with done / not-done / unknown, 3 s per-source timeout, NOT_FOUND storefront is not done, step 4 combines vouchers and campaigns. Scenarios: Getting started: a new seller sees all four steps not done, done steps are marked from the shop data, a voucher alone completes the marketing step, a seller without a storefront row sees steps 1 and 2 not done, one promotion source down and the other empty is unknown, one promotion source down and the other has a row is done, a source slower than 3 s times out
- [ ] 1.2 Failing tests then `GettingStartedCard.tsx` (server component, links, "chưa rõ" label, hides when all done, no KYC step). Scenarios: Getting started: the card hides once all steps are done, an unavailable source shows chưa rõ and keeps the dashboard, exactly one step missing keeps the card
- [ ] 1.3 Wire the card into `src/app/seller/(dashboard)/page.tsx` inside `Suspense` above the KPI row, seller id from `getPrincipal()` only, add a skeleton to `loading.tsx`; reuse `getStorefront`, `listMyListings` and the `seller-marketing-hub` wrappers `listMyShopVouchers`, `listMyCampaigns` (page size 1). Hot file: rebase on `seller-server-paging-ui` changes. Scenarios: Getting started: the seller id comes from the session, not the request, a visitor without listing.write never reaches the card, an unavailable source shows chưa rõ and keeps the dashboard
- [ ] 1.4 `npm run vendor:check`, tests, lint, typecheck in team-frontend-seller. Scenarios: all of the above

## 2. E2E track

- [ ] 2.1 `platform-e2e/tests/e2e/features/frontend/seller_getting_started.feature` (page objects `app = "seller"`; data seeded through the real gateway API, no route fulfils): Getting started: a new seller completes the steps in the browser, a seller with a flash sale only sees the marketing step done, a buyer without the seller role does not see the card
- [ ] 2.2 `openspec validate seller-getting-started --strict` and `scripts/asdlc/spec_lint.py seller-getting-started`
