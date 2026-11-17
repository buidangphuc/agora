# promotion-plan-billing Specification

## Purpose
Defines paid seller plans in team-promotion: integer VND prices per 30-day term, charging the seller wallet on subscribe and renewal, downgrade, cancel and resume at the period boundary, and the renewal sweep that renews, defers or lapses due subscriptions.

## Requirements

### Requirement: Plan catalog prices are integer VND per 30-day term

`ListPlans` SHALL return FREE price 0, PRO price 199000 and PREMIUM price 499000, each with `term_days` 30, as integer VND per term. A migration SHALL correct existing rows that hold 19900000 and 49900000.

#### Scenario: Catalog shows corrected prices
- **WHEN** a client calls `ListPlans` on a database migrated from the previous seed
- **THEN** the plans are returned as (0, 30), (199000, 30) and (499000, 30) for price and term_days in tier order
- **VERIFIED BY**: team-promotion `internal/store` `TestMigrationCorrectsPlanPrices_CatalogShowsCorrectedPrices`

#### Scenario: Go catalog and migrated rows agree
- **WHEN** `DefaultPlans()` is compared with the rows of `subscription_plans` after all migrations
- **THEN** id, tier, price, term_days and limits are equal for all three plans
- **VERIFIED BY**: team-promotion `internal/store` `TestDefaultPlansMatchMigratedCatalog_GoCatalogAndMigratedRowsAgree`

#### Scenario: Existing paid subscription is grandfathered for one term
- **WHEN** the migration runs over an ACTIVE PRO row that has no period
- **THEN** the row has `current_period_end` 30 days after migration time, `auto_renew` true and no `ChargeSellerFee` call is made
- **VERIFIED BY**: team-promotion `internal/store` `TestMigrationGrandfathersActiveSubscriptions_ActivePaidRowGetsOneTerm`

#### Scenario: Non-positive term is rejected by the schema
- **WHEN** a plan row is inserted or updated with `term_days` 0
- **THEN** the database rejects the write with a check-constraint violation and the catalog is unchanged
- **VERIFIED BY**: team-promotion `internal/store` `TestPlanTermDaysMustBePositive_SchemaRejectsZero`

### Requirement: Subscribing to a higher plan charges the wallet and starts a term

When a seller subscribes to a plan whose tier is higher than the seller's current plan (FREE when none), the system SHALL call `PaymentService.ChargeSellerFee` with the seller's id, the full new-plan price read from the catalog, `kind` SUBSCRIPTION and `reference_id` `sub:<subscription_id>:<period_seq+1>:<plan_id>`, and only after success SHALL activate the plan with `current_period_start` = now, `current_period_end` = now + `term_days`, `auto_renew` true and no pending plan. The seller id SHALL come only from the authenticated principal.

#### Scenario: FREE seller subscribes to PRO
- **WHEN** a FREE seller whose withdrawable balance is 500000 calls `Subscribe(plan_pro)`
- **THEN** exactly one `ChargeSellerFee` for amount 199000 is made, the response subscription is ACTIVE with `current_period_end` 30 days after now and `auto_renew` true, and `GetEntitlements` returns tier PRO
- **VERIFIED BY**: team-promotion `internal/subscription` `TestSubscribeUpgradeChargesAndActivates`

#### Scenario: Insufficient withdrawable balance is rejected
- **WHEN** a FREE seller whose withdrawable balance is 100000 calls `Subscribe(plan_pro)` and team-payment answers FAILED_PRECONDITION
- **THEN** `Subscribe` returns FAILED_PRECONDITION, the seller stays FREE and no subscription row is ACTIVE
- **VERIFIED BY**: team-promotion `internal/subscription` `TestSubscribeInsufficientBalanceChangesNothing`

#### Scenario: Payment service unavailable fails without state change
- **WHEN** `Subscribe(plan_pro)` is called and `ChargeSellerFee` times out after 2 s
- **THEN** `Subscribe` returns UNAVAILABLE and the stored subscription is byte-for-byte the previous one
- **VERIFIED BY**: team-promotion `internal/subscription` `TestSubscribePaymentUnavailableChangesNothing`

