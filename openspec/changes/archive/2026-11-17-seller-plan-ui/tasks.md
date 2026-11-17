## 1. Code track (team-frontend-seller)

- [x] 1.1 Tests first for the gateway mappers: `getMySubscription` (none = FREE, period end, auto_renew) and `cancelSubscription`, add `termDays` to `ViewPlan`; verify red (scenarios: Treats a missing subscription as FREE, Active PRO seller sees period end and renewal state)
- [x] 1.2 Tests first for the plans page: prices per term, period end, FREE, fail-closed, scope guard; verify red (scenarios: Paid plan prices show as whole VND per term, Active PRO seller sees period end and renewal state, FREE seller has no period, Subscription read failure disables the buttons, Seller without the listing scope sees nothing)
- [x] 1.3 Tests first for `SubscribeButton` dialogs and the pending guard; verify red (scenarios: Upgrade confirm dialog states price and no proration, Confirmed upgrade calls Subscribe once and refreshes, Downgrade dialog names the period end, Insufficient balance is explained and nothing changes)
- [x] 1.4 Tests first for `CancelSubscriptionButton` (scenarios: Cancel button only for renewing paid plans, Confirmed cancel calls CancelSubscription once, Cancel failure leaves the state unchanged)
- [x] 1.5 Tests first for `saveListingAction` cap mapping and `ListingForm` link (scenarios: Cap refusal shows the upgrade link, Other resource exhaustion shows no upgrade link, Plan unavailable is not shown as a cap)
- [x] 1.6 Implement `src/lib/gateway/promotion.ts` additions, `cancelSubscriptionAction` and FAILED_PRECONDITION mapping in `actions.ts`, plans page, `SubscribeButton` (modal, direction), `CancelSubscriptionButton`, `ListingForm` upgrade link (all scenarios of requirements 1-4)
- [x] 1.7 Update the `ui-seller` plan button wording test (scenario: Current plan cannot be re-subscribed, MODIFIED requirement); bump the proto copy to the monetization-contract version; `make check` green

## 2. e2e track (platform-e2e)

- [x] 2.1 `seller/seller_plans.feature` scenario "Paying subscription end to end" with steps (scenario: Paying subscription end to end)
- [x] 2.2 Scenario "Insufficient balance is explained and nothing changes" (scenario: Insufficient balance is explained and nothing changes)
- [x] 2.3 Scenario "Cancelling keeps the plan until period end" (scenario: Cancelling keeps the plan until period end)
- [x] 2.4 `seller/create-listing.feature` scenarios "Eleventh listing on FREE shows the upgrade CTA" and "Boundary, the tenth listing is accepted" (scenarios: Eleventh listing on FREE shows the upgrade CTA, Boundary, the tenth listing is accepted)
- [x] 2.5 Update `frontend/ui_seller_orders_money.feature::Current plan cannot be re-subscribed` for `Nâng cấp` wording; register all in team-frontend-seller FEATURES.yaml (done by the e2e-author)

## 3. Verification

- [x] 3.1 `openspec validate seller-plan-ui --strict`, spec_lint, team-frontend-seller checks, features check
