## Why

initiative: monetization

Seller plans are free and permanent today: `Subscribe` upserts an ACTIVE row with no charge and no term
(`team-promotion/internal/subscription/subscription.go:37-61`, no term column in `0002_subscriptions.up.sql`), and the
catalog stores PRO at 19 900 000 and PREMIUM at 49 900 000 under a "minor units" comment, so the seller UI shows
PRO at 19.900.000 VND instead of 199.000 VND. The initiative decided that plans cost money for a 30-day term, are paid
from the seller's wallet by `ChargeSellerFee`, renew automatically, and lapse to FREE when a renewal cannot be paid.
This slice makes team-promotion own that lifecycle: price correction, term columns, a charged `Subscribe`, the new
`GetMySubscription` and `CancelSubscription` RPCs, the renewal sweep, and the gateway routes.

## What Changes

- Migration `0009_plan_billing` (renumber to the next free number at build time; hot file shared with the ads slices):
  `subscription_plans.term_days INT NOT NULL DEFAULT 30`; plan prices set to FREE 0, PRO 199000, PREMIUM 499000 (integer
  VND per term, replacing the 100x values, and the column comment); `seller_subscriptions` gains `current_period_start`,
  `current_period_end`, `auto_renew BOOLEAN NOT NULL DEFAULT true`, `pending_plan_id TEXT NULL`, `period_seq INT NOT NULL
  DEFAULT 0`. Existing ACTIVE rows on a paid plan are grandfathered for one free term: period start = migration time, end =
  +30 days, auto_renew true, no charge booked (see Open decisions). `DefaultPlans()` and the SQL seed stay in sync.
- `Subscribe(plan_id)` becomes a paid action for the calling seller:
  - No paid plan to a paid plan, or a higher tier than the current one (upgrade): charge the full new-plan price
    (no proration) with `PaymentService.ChargeSellerFee(kind=SUBSCRIPTION)`, then activate the plan with a new period
    `[now, now + term_days]`, `auto_renew = true`, `pending_plan_id` cleared. A paid upgrade replaces the running period.
  - A lower tier than the current one (downgrade, including to FREE): nothing is charged; `pending_plan_id` is set and the
    current plan stays until `current_period_end`.
  - The current plan again: no charge; if `auto_renew` was false or a downgrade was pending, both are reverted (resume);
    otherwise the stored subscription is returned unchanged.
  - Subscribing to FREE with no subscription is a no-op returning no subscription.
- Charge key: `sub:<subscription_id>:<period_seq+1>:<plan_id>`. The subscription row is locked `FOR UPDATE` across the
  call so a retried request after a lost response replays the key (`created=false`) and completes activation without a
  second debit. `FAILED_PRECONDITION` from team-payment is returned as `FAILED_PRECONDITION` with nothing changed; any
  other upstream error is `UNAVAILABLE` with nothing changed.
- `GetMySubscription`: the caller's own ACTIVE subscription with `current_period_end`, `auto_renew`; FREE sellers get
  an empty response. `CancelSubscription`: sets `auto_renew = false` and clears `pending_plan_id`; the plan stays until
  `current_period_end`; idempotent; `FAILED_PRECONDITION` when the caller has no paid subscription.
- Renewal sweep inside the existing sweeper loop (`internal/consumer/sweeper.go`, selecting due rows with `FOR UPDATE SKIP
  LOCKED`): for each ACTIVE row with `current_period_end <= now`:
  `auto_renew = false` lapses to FREE (status `cancelled`); otherwise the target plan is `pending_plan_id` if set else the
  current plan, FREE target lapses (status `expired`); a paid target is charged with key `sub:<id>:<period_seq+1>:<plan_id>`
  and, on success, gets a new period starting at the renewal instant with the target plan and `pending_plan_id` cleared. A
  `FAILED_PRECONDITION` charge lapses the seller to FREE (status `expired`). A transient error (UNAVAILABLE, timeout) leaves
  the row untouched and is retried on the next sweep until `SUBSCRIPTION_RENEWAL_RETRY_WINDOW` (default 1h) after the period
  end, then the seller lapses to FREE. Lapsed sellers keep their listings; only new creates are capped by `listing-plan-cap`.
- `GetEntitlements` is unchanged in shape: ACTIVE row gives the plan's limits, otherwise FREE.
- New config (required, no `*_ENABLED` flags): `UPSTREAM_PAYMENT_ADDR`, `UPSTREAM_PAYMENT_TIMEOUT` (default 2s),
  `SUBSCRIPTION_RENEWAL_RETRY_WINDOW`. A new `internal/upstream/payment.go` client attaches a service principal.
