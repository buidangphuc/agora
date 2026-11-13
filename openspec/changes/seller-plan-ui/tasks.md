## 1. Code track (team-frontend-seller)

- [ ] 1.1 Tests first for the gateway mappers: `getMySubscription` (none = FREE, period end, auto_renew) and `cancelSubscription`, add `termDays` to `ViewPlan`; verify red (scenarios: Treats a missing subscription as FREE, Active PRO seller sees period end and renewal state)
- [ ] 1.2 Tests first for the plans page: prices per term, period end, FREE, fail-closed, scope guard; verify red (scenarios: Paid plan prices show as whole VND per term, Active PRO seller sees period end and renewal state, FREE seller has no period, Subscription read failure disables the buttons, Seller without the listing scope sees nothing)
- [ ] 1.3 Tests first for `SubscribeButton` dialogs and the pending guard; verify red (scenarios: Upgrade confirm dialog states price and no proration, Confirmed upgrade calls Subscribe once and refreshes, Downgrade dialog names the period end, Insufficient balance is explained and nothing changes)
- [ ] 1.4 Tests first for `CancelSubscriptionButton` (scenarios: Cancel button only for renewing paid plans, Confirmed cancel calls CancelSubscription once, Cancel failure leaves the state unchanged)
- [ ] 1.5 Tests first for `saveListingAction` cap mapping and `ListingForm` link (scenarios: Cap refusal shows the upgrade link, Other resource exhaustion shows no upgrade link, Plan unavailable is not shown as a cap)
- [ ] 1.6 Implement `src/lib/gateway/promotion.ts` additions, `cancelSubscriptionAction` and FAILED_PRECONDITION mapping in `actions.ts`, plans page, `SubscribeButton` (modal, direction), `CancelSubscriptionButton`, `ListingForm` upgrade link (all scenarios of requirements 1-4)
- [ ] 1.7 Update the `ui-seller` plan button wording test (scenario: Current plan cannot be re-subscribed, MODIFIED requirement); bump the proto copy to the monetization-contract version; `make check` green

## 2. e2e track (platform-e2e)

- [ ] 2.1 `seller/seller_plans.feature` scenario "Seller upgrades to PRO from the wallet balance" with steps (scenario: Paying subscription end to end)
- [ ] 2.2 Scenario "Subscribing with too little balance changes nothing" (scenario: Insufficient balance is explained and nothing changes)
- [ ] 2.3 Scenario "Seller cancels renewal and keeps PRO until period end" (scenario: Cancelling keeps the plan until period end)
- [ ] 2.4 `seller/create-listing.feature` scenarios "Seller at the plan cap sees the upgrade link" and "Seller below the plan cap creates a listing" (scenarios: Eleventh listing on FREE shows the upgrade CTA, Boundary, the tenth listing is accepted)
- [ ] 2.5 Update `frontend/ui_seller_orders_money.feature::Current plan cannot be re-subscribed` for `Nâng cấp` wording; register all in team-frontend-seller FEATURES.yaml (done by the e2e-author)

## 3. Verification

- [ ] 3.1 `openspec validate seller-plan-ui --strict`, spec_lint, team-frontend-seller checks, features check
