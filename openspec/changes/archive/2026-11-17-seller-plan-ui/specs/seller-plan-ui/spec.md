# seller-plan-ui Specification

## ADDED Requirements

### Requirement: The plans page shows each plan's price per term and the seller's current plan with its period

`/seller/plans` SHALL read `ListPlans`, `GetMySubscription` and `GetEntitlements` through the gateway in parallel. Each plan card SHALL show `formatPrice(plan.price)` followed by `/ {term_days} ngày` (integer VND, never scaled), and its caps from the entitlement `limits` for the current plan only. The current plan SHALL be the tier of the seller's subscription, or FREE when `GetMySubscription` returns none. The current plan card SHALL carry the `Gói hiện tại` Tag and a disabled button; a paid current plan SHALL also show `Hết hạn ngày dd/MM/yyyy` from `current_period_end` and `Tự động gia hạn: Bật` or `Tắt` from `auto_renew`.

#### Scenario: Paid plan prices show as whole VND per term
- **WHEN** a seller opens `/seller/plans` and the catalog has PRO 199000 and PREMIUM 499000 with term_days 30
- **THEN** the PRO card shows `₫199.000/ 30 ngày` and the PREMIUM card shows `₫499.000/ 30 ngày` and no card shows a price above `₫499.000`
- **VERIFIED BY**: team-frontend-seller `src/app/seller/plans/page.test.tsx` `renders prices per term in whole VND`

#### Scenario: Active PRO seller sees period end and renewal state
- **WHEN** a seller whose subscription is PRO with current_period_end 2026-12-13T10:00:00+07:00 and auto_renew true opens `/seller/plans`
- **THEN** the PRO card shows the `Gói hiện tại` Tag with a disabled button, the text `Hết hạn ngày 13/12/2026` and `Tự động gia hạn: Bật`
- **VERIFIED BY**: team-frontend-seller `src/app/seller/plans/page.test.tsx` `shows period end and renewal for the current paid plan`

#### Scenario: FREE seller has no period
- **WHEN** a seller for whom `GetMySubscription` returns no subscription opens `/seller/plans`
- **THEN** the FREE card is the current plan with its caps (max_listings 10) and no `Hết hạn ngày` text is rendered anywhere
- **VERIFIED BY**: team-frontend-seller `src/app/seller/plans/page.test.tsx` `treats a missing subscription as FREE`

#### Scenario: Subscription read failure disables the buttons
- **WHEN** `GetMySubscription` fails with UNAVAILABLE
- **THEN** the page lists the plans, shows an error Alert `Không tải được gói hiện tại. Thử lại sau.`, no card has the `Gói hiện tại` Tag and every subscribe and cancel button has `aria-disabled="true"`
- **VERIFIED BY**: team-frontend-seller `src/app/seller/plans/page.test.tsx` `fails closed when the subscription cannot be read`

#### Scenario: Seller without the listing scope sees nothing
- **WHEN** a principal without `listing.write` requests `/seller/plans`
- **THEN** the page renders nothing and none of the three plan reads is issued; a request with no principal redirects to login
- **VERIFIED BY**: team-frontend-seller `src/app/seller/restrictedGuard.test.tsx` `signed-in principal without listing.write is never redirected`, `no principal calls redirectGuestToLogin with the page's own path` (the `/seller/plans` case)

### Requirement: Subscribing states the charge and handles the server refusal

Choosing a non-current plan SHALL open a confirm dialog before any write. The dialog SHALL state the plan price per term and that it is debited from the withdrawable wallet balance. For a plan above the current tier it SHALL read `Nâng cấp có hiệu lực ngay, tính đủ giá gói mới, không hoàn tiền phần còn lại, chu kỳ 30 ngày mới.`; for a plan below the current tier it SHALL read `Hạ gói có hiệu lực từ ngày dd/MM/yyyy (hết chu kỳ hiện tại).`. Confirming SHALL call the `subscribeAction` Server Action once; its button SHALL be disabled while pending. On success the page SHALL revalidate and show a success toast. On FAILED_PRECONDITION the page SHALL show the message `Số dư có thể rút không đủ để trả gói này.` with a link `Xem ví` to `/seller/wallet`, and the displayed current plan SHALL not change.