#### Scenario: Retry after a lost response is not charged twice
- **WHEN** `ChargeSellerFee` succeeded but the commit failed, and the seller calls `Subscribe(plan_pro)` again
- **THEN** the second call sends the same `reference_id`, receives `created=false`, activates PRO and the wallet holds one SUBSCRIPTION_FEE of 199000
- **VERIFIED BY**: team-promotion `internal/subscription` `TestSubscribeRetryReplaysChargeKey`

#### Scenario: Upgrade charges the full price without proration
- **WHEN** a PRO seller 10 days into a term calls `Subscribe(plan_premium)`
- **THEN** the charge amount is 499000, the new `current_period_start` is now and `current_period_end` is now + 30 days
- **VERIFIED BY**: team-promotion `internal/subscription` `TestSubscribeUpgradeNoProration`

#### Scenario: Missing or unknown plan id is invalid
- **WHEN** `Subscribe` is called with an empty `plan_id` or an id absent from the catalog
- **THEN** it returns INVALID_ARGUMENT for the empty id and NOT_FOUND for the unknown id, and no `ChargeSellerFee` call is made
- **VERIFIED BY**: team-promotion `internal/rpc` `TestSubscribeRejectsBadPlanWithoutCharging`

#### Scenario: Subscribe without seller scope is denied
- **WHEN** a principal without `listing.write` calls `Subscribe`
- **THEN** it returns PERMISSION_DENIED and no `ChargeSellerFee` call is made
- **VERIFIED BY**: team-promotion `internal/rpc` `TestSubscribeRequiresSellerScope`

#### Scenario: Concurrent subscribes charge once
- **WHEN** two `Subscribe(plan_pro)` calls for the same seller run concurrently against a real database
- **THEN** one SUBSCRIPTION_FEE entry exists and both calls return an ACTIVE PRO subscription
- **VERIFIED BY**: team-promotion `internal/store` `TestConcurrentSubscribeSingleCharge_SubscriptionRowTwoCallsChargeOnce`

### Requirement: Downgrade, cancel and resume take effect at the period boundary

When a seller subscribes to a lower-tier plan the system SHALL charge nothing, keep the current plan until `current_period_end` and record the lower plan as pending. `CancelSubscription` SHALL set `auto_renew` false, clear any pending plan and keep the plan until `current_period_end`. Subscribing to the current plan SHALL charge nothing and SHALL revert a pending downgrade and `auto_renew` false.

#### Scenario: Downgrade is deferred
- **WHEN** a PREMIUM seller with 12 days left calls `Subscribe(plan_pro)`
- **THEN** no `ChargeSellerFee` call is made, `GetEntitlements` still returns PREMIUM and `GetMySubscription` shows the same `current_period_end`
- **VERIFIED BY**: team-promotion `internal/subscription` `TestSubscribeDowngradeDeferred`

#### Scenario: Cancel keeps the plan until the end
- **WHEN** a PRO seller calls `CancelSubscription`
- **THEN** the response has `auto_renew` false, the same `current_period_end` and the tier PRO, and a second `CancelSubscription` returns the same state
- **VERIFIED BY**: team-promotion `internal/subscription` `TestCancelStopsAutoRenewKeepsPlan`

#### Scenario: Cancel without a paid plan is rejected
- **WHEN** a FREE seller calls `CancelSubscription`
- **THEN** it returns FAILED_PRECONDITION and no row is written
- **VERIFIED BY**: team-promotion `internal/subscription` `TestCancelFreeSellerRejected`

#### Scenario: Re-subscribing to the current plan resumes renewal
- **WHEN** a PRO seller with `auto_renew` false calls `Subscribe(plan_pro)`
- **THEN** no `ChargeSellerFee` call is made and the subscription has `auto_renew` true
- **VERIFIED BY**: team-promotion `internal/subscription` `TestSubscribeCurrentPlanResumes`

