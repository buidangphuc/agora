## ADDED Requirements

### Requirement: Erasing a seller ends their subscription without a charge

On `identity.events/UserErased` team-promotion SHALL, in the same transaction as the voucher erasure and keyed only by the event's `user_id`, set the seller's `seller_subscriptions` row to plan FREE with status `cancelled`, `auto_renew` false and no pending plan, keeping `id`, `period_seq` and the period dates. It SHALL NOT call `ChargeSellerFee` or refund anything, the renewal sweep SHALL never select that row again, and a seller with no row or with a row that is not ACTIVE, not auto-renewing and has no pending plan SHALL be left unchanged. Redelivery SHALL change nothing, and a store error SHALL roll the whole erasure back for retry.

#### Scenario: Erasure ends an auto-renewing paid plan now
- **GIVEN** seller U holds PRO ACTIVE with `auto_renew` true and a pending downgrade to FREE, and seller W holds PREMIUM ACTIVE with `auto_renew` true
- **WHEN** a `UserErased` event for U is consumed
- **THEN** U's row has plan `plan_free`, status `cancelled`, `auto_renew` false, no pending plan and the same `id` and `period_seq`, `GetActiveSubscriptionBySeller(U)` returns not found, and W's row is unchanged
- **VERIFIED BY**: team-promotion `internal/store` `TestEraseUserEndsPaidSubscription`

#### Scenario: The renewal sweep never charges an erased seller
- **GIVEN** U's PRO subscription was ended by erasure
- **WHEN** the renewal sweep lists due subscriptions at a time after U's old period end
- **THEN** U's row is not listed, so no SUBSCRIPTION_FEE is charged for it
- **VERIFIED BY**: team-promotion `internal/store` `TestEraseUserSubscriptionNotDueForRenewal`

#### Scenario: A seller without a paid plan is unchanged
- **GIVEN** seller V has an expired FREE row with `auto_renew` false and no pending plan, and user X has no subscription row
- **WHEN** a `UserErased` event for V and then one for X are consumed
- **THEN** V's row is byte-identical to before and no row exists for X
- **VERIFIED BY**: team-promotion `internal/store` `TestEraseUserEndsPaidSubscription`

#### Scenario: Redelivery leaves the ended subscription unchanged
- **GIVEN** a `UserErased` event for U was consumed
- **WHEN** the same event and a second event for U are consumed
- **THEN** both complete without error and U's subscription row is unchanged by them
- **VERIFIED BY**: team-promotion `internal/store` `TestEraseUserIdempotent`

#### Scenario: A store error leaves the plan running for retry
- **GIVEN** the promotion database fails after the subscription update but before the commit
- **WHEN** a `UserErased` event for U is consumed
- **THEN** U's subscription is still PRO ACTIVE with `auto_renew` true, U's vouchers are still active, the error is returned for retry, and after recovery the subscription is ended
- **VERIFIED BY**: team-promotion `internal/store` `TestEraseUserMonetizationRollsBack`

#### Scenario: An erased seller's plan is never renewed on the real stack
- **GIVEN** a funded seller subscribed to `plan_pro` through the gateway
- **WHEN** a `UserErased` event for that seller is consumed, the old period end is brought forward and the sweep runs
- **THEN** the seller's `GetMySubscription` is empty, `GetEntitlements` returns FREE and the seller's ledger holds exactly one SUBSCRIPTION_FEE of -199000
- **VERIFIED BY**: platform-e2e `features/promotion/seller_erasure_monetization.feature` "An erased seller's plan is never renewed"