#### Scenario: Upgrade confirm dialog states price and no proration
- **WHEN** a FREE seller activates the PRO `Nâng cấp` button
- **THEN** a dialog shows `₫199.000`, the wallet-debit wording and the immediate-upgrade text, and `Subscribe` has not been called until the seller confirms
- **VERIFIED BY**: team-frontend-seller `src/features/seller/SubscribeButton.test.tsx` `upgrade dialog states price before any write`

#### Scenario: Confirmed upgrade calls Subscribe once and refreshes
- **WHEN** the seller confirms and then double-clicks the confirm button while the call is pending
- **THEN** `Subscribe` is called exactly once with the PRO plan id, a success toast `Đã đăng ký gói` appears and `/seller/plans` is revalidated
- **VERIFIED BY**: team-frontend-seller `src/features/seller/SubscribeButton.test.tsx` `confirm calls subscribe once`

#### Scenario: Downgrade dialog names the period end
- **WHEN** a PRO seller with current_period_end 2026-12-13 activates the FREE or lower-tier `Hạ gói` button
- **THEN** the dialog shows `Hạ gói có hiệu lực từ ngày 13/12/2026 (hết chu kỳ hiện tại).` and no upgrade wording
- **VERIFIED BY**: team-frontend-seller `src/features/seller/SubscribeButton.test.tsx` `downgrade dialog names the period end`

#### Scenario: Subscribe denied by the gateway shows an error and no change
- **WHEN** `Subscribe` fails with PERMISSION_DENIED or UNAUTHENTICATED
- **THEN** an error toast `Đăng ký gói thất bại.` appears, the current plan card is unchanged and no success toast is shown
- **VERIFIED BY**: team-frontend-seller `src/features/seller/SubscribeButton.test.tsx` `denied subscribe shows an error and no change`

#### Scenario: Insufficient balance is explained and nothing changes
- **WHEN** `Subscribe` fails with FAILED_PRECONDITION because the balance of 100000 VND is below the PRO price 199000
- **THEN** the seller sees `Số dư có thể rút không đủ để trả gói này.` with a `Xem ví` link to `/seller/wallet`, an error toast appears and after reload the current plan is still FREE
- **VERIFIED BY**: platform-e2e `seller/seller_plans.feature::Insufficient balance is explained and nothing changes`

#### Scenario: Paying subscription end to end
- **WHEN** a seller with a withdrawable balance of at least 199000 VND confirms the PRO upgrade
- **THEN** after reload PRO is the current plan with `Hết hạn ngày` 30 days ahead, and the wallet ledger shows one subscription debit of 199000
- **VERIFIED BY**: platform-e2e `seller/seller_plans.feature::Paying subscription end to end`

### Requirement: The seller can stop auto-renew

While the seller's subscription is a paid tier, ACTIVE, with `auto_renew` true, the current plan card SHALL show a `Hủy gia hạn` button. Activating it SHALL open a confirm dialog `Gói vẫn dùng được đến ngày dd/MM/yyyy, sau đó chuyển về Miễn phí.`; confirming SHALL call `cancelSubscriptionAction` once, which calls `CancelSubscription` with no seller id, revalidates `/seller/plans` and shows a success toast. When `auto_renew` is false, or the seller is FREE, the button SHALL NOT render, and the card SHALL show `Sẽ chuyển về Miễn phí ngày dd/MM/yyyy` for a paid plan with auto_renew false.

#### Scenario: Cancel button only for renewing paid plans
- **WHEN** the page renders for a PRO seller with auto_renew true, then for a PRO seller with auto_renew false, then for a FREE seller
- **THEN** `Hủy gia hạn` is present only in the first render, and the second shows `Sẽ chuyển về Miễn phí ngày 13/12/2026`
- **VERIFIED BY**: team-frontend-seller `src/features/seller/CancelSubscriptionButton.test.tsx` `renders only while auto renew is on`

#### Scenario: Confirmed cancel calls CancelSubscription once
- **WHEN** the seller confirms the cancel dialog
- **THEN** `CancelSubscription` is called once with an empty request, a success toast `Đã hủy gia hạn` appears and the page revalidates
- **VERIFIED BY**: team-frontend-seller `src/features/seller/CancelSubscriptionButton.test.tsx` `confirm cancels once`

