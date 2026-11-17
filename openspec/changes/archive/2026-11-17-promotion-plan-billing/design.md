## Context

team-promotion owns plans (`subscription_plans`, `seller_subscriptions`), team-payment owns the wallet. The initiative
fixed: wallet-paid (`ChargeSellerFee`), 30-day term, upgrade immediate and unprorated, downgrade and cancel at period end,
auto-renew on, failed renewal means FREE.

## Decisions

1. **Charge then commit, under a row lock.** `Subscribe` opens a transaction, `SELECT ... FOR UPDATE` the seller's row
   (inserting a placeholder FREE-less row when none exists, with a generated id and `period_seq = 0`), calls
   `ChargeSellerFee`, then updates the row and commits. A crash between charge and commit is healed by the retry: same
   key, `created=false`, activation proceeds. Holding a row lock across a 2 s gRPC call is acceptable because the lock is
   per seller and contention is the same seller's own concurrent requests.
2. **Key = `sub:<id>:<period_seq+1>:<plan_id>`**, not period start. `period_seq` is monotonic and survives retries; a
   `now`-based key would differ per retry and double charge. Including the plan id means a retry for a different plan at
   the same sequence is a new charge; the earlier fee is then orphaned (accepted: needs a client that changes its mind
   between a lost response and the retry; visible in the wallet).
3. **No subscription row for FREE.** A seller with no ACTIVE row is FREE (existing behaviour, `GetEntitlements`).
   Lapse sets `status` to `expired` (charge failed or downgrade to FREE) or `cancelled` (auto_renew was off); the unique
   `seller_id` row is kept so re-subscribing upserts and `period_seq` continues, keeping keys unique.
4. **Tier comparison** uses `plan.tier` (FREE 1 < PRO 2 < PREMIUM 3). Upgrade replaces the running period; the remaining
   days of the old period are forfeited (no proration, locked decision).
5. **Renewal starts the new period at the renewal instant**, not at the old end, so downtime of the sweeper never causes
   multiple back-to-back catch-up charges.
6. **Transient vs definitive failure.** Only `FAILED_PRECONDITION` (insufficient withdrawable balance) lapses at once;
   other errors retry until the retry window closes. This keeps a team-payment restart from demoting every seller.
7. **Price correction in the same migration**, idempotent `UPDATE ... WHERE id = ... `, so environments seeded earlier
   converge. `DefaultPlans()` is the single Go source and a test pins it equal to the migrated rows.
8. **Gateway forwards only the two new seller RPCs;** `ChargeSellerFee` stays internal.

## Risks

- Lock held over a network call: bounded by `UPSTREAM_PAYMENT_TIMEOUT`.
- Sweeper and Subscribe racing on one row: both use `FOR UPDATE` (sweeper with `SKIP LOCKED`, so it skips a row mid-Subscribe
  and picks it up on the next tick).
- Migration number clash with the ads slices: build serialises these slices; renumber on rebase.
