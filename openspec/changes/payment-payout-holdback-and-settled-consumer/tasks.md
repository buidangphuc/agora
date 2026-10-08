> Single repo (`team-payment`). Go checks on the host (`gofmt -l`, `go vet`, `go test -race`); Postgres-backed tests need
> `TEST_DATABASE_URL` (throwaway `postgres:16-alpine` on a free high port, removed afterwards).

## 1. team-payment: hold-back

- [x] 1.1 `PAYOUT_HOLD_DAYS` in config (default 7, 0..3650), `.env.example`, README; verify config tests
- [x] 1.2 `Holdback`, `ErrFundsOnHold`/`FundsOnHoldError`, pure `computeWithdrawal` / `checkWithdrawable`; `DebitIfSufficient(…, Holdback)` and `Withdrawable` on the Postgres and in-memory ledgers; verify unit + Postgres tests (within window, after window, refund inside/outside window, concurrency, 0 days)
- [x] 1.3 Service `WithPayoutHold` / `WithClock`, handler error mapping to the two `FAILED_PRECONDITION` messages; verify service and handler tests

## 2. team-payment: settlement consumer

- [x] 2.1 `PaymentService.CreditSettlement` (shared with the inline path); verify tests
- [x] 2.2 `internal/consumer.SettlementConsumer` with retry/backoff/DLQ/commit discipline; verify redelivery, missing order -> retry then DLQ, seller resolved -> credit once, malformed -> DLQ
- [x] 2.3 franz-go reader/DLQ producer in `internal/bootstrap`, wired from `main` when `KAFKA_ENABLED` and the DB are on; env keys documented; verify env-drift test covers the Kafka keys

## 3. Docs

- [x] 3.1 README (payout hold-back, refund interplay, consumer, env table, e2e note)

## 4. Gate

- [x] 4.1 `gofmt -l`, `go vet ./...`, `go test -race ./...` with and without `TEST_DATABASE_URL`; `openspec validate payment-payout-holdback-and-settled-consumer --strict`

## 5. Follow-ups outside this change (not edited here)

- [x] 5.1 `platform-e2e`: two scenarios credit a seller by settling a payment and withdraw immediately, which the default 7-day hold refuses: `features/fintech/seller_payout.feature` "Seller requests a bank payout from wallet balance" (steps in `step_definitions/group_c_steps.py`) and `features/security/payment_access.feature` "A seller reads and withdraws from their own wallet" (steps in `step_definitions/wave2_access_steps.py`, `seller_withdraws` / `withdrawals_succeed`). Run the e2e stack's `team-payment` with `PAYOUT_HOLD_DAYS=0`, and add one hold-back scenario (fresh proceeds -> `FAILED_PRECONDITION` "held until") that needs the default or an explicit `PAYOUT_HOLD_DAYS` > 0 stack. The "never credited" payout scenario and the denied-payout scenarios are unaffected. (evidence: platform-e2e@a38643e payout Given step credits via settled payment, payment_access "A seller reads and withdraws from their own wallet" uses a paid-order-funded wallet; root docker-compose.services.yaml PAYOUT_HOLD_DAYS=0; platform-e2e@6760b73 + team-payment@db9114a hold-back scenario "Fresh sale proceeds are held back from payout", skipped unless E2E_PAYOUT_HOLD_DAYS>0)
- [x] 5.2 Root compose: add `PAYOUT_HOLD_DAYS`, `PAYMENT_SETTLEMENT_CONSUMER_GROUP`, `PAYMENT_SETTLEMENT_DLQ_TOPIC` to `team-payment`, create the DLQ topic in `redpanda-init` (evidence: root docker-compose.services.yaml PAYOUT_HOLD_DAYS/PAYMENT_SETTLEMENT_CONSUMER_GROUP/PAYMENT_SETTLEMENT_DLQ_TOPIC on team-payment, payment.events.settlement.dlq created in redpanda-init; uncommitted in root working tree)