#### Scenario: Cancel without seller scope is denied
- **WHEN** a principal without `listing.write` calls `CancelSubscription`
- **THEN** it returns PERMISSION_DENIED and the subscription is unchanged
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCancelSubscriptionRequiresSellerScope`

### Requirement: GetMySubscription and CancelSubscription act on the caller's own subscription

`GetMySubscription` and `CancelSubscription` SHALL read and change only the subscription of the authenticated seller. `GetMySubscription` SHALL return an empty response for a seller on FREE. team-gateway SHALL forward both RPCs and SHALL NOT expose `ChargeSellerFee`.

#### Scenario: Paid seller reads own subscription
- **WHEN** a PRO seller calls `GetMySubscription`
- **THEN** the response carries plan PRO, status ACTIVE, `current_period_end` and `auto_renew`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestGetMySubscriptionReturnsOwn`

#### Scenario: FREE seller gets an empty response
- **WHEN** a seller with no ACTIVE row calls `GetMySubscription`
- **THEN** the response has no subscription and the status code is OK
- **VERIFIED BY**: team-promotion `internal/rpc` `TestGetMySubscriptionFreeIsEmpty`

#### Scenario: Another seller's subscription is not reachable
- **WHEN** seller B calls `GetMySubscription` or `CancelSubscription` while seller A holds PRO
- **THEN** B sees no subscription for A, A's `auto_renew` stays true and no request field can name A
- **VERIFIED BY**: team-promotion `internal/rpc` `TestSubscriptionRPCsIgnoreOtherSellers`

#### Scenario: Unauthenticated call is denied at the edge
- **WHEN** a client without a token calls `GetMySubscription` or `CancelSubscription` through team-gateway
- **THEN** the gateway returns UNAUTHENTICATED and team-promotion receives no call
- **VERIFIED BY**: team-gateway `internal/edge` `TestSubscriptionRoutesRequireAuth`

#### Scenario: Gateway forwards the new RPCs and never ChargeSellerFee
- **WHEN** an authenticated seller calls both new RPCs through the gateway and a client requests `ChargeSellerFee`
- **THEN** the two calls reach team-promotion and the `ChargeSellerFee` path is not registered (404)
- **VERIFIED BY**: team-gateway `internal/edge` `TestSubscriptionForwardsNewRPCsOnly`

### Requirement: The renewal sweep renews, defers or lapses due subscriptions

For each ACTIVE subscription with `current_period_end` <= now the sweeper SHALL, under `FOR UPDATE SKIP LOCKED`: set the seller to FREE with status `cancelled` when `auto_renew` is false; otherwise charge the target plan (pending plan if set, else the current plan) once per period with key `sub:<id>:<period_seq+1>:<plan_id>` and start a new term at the renewal instant; lapse to FREE with status `expired` when the target is FREE or the charge returns FAILED_PRECONDITION. A transient charge error SHALL leave the row unchanged until `SUBSCRIPTION_RENEWAL_RETRY_WINDOW` after the period end, then lapse to FREE.

#### Scenario: Auto-renew charges one fee per period
- **WHEN** a PRO seller with `auto_renew` true and a funded wallet reaches `current_period_end` and the sweep runs twice
- **THEN** one SUBSCRIPTION_FEE of 199000 exists for the period and `current_period_end` moved forward by 30 days
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweepRenewsOncePerPeriod`

#### Scenario: Cancelled subscription lapses to FREE
- **WHEN** the period of a PRO seller with `auto_renew` false ends and the sweep runs
- **THEN** no charge is made, `GetEntitlements` returns FREE and the stored status is `cancelled`
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweepCancelledLapsesToFree`

#### Scenario: Pending downgrade renews at the lower price
- **WHEN** the period of a PREMIUM seller with pending plan PRO ends and the sweep runs
- **THEN** the charge amount is 199000, the plan is PRO and `pending_plan_id` is empty
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweepAppliesPendingDowngrade`

#### Scenario: Failed renewal charge sets the seller to FREE
- **WHEN** the renewal charge returns FAILED_PRECONDITION
- **THEN** `GetEntitlements` returns FREE, the stored status is `expired` and the seller's existing listings are untouched
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweepInsufficientBalanceLapsesToFree`

