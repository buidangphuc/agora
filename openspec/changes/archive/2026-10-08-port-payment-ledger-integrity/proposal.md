## Why

The retired `full_team_repo` made seller money correct; agora's `team-payment` does not (audit
`agora-archive/full_team_repo/port-audit/team-payment.md`, items 1, 2, 3 and 6). Today:

- `ProcessMockPayment` credits the seller **inline and best-effort** (`creditSellerWallet` in
  `internal/service/payment.go`): a failed `GetOrder` or insert loses the credit for good, the row carries no
  reference to the payment, and nothing stops a second credit. It also credits at payment time, so a late payment
  for an order that `team-order` already cancelled (and, per `order-lifecycle-guards`, never marks `Paid`) still pays
  the seller.
- `RefundPayment` only flips the transaction to `REFUNDED`; the seller keeps the refunded money. The status flip is
  not a compare-and-set either.
- Nothing holds fresh proceeds back, so a seller can withdraw a sale the moment it settles and leave the platform to
  fund a later refund.
- A buyer may cancel a `Paid` order (`order-lifecycle-guards`), but nothing refunds the buyer or takes the money back
  from the seller: `team-order` emits no cancellation fact and `team-payment` never hears of it.

## What Changes

- **team-payment, seller settlement credit.** The seller is credited from `team-order`'s durable domain fact
  `OrderPaidEvent` on `order.events`, which `team-order` writes once, in the same transaction as the only
  `Pending → Paid` compare-and-set. A new consumer group in `team-payment` appends one `ORDER_SETTLEMENT` credit per
  payment transaction (amount = the transaction amount, seller = the event's seller), idempotently, with bounded
  retry, a dead-letter topic and commit-after-apply. **The inline credit in `ProcessMockPayment` is removed.** A
  cancelled order's late payment produces no `OrderPaidEvent` and so never credits the seller.
- **team-payment, ledger integrity (migration `0006`).** `wallet_ledger.reference_id`; a unique index on
  `(type, reference_id)`; a type/sign `CHECK` (settlement > 0, refund deduction < 0, payout < 0 except a `REJECTED`
  compensating reversal > 0); a reference required for settlement and refund-deduction rows (enforced for new rows,
  legacy rows kept); `payment_transactions.refunded_amount`.
- **team-payment, refund deduction.** `RefundPayment` moves `PAID → REFUNDED` by compare-and-set and, when the
  payment was credited, appends one `REFUND_DEDUCTION` of the refunded amount for the credited seller. If the refund
  lands before the credit, the credit path appends the deduction too. Either order yields exactly one credit and one
  deduction; a payment that was never credited is never deducted. Refunds are never blocked by balance or hold.
- **Cancel of a paid order refunds automatically (decided by the human).** `team-order` writes a new
  `OrderCancelled` fact (with the status the order was cancelled from) to `order.events` through its outbox, in the
  same transaction as the compare-and-set claim to `Cancelled`. `team-payment` consumes it in the same durable
  consumer and, when the order was cancelled from `Paid`, refunds the full payment through the same refund path
  (`PAID → REFUNDED` compare-and-set + exactly one `REFUND_DEDUCTION` for a credited payment). A payment already
  refunded is left alone; redelivery is a no-op; the cancel and the credit may arrive in either order.
- **team-payment, payout hold-back.** Withdrawable = `max(0, balance − held)`, where `held` is the unrefunded part of
  every settlement credit still inside the hold window. `RequestWalletPayout` and `RequestPayout` refuse anything
  above it with `FAILED_PRECONDITION` `amount is held until <RFC3339> (refund window)`; a request above the balance
  keeps `insufficient wallet balance`. `PAYOUT_HOLD_DAYS` (default 7, 0–3650, 0 = off) with an optional
  `PAYOUT_HOLD_WINDOW` Go-duration override (used by the e2e overlay); invalid values refuse startup.
  `GetWalletBalance` / `GetSellerWallet` keep returning the full ledger sum.
- **Root compose.** `team-payment` gets the consumer env; `redpanda-init` creates the new DLQ topic.
- **platform-e2e.** New overlay `compose/payment-ledger.override.yaml` (`PAYOUT_HOLD_WINDOW=20s` on
  `team-payment`), new features for the three capabilities, and the existing bank-payout scenario waits out the hold.
- **platform-core docs.** ADR-0009 addendum: `team-payment` consumes `order.events` for the settlement credit.
- **platform-core proto (lands first, alone, additive).** New message `platform.order.v1.OrderCancelled`
  (`order_id`, `buyer_id`, `seller_id`, `previous_status`, `total_amount`, `currency`, `cancelled_at`) in
  `order/v1/order.proto`. No existing message or RPC changes. Re-vendored by `team-order` (producer) and
  `team-payment` (consumer); other `order.events` consumers (`team-notification`, `team-analytics`) filter by
  envelope type and need no change.
- **team-order.** The cancel claim (`CancelOrder`, `ForceFailSaga`) writes the `OrderCancelled` outbox row in its
  transaction; the existing relayer publishes it.
- Supersedes the carried-over `payment-payout-holdback-and-settled-consumer` (written for the old code); it is
  retired by this change.

## Capabilities

### New Capabilities

- `seller-settlement-credit`: one durable, idempotent seller credit per paid order, driven by `OrderPaidEvent`, and
  the ledger store's uniqueness and sign rules.
- `seller-refund-deduction`: one refund deduction per refunded, credited payment, whatever order the refund and the
  credit arrive in, including the automatic full refund when a `Paid` order is cancelled.
- `seller-payout-holdback`: payouts draw only on proceeds outside the refund hold window; error shapes and the
  window's configuration.

### Modified Capabilities

(none: `payment-access-control` and `order-lifecycle-guards` keep their requirements; this change relies on
`order-lifecycle-guards` "Only a still-pending order is marked paid by settlement".)

## Non-goals

- Refund UX, automatic refunds on return approval, real PSP or real payouts, AI-first work.
- Refunding a payment that succeeded late for an order cancelled while still `Pending` (cancelled from `Pending`, so
  no refund is triggered; the seller is not credited either). Only cancels from `Paid` refund automatically.
- A wallet "available / held" field on the wire (needs a proto change; the hold is visible through the payout error).
- Strict per-order ordering of `payment.events` (audit item 6). The current claim **can** overtake (a backed-off row
  lets a newer row of the same order through, and one failed row does not stop the rest of its batch), but
  `payment.events` carries only `PaymentSettled`, which is emitted only as `PAID`, and `team-order` applies it by
  compare-and-set; overtaking between duplicate `PAID` events has no observable effect. It must be done before a
  second payment event type (e.g. `PaymentRefunded`) is added. This change adds none.
- Making the settle-path status write in `ProcessMockPayment` a compare-and-set (a pre-existing race unrelated to
  ledger money, see design Risks).
- Kafka wiring for `team-payment` in `platform-gitops` deployed envs (already absent: `payment.events` is not relayed
  there today, so neither payment nor credit flows exist there).

## Impact

- Repos: `platform-core` (proto `OrderCancelled`, ADR-0009 addendum), `team-order` (cancel fact in the claim
  transaction, re-vendor), `team-payment` (code, migration `0006`, config, README, `.env.example`, `FEATURES.yaml`,
  re-vendor), root `docker-compose.services.yaml`, `platform-e2e`. `team-gateway`, `team-frontend`: no change.
- Architecture rules: 3 respected (team-payment reads only its own DB; seller and amount come from the event and its
  own transaction, no new cross-service call; `team-order` never touches payment data), 4 respected (the new fact is
  defined in `platform-core` first, additively), 5 respected (state-change facts on Kafka `order.events`, keyed by
  order id).
- **Behaviour change**: with the default 7-day hold a seller cannot withdraw a sale the moment it settles; the credit
  now appears a few seconds after payment (two outbox hops) instead of synchronously; with Kafka off there is no
  credit (there is no `Paid` order either).
- First deploy: legacy credits have no reference; the new consumer group starts at the latest offset so historical
  `OrderPaidEvent`s are not re-credited (design D10).
