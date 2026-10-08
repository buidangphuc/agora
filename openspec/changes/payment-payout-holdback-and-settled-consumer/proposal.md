## Why

Two follow-ups to `payment-wallet-ledger-single-source`, both decided by the user.

1. **Refund exposure.** If the platform refunds a buyer after the seller already withdrew everything, the platform
   pays the refund out of its own pocket (the seller's balance goes negative, D5 of the previous change). The user
   wants a mechanism that stops sellers from withdrawing everything, computes what may be withdrawn, and keeps the
   refund-covering money in the app.
2. **Best-effort credit.** The settlement credit is written inline by `ProcessMockPayment` and is only healed when
   that RPC is called again. If `team-order` is down at settle time the seller is not credited until someone
   retries by hand. `team-payment` already emits `PaymentSettled` on `payment.events` through its outbox, so the
   credit can be driven by a durable at-least-once consumer with retry and a dead-letter topic.

## What Changes

- **team-payment** (capability `seller-payout-holdback`): the ledger sum stays the wallet balance (truth), but the
  amount a seller may WITHDRAW is `balance - held`, where `held` is the unrefunded part of every `ORDER_SETTLEMENT`
  credit still inside the hold window. Config `PAYOUT_HOLD_DAYS` (default 7, integer 0..3650, 0 = no hold).
  `RequestPayout` / `RequestWalletPayout` check `amount <= withdrawable` inside the existing per-seller advisory-locked
  transaction. Still `FAILED_PRECONDITION`, with two messages: `insufficient wallet balance` (amount > balance) and
  `amount is held until <date> (refund window)` (amount <= balance but > withdrawable). Refunds are never blocked.
- **team-payment** (capability `payment-settlement-consumer`): a franz-go consumer group on `payment.events` credits
  the seller's ledger for each `PaymentSettled` (PAID), idempotently on `(ORDER_SETTLEMENT, payment id)`, resolving the
  seller via `team-order.GetOrder` as service principal `service-team-payment` (`order.read`). Bounded retry with
  backoff, then a DLQ topic; offsets are committed only after apply or DLQ. Runs only when `KAFKA_ENABLED=true` and
  the database is enabled. The inline credit stays as an idempotent fast path (same key).
- New env: `PAYOUT_HOLD_DAYS`, `PAYMENT_SETTLEMENT_CONSUMER_ENABLED`, `PAYMENT_SETTLEMENT_CONSUMER_GROUP`,
  `PAYMENT_SETTLEMENT_DLQ_TOPIC`. README and `.env.example` updated.
- **platform-core**: no proto change. `GetSellerWallet` / `GetWalletBalance` keep returning the ledger balance; the
  existing messages have no available/pending field, so the hold is visible to a seller only through the payout
  error message.

## Non-goals

- No proto/contract, migration or gateway/frontend change (a wallet "available" field is a follow-up needing a proto change).
- No per-order hold lifecycle table: the hold is derived from ledger rows and the clock.
- No real payout processing; payouts stay `PENDING` mock entries.
- No `platform-e2e` edits in this change; the scenarios that must change are listed in `tasks.md` section 5.

## Impact

- Repo: `team-payment` only (rules 3 and 4 respected: own DB, contract untouched).
- **Behaviour change**: with the default `PAYOUT_HOLD_DAYS=7` a seller cannot withdraw a sale the moment it settles.
  E2E and local flows that credit then immediately pay out must set `PAYOUT_HOLD_DAYS=0` on `team-payment`.
- A refund can still drive a balance negative, but only for proceeds that were already past the window and paid out.