#### Scenario: Transient payment error is retried inside the window
- **WHEN** the charge returns UNAVAILABLE 10 minutes after the period end with a 1 h retry window
- **THEN** the row stays ACTIVE and unchanged, and the next sweep charges again with the same key
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweepTransientErrorRetriesWithinWindow`

#### Scenario: Retry window exhausted lapses the seller
- **WHEN** the charge still returns UNAVAILABLE 61 minutes after the period end
- **THEN** the seller lapses to FREE with status `expired`
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweepTransientErrorPastWindowLapses`

#### Scenario: Two sweepers do not double charge
- **WHEN** two sweepers run concurrently against one due row on a real database
- **THEN** one SUBSCRIPTION_FEE entry exists for the period
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweepConcurrentSweepersSingleCharge`

### Requirement: The payment upstream is required configuration

team-promotion SHALL read `UPSTREAM_PAYMENT_ADDR` and `UPSTREAM_PAYMENT_TIMEOUT` (default 2s, must be > 0) and `SUBSCRIPTION_RENEWAL_RETRY_WINDOW` (default 1h, must be > 0) and SHALL refuse to start when the address is blank or a duration is not positive. No `*_ENABLED` flag SHALL exist.

#### Scenario: Blank payment address is rejected at startup
- **WHEN** the service starts with `UPSTREAM_PAYMENT_ADDR` empty
- **THEN** config validation fails with an error naming `UPSTREAM_PAYMENT_ADDR` and the process does not serve
- **VERIFIED BY**: team-promotion `internal/config` `TestUpstreamRequiresPaymentAddr`

#### Scenario: Non-positive timeout is invalid
- **WHEN** `UPSTREAM_PAYMENT_TIMEOUT` is 0s
- **THEN** config validation fails with an error naming `UPSTREAM_PAYMENT_TIMEOUT`
- **VERIFIED BY**: team-promotion `internal/config` `TestUpstreamRejectsNonPositivePaymentTimeout`

### Requirement: Paid plan lifecycle works end to end on the real stack

The stack SHALL charge a real wallet through team-payment for a plan subscribed through team-gateway, and SHALL reflect upgrade, cancel and lapse in `GetMySubscription` and in the seller wallet ledger.

#### Scenario: Seller upgrades to PRO and the wallet is debited
- **WHEN** a seller with a withdrawable balance of at least 199000 calls `Subscribe(plan_pro)` through the gateway
- **THEN** `GetMySubscription` shows PRO ACTIVE with a period end 30 days ahead and the wallet shows one SUBSCRIPTION_FEE of -199000
- **VERIFIED BY**: platform-e2e `features/promotion/seller_plan_billing.feature` "Seller upgrades to PRO and the wallet is debited"

#### Scenario: Seller without funds cannot upgrade
- **WHEN** a seller with a zero wallet calls `Subscribe(plan_pro)` through the gateway
- **THEN** the call fails with FAILED_PRECONDITION and `GetMySubscription` is still empty
- **VERIFIED BY**: platform-e2e `features/promotion/seller_plan_billing.feature` "Seller without funds cannot upgrade"

#### Scenario: Cancelled plan lapses at period end
- **WHEN** a PRO seller cancels and the period end is brought forward by the test hook of the stack and the sweep runs
- **THEN** `GetEntitlements` returns FREE and no second SUBSCRIPTION_FEE exists
- **VERIFIED BY**: platform-e2e `features/promotion/seller_plan_billing.feature` "Cancelled plan lapses at period end"

#### Scenario: Another seller cannot cancel my plan
- **WHEN** seller B calls `CancelSubscription` through the gateway while seller A holds PRO
- **THEN** A's `GetMySubscription` still shows `auto_renew` true
- **VERIFIED BY**: platform-e2e `features/promotion/seller_plan_billing.feature` "Another seller cannot cancel my plan"
