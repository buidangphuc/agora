## Context

`team-payment` balance = `SUM(wallet_ledger.amount)`; payouts debit it under a per-seller advisory lock
(`DebitIfSufficient`). Settlement credit and refund deduction are keyed on the payment transaction id (unique
`(type, reference_id)`, migration 0006). The outbox emits `PaymentSettled` (ids only, no amount) to `payment.events`;
`team-order` and `team-notification` already consume Kafka with franz-go, manual commit, in-process retry and a DLQ.

## Decisions

### D1. Balance stays the truth; a derived "withdrawable" gates payouts

`withdrawable = max(0, balance - held)`. Nothing is stored; `held` is computed from the ledger and a clock inside the
payout transaction, so there is no extra table, migration or background job and no state to drift.

### D2. What is held

`held = sum over ORDER_SETTLEMENT entries with status COMPLETED and created_at > now - PAYOUT_HOLD_DAYS of
max(0, credit + refunds)`, where `refunds` is the sum of `REFUND_DEDUCTION` rows with the same `reference_id` (both
are keyed on the payment transaction id). A refund of a held sale therefore consumes the held money it was reserved
for instead of also eating into free money (otherwise a refunded in-window sale would be penalised twice: the
balance drops AND the sale still counts as held). A refund of a sale past the window (or of a sale whose credit was
already paid out) reduces `balance` and so reduces withdrawable immediately; it is never blocked. Net effect: refunds
always apply, and they hit the money the platform is holding first. PAYOUT entries never reduce `held`: a payout can
only take withdrawable money, so held money is never paid out.

### D3. Check inside the existing advisory-locked transaction

`DebitIfSufficient` takes a `Holdback{Window, Now}`. After the lock it reads the balance (insufficient ->
`ErrInsufficientBalance`, as before), then, if the window is non-zero, reads the in-window credits with their matching
refunds, computes `held`, and rejects with `*FundsOnHoldError` (matches `ErrFundsOnHold`) when `amount > balance - held`.
Two concurrent payouts serialise on the lock and the second sees the first's PAYOUT debit in `balance`, so they cannot
overdraw withdrawable. The in-memory repository does the same under its mutex via the same pure function.

### D4. Error shape and leakage

Both are `FAILED_PRECONDITION`. `insufficient wallet balance` is unchanged. Hold: `amount is held until <YYYY-MM-DD>
(refund window)`. The date is when enough credits expire to cover the request (credits released oldest first); it is
at most the newest credit's `created_at + window`. The message carries no amounts, so it leaks nothing beyond "some
of your money is held", which is less than the seller's own ledger already shows.

### D5. Config

`PAYOUT_HOLD_DAYS` in `config.Settings` (`Payout` group), default 7, parsed as int, validated `0 <= n <= 3650`
(startup error otherwise). 0 restores exactly the previous behaviour (no hold query is even run). Default 7 is a
conventional marketplace refund/return window; the project has no stricter return period to align with.
The service gets `WithPayoutHold(days)` and `WithClock(func() time.Time)` (tests inject the clock).

### D6. Visibility

The proto `SellerWallet` / `GetWalletBalanceResponse` carry only `balance`. Overloading `balance` with withdrawable
would make the two RPCs disagree with `ListLedgerEntries` and break the "ledger is the balance" invariant, so they stay
as is and the hold surfaces only through the payout error. A service method `PaymentService.Withdrawable` exists for a
future proto field. Documented in README.

### D7. Settlement credit via consumer; inline fast path kept

New `internal/consumer.SettlementConsumer`, modelled on `team-order`'s payment consumer. Per record: decode
`EventEnvelope`, ignore other types, require `event_id`, decode `PaymentSettled`, ignore non-PAID, then call
`PaymentService.CreditSettlement(ctx, payment_id)` which loads the transaction (amount is not in the event), resolves
the seller through `team-order.GetOrder` (`upstream.AsService`) and `AppendEntryOnce(ORDER_SETTLEMENT, tx.ID)`.
No separate dedupe table: the unique `(type, reference_id)` index already makes redelivery a no-op, and the credit is the
only effect.

Error classes: malformed envelope/payload, missing ids, an unknown transaction, a transaction that is not PAID/REFUNDED
and a non-positive amount are permanent (straight to DLQ). Everything else, including a missing order and an order with
an empty seller id (both surface as `ErrOrderNotFound`; an order can lag or be a transient fault, and a settled payment
must have an order), is retried `MaxAttempts` (5) times with linear backoff (200ms * attempt) and then parked on the DLQ
topic. A record's offset is committed only after apply or a successful DLQ produce; if the DLQ produce fails, the same
record is retried (every `FetchPause`) and nothing later on the partition is processed or committed until it succeeds, so a
broker outage stalls the consumer instead of dropping records (a stalled consumer shows up as lag).

The inline credit in `ProcessMockPayment` is KEPT as a fast path: it uses the same `(ORDER_SETTLEMENT, tx.ID)` key, so
whichever path runs first wins and the other is a no-op; it keeps the local stack with Kafka off, and the existing
"heal on retry" behaviour, working, and gives immediate credit for e2e. The consumer is the durable path (retry + DLQ,
independent of the RPC being re-called). Trade-off: the credit timestamp (start of the hold window) is whichever path
wrote first, at most seconds apart. Removing the inline path would make the credit depend on Kafka being up.

### D8. Consumer wiring and config

Kafka env stays in `internal/bootstrap` (as for the relayer). New keys: `PAYMENT_SETTLEMENT_CONSUMER_ENABLED` (default
true), `PAYMENT_SETTLEMENT_CONSUMER_GROUP` (default `team-payment.settlement`), `PAYMENT_SETTLEMENT_DLQ_TOPIC`
(default `payment.events.settlement.dlq`; deliberately not `payment.events.dlq`, which is `team-order`'s). The consumer
starts only when `KAFKA_ENABLED=true` and the database is enabled, from `main` after the service exists. A test now
checks that `.env.example` documents the bootstrap keys too (the existing drift test only covers `config.Settings`).

## Risks / Trade-offs

- Clock skew between app instances shifts the window edge by the skew; irrelevant at day granularity.
- `held` is computed with a join over the seller's in-window credits per payout; bounded by sale volume per week per
  seller and served by `idx_wallet_ledger_seller_created`.
- A DLQ'd credit needs an operator to replay it (re-produce the record, or call `ProcessMockPayment` for the
  transaction). There is no automatic DLQ reprocessor.
- DLQ outage stalls the consumer: a record whose DLQ produce fails is retried indefinitely and blocks the partition (no later commit can move the offset past it), so a long DLQ outage shows up as consumer lag rather than lost records. Alert on lag.
- Existing sellers' recent sales become non-withdrawable at deploy time (intended); old proceeds are unaffected.