#### Scenario: Cancel denied by the gateway shows an error
- **WHEN** `CancelSubscription` fails with PERMISSION_DENIED or NOT_FOUND
- **THEN** an error toast `Hủy gia hạn thất bại.` appears and the card is unchanged
- **VERIFIED BY**: team-frontend-seller `src/features/seller/CancelSubscriptionButton.test.tsx` `denied cancel shows an error`

#### Scenario: Cancel failure leaves the state unchanged
- **WHEN** `CancelSubscription` fails with UNAVAILABLE
- **THEN** an error toast `Hủy gia hạn thất bại.` appears and the card still shows `Tự động gia hạn: Bật`
- **VERIFIED BY**: team-frontend-seller `src/features/seller/CancelSubscriptionButton.test.tsx` `cancel failure keeps renewal on`

#### Scenario: Cancelling keeps the plan until period end
- **WHEN** a PRO seller cancels auto-renew in the running stack and reloads `/seller/plans`
- **THEN** PRO is still the current plan, `Tự động gia hạn: Tắt` and `Sẽ chuyển về Miễn phí ngày` show, and no refund entry appears in the wallet
- **VERIFIED BY**: platform-e2e `seller/seller_plans.feature::Cancelling keeps the plan until period end`

### Requirement: A plan-cap refusal on listing creation offers the upgrade

When `CreateListing` fails with RESOURCE_EXHAUSTED and `ErrorInfo.reason` `PLAN_LISTING_LIMIT`, `saveListingAction` SHALL return a failed state with the message `Bạn đã đạt giới hạn số tin đăng của gói hiện tại.` and `upgradeUrl` `/seller/plans`, and `ListingForm` SHALL show that message in an error Alert with a link `Nâng cấp gói` to `/seller/plans`, keeping every typed value. A RESOURCE_EXHAUSTED with any other reason, or any other error, SHALL NOT show the upgrade link. The `Alert` is the only change; no client-side count of listings is made.

#### Scenario: Cap refusal shows the upgrade link
- **WHEN** `createListing` rejects with RESOURCE_EXHAUSTED and reason `PLAN_LISTING_LIMIT`
- **THEN** the returned state has ok false, the cap message and `upgradeUrl` `/seller/plans`, and the form shows the Alert and a `Nâng cấp gói` link with `href="/seller/plans"` while the title field still holds the typed value
- **VERIFIED BY**: team-frontend-seller `src/features/listing/actions.test.ts` `plan listing limit returns an upgrade url`

#### Scenario: Other resource exhaustion shows no upgrade link
- **WHEN** `createListing` rejects with RESOURCE_EXHAUSTED and reason `RATE_LIMITED`
- **THEN** the returned state has no `upgradeUrl` and the form shows no `Nâng cấp gói` link
- **VERIFIED BY**: team-frontend-seller `src/features/listing/actions.test.ts` `other resource exhausted has no upgrade url`

#### Scenario: Permission denied on create is not shown as a cap
- **WHEN** `createListing` rejects with PERMISSION_DENIED
- **THEN** the returned state has no `upgradeUrl` and the form shows no `Nâng cấp gói` link
- **VERIFIED BY**: team-frontend-seller `src/features/listing/actions.test.ts` `permission denied has no upgrade url`

#### Scenario: Plan unavailable is not shown as a cap
- **WHEN** `createListing` rejects with UNAVAILABLE
- **THEN** the form shows the generic failure message and no `Nâng cấp gói` link
- **VERIFIED BY**: team-frontend-seller `src/features/listing/actions.test.ts` `unavailable has no upgrade url`

#### Scenario: Eleventh listing on FREE shows the upgrade CTA
- **WHEN** a FREE seller who owns 10 listings submits a new listing in the running stack
- **THEN** the form shows `Bạn đã đạt giới hạn số tin đăng của gói hiện tại.` and a `Nâng cấp gói` link that opens `/seller/plans`, and the seller's listing count stays 10
- **VERIFIED BY**: platform-e2e `seller/create-listing.feature::Eleventh listing on FREE shows the upgrade CTA`

#### Scenario: Boundary, the tenth listing is accepted
- **WHEN** a FREE seller who owns 9 listings submits a valid new listing in the running stack
- **THEN** the listing is created and no `Nâng cấp gói` link is shown
- **VERIFIED BY**: platform-e2e `seller/create-listing.feature::Boundary, the tenth listing is accepted`
