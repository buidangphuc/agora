## Why

Initiatives `monetization` (paid seller plans renewed by a sweeper that charges the wallet through
`ChargeSellerFee`, and seller ad campaigns) and `trust-legal` (account deletion with `UserErased`) merged
separately. team-promotion's `UserErased` handler only deactivates owned vouchers and deletes new-user voucher
state, so an erased seller's auto-renewing plan keeps charging the wallet every term and their ad campaigns keep
being served and charged per click. The user decided on 2026-11-17 that erasure cancels both and does not block
deletion.

## What Changes

- team-promotion `EraseUser` (the `identity.events/UserErased` consumer) also, in the same transaction:
  - ends the seller's subscription now: plan FREE, status `cancelled`, `auto_renew` false, pending downgrade
    cleared, so the renewal sweep never selects it and `ChargeSellerFee` is never called for it again. No refund
    of the unused term (decided 2026-11-17); the paid SUBSCRIPTION_FEE stays in the wallet ledger.
  - ends every ad campaign of the seller that is not already ENDED (ACTIVE, PAUSED, PAUSED_BUDGET become ENDED),
    so no slot is served and no outstanding impression token charges a click. Unspent budget is dropped: it is
    bookkeeping only and was never prepaid from the wallet.
- Idempotent on redelivery; a user with no subscription or campaign is a no-op; a store error rolls back the
  whole erasure (vouchers included) and is retried.
- No proto change, no migration, no new RPC, no new configuration.
- team-identity deletion checks are unchanged: monetization state never blocks deletion.

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `promotion-plan-billing`: an erased seller's subscription ends without a charge.
- `promotion-ads-campaign-mgmt`: an erased seller's campaigns end and stop serving.

## Impact

- `team-promotion`: `internal/store/erasure.go` (two more statements in the erasure transaction), its store tests,
  README Events table, FEATURES.yaml.
- `platform-e2e`: one new feature `tests/e2e/features/promotion/seller_erasure_monetization.feature` with steps.
- No change to team-identity, team-payment or team-gateway.

## Non-goals

Refunding the unused plan term or unspent ad budget; blocking deletion on an active plan or campaign; changing the
renewal sweep itself; erasing the seller's listings (domain-seller-erasure owns them).

## Non-functional requirements

- The subscription and campaign updates run inside the existing erasure transaction: the erasure applies fully or
  not at all, and a store error is retried by the consumer; only a malformed event goes to
  `identity.events.promotion.dlq`.
- Erasure of one seller still completes within 1 h of `UserErased` (initiative NFR); each statement is a single
  indexed UPDATE on `seller_id` (`seller_subscriptions` unique on `seller_id`, `ad_campaigns_seller_idx`).
- A renewal sweep that holds the seller's row lock makes the erasure wait for its commit; the erasure then ends the
  subscription, so at most the charge already in flight completes and no later period is charged.

## Data classification

| Field | Where | Class | Purpose | Basis | Retention |
|---|---|---|---|---|---|
| subscription plan, status, auto-renew, pending plan, period dates | `seller_subscriptions` (team-promotion) | pseudonymous business record (seller id keyed) | billing the seller's plan | contract performance | set to FREE `cancelled` at erasure; row kept for accounting |
| SUBSCRIPTION_FEE entries | team-payment wallet ledger | pseudonymous business record | accounting | legal obligation | unchanged by this change |
| ad campaign listing id, budget, bid, spent, impressions, clicks, status | `ad_campaigns` (team-promotion) | pseudonymous business record | serving and billing ads | contract performance | status ENDED at erasure; counters kept |

No new field is collected, logged or sent. Logs keep the existing hashed-id debug line only.

## Threat model

- Assets: the seller's wallet (renewal charges), ad spend counters, the integrity of other sellers' plans and
  campaigns.
- Abuse cases:
  - A forged `UserErased` cancels a victim seller's plan and ads. Mitigation: unchanged from
    `promotion-user-erasure` PRO-9: erasure is driven only by records read from `identity.events`, which only
    team-identity writes; no RPC triggers it, so there is no new caller-facing permission surface.
  - Over-broad update ends another seller's plan or campaigns. Mitigation: both statements filter on the event's
    `user_id` only; scenarios prove another seller's rows are unchanged.
  - An impression token issued before erasure keeps charging. Mitigation: `RecordAdClick` charges only ACTIVE
    campaigns; a scenario proves the click after erasure is not charged.
  - The renewal sweep charges an erased seller. Mitigation: the row leaves status ACTIVE in the erasure
    transaction, and the sweep selects only ACTIVE rows.
- Authz rule: unchanged; no RPC is added or modified.