- team-gateway: forwarders for `GetMySubscription` and `CancelSubscription` in `internal/edge/promotion.go`
  (`callRead` / `callWrite`, seller auth as `Subscribe`). `ChargeSellerFee` gets no route.
- docker-compose: `UPSTREAM_PAYMENT_ADDR=team-payment-svc:<port>` on team-promotion in `docker-compose.services.yaml`
  (shared with `listing-plan-cap`, which adds `UPSTREAM_PROMOTION_ADDR` on team-domain).
- e2e: scenarios in `platform-e2e/tests/e2e/features/promotion/seller_plan_billing.feature` on the real stack.
- Consumes the proto added by `monetization-contract` (`Plan.term_days`, `SellerSubscription.current_period_end`,
  `auto_renew`, the two RPCs) and `PaymentService.ChargeSellerFee` from `payment-seller-fee-charge`. No proto edit here.

## Capabilities

### New Capabilities
- `promotion-plan-billing`: paid, termed, renewable seller plans, the two subscription RPCs and their gateway routes.

### Modified Capabilities
none (`promotion-service-runtime` does not pin the Subscribe behaviour; the free `Subscribe` is superseded by the
requirements here).

## Impact

- team-promotion: `internal/subscription`, `internal/store/subscription.go`, new migration, `internal/rpc/subscription.go`,
  `internal/consumer/sweeper.go`, `internal/upstream/payment.go`, `internal/config`, `internal/app` wiring, README env table.
- team-gateway: `internal/edge/promotion.go` and its route registration. docker-compose.services.yaml: one env line.
- platform-e2e: one feature file. Consumers: `seller-plan-ui` (plan page) and `listing-plan-cap` (entitlements, unchanged shape).

## NFR

- Latency: `GetMySubscription` and `CancelSubscription` p95 <= 50 ms locally; `Subscribe` p95 <= 400 ms locally including
  one `ChargeSellerFee` call (upstream timeout 2 s, then UNAVAILABLE). `GetEntitlements` p95 unchanged (one indexed read).
- Availability: team-payment down means `Subscribe` and renewals fail without state change; reads, cancel and entitlements
  keep working. A renewal is attempted within 2 sweep intervals of the period end; backlog metric
  `subscription_renewals_overdue` exposes a stalled sweeper.
- Consistency: at most one fee per (subscription, period_seq, plan); the sum of SUBSCRIPTION_FEE entries for a seller equals
  the sum of charged periods; concurrent Subscribe calls for the same seller serialise on the row lock.
- Money: integer VND int64, no proration, no fractional units.

## Data classification

| Field | Purpose | Consent basis | Retention |
|---|---|---|---|
| `seller_subscriptions.seller_id` (pseudonymous account id) | bind the plan to the seller for billing and entitlements | contract performance (seller plan purchase) | life of the seller account; lapsed rows kept 12 months for billing disputes, then purged by the existing purge loop |
| `current_period_start/end`, `auto_renew`, `pending_plan_id`, `period_seq` | term and renewal state | contract performance | as the row |
| fee amount and `reference_id` sent to team-payment | idempotent wallet debit | contract performance | held by team-payment's ledger retention |

No buyer data. Logs carry a seller-id fingerprint, never the raw id (sloglint forbidden-keys). Whether 12 months of lapsed
rows is acceptable is a DPO item (see Open decisions).

## Threat model

- Assets: seller wallet balance (debited), plan entitlements (money-equivalent perks), ledger integrity.
- Abuse cases: (1) a seller subscribing on behalf of another seller: seller id comes only from the authenticated principal,
  never the request; (2) double charge by client retry, concurrent tabs or a sweeper race: row lock plus the period-keyed
  `reference_id` makes the debit idempotent; (3) free upgrades by racing Subscribe and Cancel: both take the row lock;
  (4) calling `ChargeSellerFee` through the edge to debit any seller: no gateway route, service-only in team-payment;
  (5) keeping a paid plan after a failed renewal: the sweep lapses on `FAILED_PRECONDITION` and after the retry window;
  (6) price tampering: the amount is read from `subscription_plans`, never from the request.
- Authz rule: `Subscribe`, `GetMySubscription`, `CancelSubscription` require a seller principal with `listing.write`
  (as `Subscribe` today) and act on the caller's own seller id; `ListPlans` stays public; `GetEntitlements` keeps its rule
  (own id, or any id for a service principal).

## Open decisions (for a human)

- Grandfathering: existing ACTIVE paid rows get one free 30-day term (assumed) rather than being lapsed at deploy.
- Transient renewal failures retry for `SUBSCRIPTION_RENEWAL_RETRY_WINDOW` (1h assumed) before lapsing; the locked
  spec says only "if the renewal charge fails the seller is set to FREE".
- Retention of lapsed subscription rows (12 months assumed) needs DPO sign-off.
