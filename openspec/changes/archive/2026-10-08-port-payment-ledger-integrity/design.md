## Context

See proposal.md (Why). Current agora state that shapes the approach, all verified in code:

- `team-payment` balance = `SUM(wallet_ledger.amount)`; payouts go through `LedgerRepository.AppendDebit`
  (per-seller `pg_advisory_xact_lock(hashtextextended('wallet_ledger:'||seller))`, balance check, insert) and
  `RequestPayout` links `payout_requests.ledger_entry_id` (migration `0005_payout_ledger_link`). If the bank-detail
  insert fails it appends a compensating `PAYOUT` entry of `+amount` with status `REJECTED`.
- `wallet_ledger` has no reference column and no constraint on type or sign (migration `0004`).
- Settlement: `ProcessMockPayment` → `settlePaid` (payment `PAID` + `PaymentSettled` outbox row in one tx) →
  `creditSellerWallet` (best-effort `GetOrder` + `AppendEntry`, no reference). `RefundPayment` only updates status,
  unconditionally, and stores the reason in `provider_reference`; the refunded amount is not stored.
- `team-order` consumes `payment.events` and moves an order `Pending → Paid` by compare-and-set
  (`UpdateOrderStatusFrom`); in **that same transaction** it writes an `OrderPaidEvent` outbox row (stable
  `event_id = OrderPaidEventID(order_id)`, items carry the order's single `seller_id`, `total_amount`, `paid_at`)
  relayed to `order.events` keyed by order id. A settlement for a non-pending order (late payment after cancel) is
  acknowledged without change and emits nothing.
- `team-payment` has a Kafka producer (outbox relayer) but no consumer. `team-order`'s `PaymentConsumer.Run`
  (fetch → bounded retry → DLQ → commit only after apply or DLQ) is the in-repo model.
- `payment.events` carries only `PaymentSettled`, emitted only as `PAID` (the failure path emits nothing).
- `team-order` `CancelOrder` (and `ForceFailSaga`) claim `Cancelled` from `[Pending, Paid]` with the same
  `UpdateOrderStatusFrom` compare-and-set; the claim writes no outbox row, and no order-cancelled message exists in
  `platform-core` (`order.proto` has only `OrderPaidEvent` and `OrderShipped` as `order.events` payloads). `paid_at`
  is set exactly when an order went through `Paid`. The other `order.events` consumers (`team-notification`,
  `team-analytics`) filter on `EventEnvelope.type`.

## Goals / Non-Goals

**Goals:** a settlement credit that is never lost, never doubled and never paid for an order that did not become
paid; one refund deduction per refunded credited payment; payouts that cannot take money still inside the refund
window; every property observable through the gateway or the real stores.

**Non-Goals:** see proposal.md. Design-level: no dedupe table (the ledger's unique key is the dedupe), no hold table
or background job (held is derived), no synchronous call between `team-order` and `team-payment`.

## Decisions

### D1. Trigger the credit from `OrderPaidEvent`, not `PaymentSettled` or the RPC

`OrderPaidEvent` is written exactly once per order, atomically with the only `Pending → Paid` transition, so "seller
is owed the money" coincides with "the order became paid". This is what reconciles the credit with
`order-lifecycle-guards`: a payment that loses to a cancel never produces the event and never credits.
Alternatives: (a) consume our own `PaymentSettled` and check `GetOrder` status — races with `team-order`'s consumer
(order still `Pending` → must retry and guess), needs a cross-service call per credit, and a `Paid → Cancelled` order
would look the same as a never-paid one; (b) keep the inline credit as a fast path (old change D7) — it credits
before the order decision, so it credits cancelled orders and cannot be made correct. **The inline credit is
removed.** Consequence: the credit lags payment by two relayer polls (~1–3 s locally).

### D2. Credit computation

Per `OrderPaidEvent`: seller = the line items' `seller_id` (must be non-empty and identical across items, else
permanent); transaction = `team-payment`'s own transaction for `order_id` with status `PAID` or `REFUNDED`
(none → permanent); amount = the transaction amount (what the buyer paid; a mismatch with `total_amount` is logged,
the transaction wins). `reference_id` = payment transaction id. No call to `team-order`.

### D3. Ledger integrity (migration `0006_wallet_ledger_integrity`)

- `wallet_ledger.reference_id VARCHAR(64) NULL`; `CREATE UNIQUE INDEX ... ON wallet_ledger (type, reference_id)
  WHERE reference_id IS NOT NULL`.
- `CHECK` type/sign (validated; existing data complies: settlements positive, payouts negative, reversals
  `PAYOUT`/`REJECTED`/positive): `ORDER_SETTLEMENT → amount > 0`, `REFUND_DEDUCTION → amount < 0`,
  `PAYOUT → amount < 0 OR (status = 'REJECTED' AND amount > 0)`, `type IN (the three)`.
- `CHECK (type NOT IN ('ORDER_SETTLEMENT','REFUND_DEDUCTION') OR reference_id IS NOT NULL) NOT VALID`: legacy
  unreferenced credits stay, every new row must carry a reference.
- `payment_transactions.refunded_amount BIGINT NOT NULL DEFAULT 0`, `CHECK (refunded_amount >= 0 AND
  refunded_amount <= amount)`.
- Renumbered: the old repo's `0005_payment_outbox_seq` / `0006_wallet_ledger_integrity` become one `0006` here,
  after agora's `0005_payout_ledger_link` (kept; `ledger_entry_id` untouched). No outbox migration (D11).

### D4. Credit and deduction serialise on the payment row

Credit tx: `SELECT ... FROM payment_transactions WHERE order_id = $1 AND status IN (PAID, REFUNDED) FOR UPDATE`,
then the seller lock (same key as payouts), `INSERT ORDER_SETTLEMENT ... ON CONFLICT DO NOTHING`, and if the row is
`REFUNDED` with `refunded_amount > 0`, `INSERT REFUND_DEDUCTION(-refunded_amount) ... ON CONFLICT DO NOTHING`.
Refund tx: `UPDATE payment_transactions SET status = REFUNDED, refunded_amount = $2 ... WHERE id = $1 AND status =
PAID RETURNING` (zero rows → `ErrInvalidRefund`, so concurrent refunds have one winner), then if an
`ORDER_SETTLEMENT` with that reference exists, take its seller's lock and insert the deduction for that seller.
The row lock orders refund vs credit; the unique index makes each write once; lock order is always payment row →
seller lock, and payouts take only the seller lock, so no deadlock. Result: credit-then-refund and refund-then-credit
both end with one credit and one deduction; refund of a never-credited payment writes no deduction, and nothing
credits it later because no `OrderPaidEvent` exists for it. The refund needs no `team-order` call for the deduction
(the seller is the credit row's seller). Alternative rejected: deduct at refund time using `GetOrder`'s seller even
without a credit — drives a never-credited seller negative.

### D5. Consumer transport and error classes

`internal/consumer.SettlementConsumer` mirrors `team-order`'s `PaymentConsumer.Run`: fetch, apply with up to 5
attempts and linear backoff (200 ms × attempt), then DLQ; commit only after apply or a successful DLQ produce; if the
DLQ produce fails the same record is retried and nothing later on the partition is committed. Handle
`platform.order.v1.OrderPaidEvent` (credit) and `platform.order.v1.OrderCancelled` (D12); ignore every other type
(e.g. `OrderShipped`). Permanent (straight to DLQ): malformed
envelope/payload, missing `event_id`/`order_id`, no or mixed seller, no `PAID`/`REFUNDED` transaction for the order,
non-positive amount. Transient (retry, then DLQ): DB errors. Redelivery is a no-op through the unique index; no
dedupe table.

### D6. Consumer configuration

Bootstrap env (next to the relayer's `kafkaConfigFromEnv`): `ORDER_EVENTS_TOPIC` (default `order.events`),
`PAYMENT_SETTLEMENT_CONSUMER_ENABLED` (default `true`), `PAYMENT_SETTLEMENT_CONSUMER_GROUP` (default
`team-payment.settlement`), `PAYMENT_SETTLEMENT_DLQ_TOPIC` (default `order.events.payment-settlement.dlq`; not
`order.events.dlq`, which is `team-notification`'s). Runs only with `KAFKA_ENABLED=true` and the DB enabled. The
env-drift test is extended to the bootstrap keys.

### D7. Hold-back computation

`held = Σ over the seller's ORDER_SETTLEMENT rows with status COMPLETED and created_at > now − window of
max(0, credit + Σ REFUND_DEDUCTION with the same reference_id)`; `withdrawable = max(0, balance − held)`. A refund of
a held sale consumes the held amount it was reserved for (not free money); a refund of an older sale lowers balance
and so withdrawable; `PAYOUT` rows never lower `held`. Computed inside `AppendDebit`'s seller-locked transaction
(the signature gains a hold `{Window, Now}`), so concurrent payouts serialise and the second sees the first's debit.
The window starts at the credit's `created_at` (consumption time, a few seconds after `paid_at`). The in-memory
ledger uses the same pure function. A window of 0 skips the query.

### D8. Errors and visibility

Above balance: `FAILED_PRECONDITION` `insufficient wallet balance` (unchanged). Within balance but above
withdrawable: `FAILED_PRECONDITION` `amount is held until <RFC3339 UTC> (refund window)`, the instant when enough
held credits (oldest first) expire to cover the amount; no amounts in the message. RFC3339 instead of the old
`YYYY-MM-DD` because a seconds-scale e2e window needs a meaningful instant. `GetWalletBalance`, `GetSellerWallet`
and `ListLedgerEntries` are unchanged (balance = ledger sum).

### D9. Hold configuration

`PAYOUT_HOLD_DAYS` int in `config.Settings` (`Payout` group), default 7, valid 0–3650; `PAYOUT_HOLD_WINDOW` optional
Go duration, when non-empty it overrides the days and must be 0 ≤ d ≤ 3650 days. Any invalid value fails startup
naming the key (money config fails closed, unlike the reservation TTL's fallback). The override exists because the
overlay needs seconds (`20s`), following the `RESERVATION_TTL` precedent, while operators keep a days knob. The
effective window is logged at startup.

### D10. First deploy and legacy credits

Legacy credits have no reference, so re-consuming old `OrderPaidEvent`s would credit those orders again. The new
group therefore starts at the **latest** offset (`kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd())`). Orders paid by
the old binary were already credited inline; the deploy window (old binary credited inline, its `OrderPaidEvent`
consumed after the group exists) is seconds wide and only on the first deploy. Local and e2e stacks are rebuilt. A paid order credited by the old binary (unreferenced) and cancelled after the deploy is refunded to the
buyer but its seller is not deducted (no referenced credit to follow, D4); same window, documented.

### D11. Outbox ordering not ported

Verified: `ClaimPending` orders by `(available_at, created_at)` and skips locked rows, and a failed row is re-gated by
backoff while later rows of the same order remain claimable, so overtaking is possible. It is not observable today
(proposal Non-goals), so the old `seq` migration, `ReleaseClaims` and stop-at-first-failure are not ported; a
follow-up must port them before a second event type joins `payment.events`.

### D12. Automatic refund when a `Paid` order is cancelled

Contract (lands first, additive): in `platform-core/packages/proto/platform/order/v1/order.proto`

```proto
// OrderCancelled is emitted (via the outbox, to order.events, key = order_id) in the same
// transaction as the compare-and-set claim that moves an order to CANCELLED.
message OrderCancelled {
  string order_id = 1;
  string buyer_id = 2;
  string seller_id = 3;
  OrderStatus previous_status = 4;  // ORDER_STATUS_PENDING | ORDER_STATUS_PAID
  int64 total_amount = 5;
  string currency = 6;
  google.protobuf.Timestamp cancelled_at = 7;
}
```

`team-order`: when `UpdateOrderStatusFrom` wins a claim with `to == Cancelled`, it enqueues the row in the same
transaction (a `WithCancelledOutbox` builder next to `WithPaidOutbox`), with a stable
`event_id = SHA1-UUID("agora/team-order/order.events/OrderCancelled", order_id)` (an order is cancelled at most once)
and `previous_status = PAID` iff `paid_at` is set. A lost claim writes nothing. The existing relayer publishes it.

`team-payment`: the D5 consumer also handles `platform.order.v1.OrderCancelled`. `previous_status != PAID` → ignore.
Otherwise load the order's transaction: `PAID` → run the D4 refund tx as the system with the full transaction amount
(reason `order_cancelled`); `REFUNDED` → no-op (a seller or admin refund already happened; its deduction stands, no
second one); no `PAID`/`REFUNDED` transaction → permanent, DLQ. Because the refund tx and the credit tx serialise on
the payment row and both deduction writes are unique per reference, the result is one credit and one deduction whether
`OrderPaidEvent` or `OrderCancelled` is consumed first (both are keyed by order id but `team-order`'s relayer may
reorder them after a produce failure), and redelivery of either is a no-op.

Alternatives rejected: `team-order` calling `RefundPayment` synchronously in `CancelOrder` (couples the cancel's
success to `team-payment`, needs a retry store for failures, and a buyer principal cannot refund); `team-payment`
polling `GetOrder` (no trigger, cross-service reads per payment).

## Risks / Trade-offs

- [Credit now depends on Kafka and both relayers] → it already did for the order becoming `Paid`; a stalled consumer
  shows as lag on `team-payment.settlement`; a DLQ'd record is replayed by re-producing it (idempotent).
- [Paid-then-cancelled order] → credited on `OrderPaidEvent`, then refunded in full and deducted on `OrderCancelled`
  (D12); a held sale's refund consumes its held amount, so a cancel inside the window never touches free money.
- [`OrderCancelled` before `OrderPaidEvent` on the wire] → D4 makes the end state identical; the credit and its
  deduction are written together when the credit arrives second.
- [Proto addition] → additive message only; `buf breaking` (FILE) passes; consumers that do not re-vendor ignore the
  unknown envelope type.
- [Pre-existing: `ProcessMockPayment`'s status write is not a compare-and-set, so a racing failure call can overwrite
  `PAID` with `FAILED`] → the credit then goes to the DLQ (no `PAID`/`REFUNDED` transaction) instead of being written;
  visible, not silent. Fixing the settle CAS is a separate change.
- [Deploy window double credit (D10)] → seconds, first deploy only, local mock money; documented in README.
- [Hold query cost] → bounded by a seller's credits in one window; served by `idx_wallet_ledger_seller_created`.
- [Existing e2e payout scenarios break under any hold] → the e2e overlay sets 20 s and the bank-payout step waits it
  out; without the overlay the wait guard fails naming the overlay.

## Migration Plan

1. Apply `0006` (online: nullable column, partial unique index, validated sign CHECK, `NOT VALID` reference CHECK).
   Before applying in a long-lived DB run `SELECT type, count(*) FROM wallet_ledger WHERE NOT (<sign rule>)` and
   expect zero.
2. Merge the proto, re-vendor `team-order` and `team-payment`. Deploy `team-payment` first (consumer starts at
   latest and already understands `OrderCancelled`), then `team-order` (starts emitting it); the reverse order would
   publish cancel facts before a consumer group exists, and those cancels would not be refunded.
3. Rollback: `team-order` can roll back alone (it stops emitting `OrderCancelled`; cancels of paid orders are then
   not refunded, as today). `team-payment`: redeploy the previous binary (it ignores the new column; its inline credit
   returns) and run `0006` down (drops index, CHECKs, columns).

