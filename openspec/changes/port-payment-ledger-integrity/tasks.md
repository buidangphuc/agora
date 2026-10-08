> **Order.** Group 1 (contract) runs first, serially and alone: the additive `OrderCancelled` message is merged in
> `platform-core` and re-vendored before anything else starts. Then two tracks run in parallel: **CODE** (group 2
> `team-payment`, group 3 `team-order`, group 4 root compose; disjoint write-sets, one agent per repo; `team-payment`
> must be deployable before `team-order` emits the new fact on a shared stack, design Migration Plan) and **E2E**
> (group 5, `platform-e2e` + `team-payment/FEATURES.yaml`, written from the specs, red first). Group 6 is the
> convergence gate. One fix = one commit. CI-equivalent checks before every commit: `platform-core`:
> `make lint-proto` + `make breaking`; `team-payment` and `team-order`: `gofmt -l .` (empty), `go vet ./...`,
> `go test -race ./...` with and without `TEST_DATABASE_URL` (throwaway `postgres:16-alpine` on a free high port,
> removed afterwards); `platform-e2e`: `make check` + `make features-check`.

## 1. Contract and docs (platform-core) — first, alone

- [ ] 1.1 Add `message OrderCancelled { order_id = 1; buyer_id = 2; seller_id = 3; OrderStatus previous_status = 4;
  total_amount = 5; currency = 6; google.protobuf.Timestamp cancelled_at = 7; }` with the emission comment (design
  D12) to `platform-core/packages/proto/platform/order/v1/order.proto`, nothing renumbered or removed; verify
  `make lint-proto` and `make breaking` pass and `git diff --stat` touches only `order.proto`
- [ ] 1.2 Re-vendor `proto/` and run `buf generate` in `team-order` and `team-payment` (the only producer and
  consumer); verify `go build ./...` passes in both and the diff is vendored proto (plus generated paths where a repo
  tracks them), no hand edits; confirm `team-notification` and `team-analytics` filter `order.events` by envelope
  type (no re-vendor needed) by reading their order consumers
- [ ] 1.3 Add an ADR-0009 addendum (`platform-core/docs/ADR/0009-payment-order-event-integration.md`): the seller
  settlement credit is driven by `OrderPaidEvent`, the automatic refund of a cancelled paid order by `OrderCancelled`
  (both on `order.events`, consumer group `team-payment.settlement`, DLQ `order.events.payment-settlement.dlq`), the
  inline credit is gone, refund deduction and hold-back live in `team-payment`'s ledger, and `payment.events` stays
  single-type (ordering follow-up, design D11); verify each statement maps to a requirement in this change's specs

## 2. team-payment (code track)

- [ ] 2.1 Add migration `0006_wallet_ledger_integrity` (+ `.down.sql`) per design D3 (reference_id, partial unique
  `(type, reference_id)`, validated type/sign CHECK, `NOT VALID` reference-required CHECK,
  `payment_transactions.refunded_amount` + CHECK); verify it applies on a fresh `payment_db` and on one seeded with
  legacy unreferenced credits and a `REJECTED` +amount reversal, the down migration reverses it, and the sign
  pre-check query returns zero rows on the local DB
- [ ] 2.2 Ledger repository: `reference_id` on `LedgerEntry`/`ledgerColumns`, `AppendEntryOnce` (insert … `ON
  CONFLICT DO NOTHING`, reports whether it wrote), the payment-row-locked `CreditSettlement` and `RefundTx`
  operations of design D4 (Postgres and in-memory); verify Postgres tests: duplicate credit is a no-op, credit then
  refund and refund then credit both end with one credit + one deduction, refund without credit writes no deduction,
  concurrent refunds have one winner, the store rejects the three invalid rows of the `seller-settlement-credit`
  store scenarios
- [ ] 2.3 Hold-back: `Payout` config group with `PAYOUT_HOLD_DAYS` / `PAYOUT_HOLD_WINDOW` (design D9, startup error
  naming the key, effective window logged), pure `held`/`withdrawable`/release-instant function (design D7/D8),
  `AppendDebit` taking the hold inside the seller-locked tx, `ErrFundsOnHold` with the release instant; verify unit
  tests for every `seller-payout-holdback` scenario with an injected clock (fresh, past window, mixed, refund of held
  sale, refund past window, 0 disables, concurrency under `-race` and against Postgres), config tests (unset → 7 days,
  `-1`, `3651`, `abc`, `soon`, override precedence)
- [ ] 2.4 Service and handler: remove `creditSellerWallet` from `ProcessMockPayment`; `RefundPayment` through the D4
  refund tx (CAS, `refunded_amount`, deduction for the credited seller), still `FAILED_PRECONDITION` on a non-`PAID`
  payment; `RequestWalletPayout` / `RequestPayout` map `ErrFundsOnHold` to `FAILED_PRECONDITION` `amount is held until
  <RFC3339> (refund window)` and keep `insufficient wallet balance`; verify service and handler tests: mock payment
  writes no ledger row, refund messages and codes, held vs insufficient messages contain no amount, `RequestPayout`
  writes no payout request when held
- [ ] 2.5 `internal/consumer.SettlementConsumer` (design D2, D5): decode `EventEnvelope`, ignore non-`OrderPaidEvent`,
  resolve seller from items and the transaction from the own DB, call `CreditSettlement`; bounded retry, DLQ,
  commit-after-apply, DLQ-produce failure blocks the partition; verify unit tests with fake reader/DLQ: credit once,
  redelivery no-op, `OrderShipped` ignored, malformed → DLQ after one attempt, no transaction → DLQ, mixed sellers →
  DLQ, DB error retried then DLQ, DLQ outage commits nothing later
- [ ] 2.6 Bootstrap and main: franz-go group reader on `ORDER_EVENTS_TOPIC` starting at the latest offset for a new
  group (design D10), DLQ producer, keys of design D6, started only with `KAFKA_ENABLED=true` and the DB, stopped
  before the pool closes; extend the env-drift test to bootstrap keys; update `.env.example` and README (credit flow,
  refund deduction, hold-back and its error, env table, first-deploy note, e2e overlay); verify `go build ./...`, the
  drift test, and a local run that logs the consumer start and the effective hold window
- [ ] 2.7 Consumer handles `OrderCancelled` (design D12): `previous_status != PAID` ignored; `PAID` transaction →
  system full refund through the D4 refund tx (reason `order_cancelled`); `REFUNDED` → no-op; no transaction → DLQ;
  verify unit + Postgres tests: cancel after credit → one deduction of the full amount, cancel before credit then
  credit → one credit + one deduction, redelivered cancel → no change, seller partial refund then cancel → one
  deduction of the partial amount, `PENDING` cancel → nothing written
- [ ] 2.8 Run `gofmt -l .`, `go vet ./...`, `go test -race ./...` with and without `TEST_DATABASE_URL`; verify all
  green and record the output in the commit message

## 3. team-order (code track)

- [ ] 3.1 Add `events.BuildCancelledOutboxRow` (envelope type `platform.order.v1.OrderCancelled`, stable
  `OrderCancelledEventID(order_id)`, `previous_status = PAID` iff `paid_at` set, key = order id) and a
  `WithCancelledOutbox` repository option; `UpdateOrderStatusFrom` enqueues it in the claim transaction only when
  `to == Cancelled` and the claim won (Postgres and in-memory); wire it in `cmd/server/main.go`; verify repository
  tests: one row per won cancel from `Pending` and from `Paid` with the right `previous_status`, none for a lost claim
  or a rolled-back tx, none for other transitions; service tests: `CancelOrder` and `ForceFailSaga` both emit through
  the claim
- [ ] 3.2 README (`order.events` now also carries `OrderCancelled`, and who consumes it); verify `gofmt -l .`,
  `go vet ./...`, `go test -race ./...` with `TEST_DATABASE_URL` are green

## 4. Root compose (code track)

- [ ] 4.1 In `docker-compose.services.yaml`: `team-payment` gets `ORDER_EVENTS_TOPIC=order.events`,
  `PAYMENT_SETTLEMENT_CONSUMER_GROUP=team-payment.settlement`,
  `PAYMENT_SETTLEMENT_DLQ_TOPIC=order.events.payment-settlement.dlq` (hold left at its 7-day default);
  `redpanda-init` creates `order.events.payment-settlement.dlq`; verify `docker compose config` shows the keys, a
  fresh `up` lists the topic in `rpk topic list`, and `rpk group describe team-payment.settlement` shows the group
  after a payment
- [ ] 4.2 Add `platform-e2e/compose/payment-ledger.override.yaml` (`team-payment: PAYOUT_HOLD_WINDOW: 20s`, service
  name only, header comment like `order-inventory.override.yaml`) and a README section next to the order/inventory
  one; verify `docker compose -f docker-compose.yaml -f platform-e2e/compose/payment-ledger.override.yaml config |
  grep PAYOUT_HOLD` shows `20s` and plain `docker compose config` does not
- [ ] 4.3 Confirm no `platform-gitops` change: `envs/services/team-payment.yaml` and the staging/prod overlays leave
  the new keys unset (7-day default; Kafka already off there, proposal Non-goals) and `team-order` needs no new key;
  verify `helm template` for `team-payment` and `team-order` is byte-identical before and after

## 5. E2E track (platform-e2e + team-payment/FEATURES.yaml)

- [ ] 5.1 Add stack helpers `tests/e2e/support/payment_ledger_stack.py` (container names from env with compose
  defaults; effective hold window read from the running `team-payment` container env with a guard that fails naming
  the overlay when the window exceeds a max wait; `docker stop/start` of `team-payment`; `rpk` produce/consume/group-seek through
  `docker exec` on the redpanda container; `psql` into `payment_db`) and flows to pay N orders for one seller, wait for
  a credit, and pay a sentinel order; verify `make check` and a smoke run that pays one order and observes its credit
- [ ] 5.2 Add `team-payment/FEATURES.yaml` entries, one per capability (`payment.settlement-credit`,
  `payment.refund-deduction`, `payment.payout-holdback`), with one `acceptance` line per spec scenario named 1:1,
  `status: planned`, notes naming the overlay and the serial lane for restart/replay scenarios; verify
  `make features-check` validates the manifest
- [ ] 5.3 `features/payment/settlement_credit.feature` + steps: the 11 `seller-settlement-credit` scenarios (restart,
  replay and malformed-record scenarios tagged for the serial lane; store scenarios via `psql`, balance re-read through
  the gateway); verify red before groups 2–3 land and green after
- [ ] 5.4 `features/payment/refund_deduction.feature` + steps: the 11 `seller-refund-deduction` scenarios (refunds as
  the order's seller and cancels as the buyer through the gateway; the stopped-`team-payment` and replay scenarios in
  the serial lane; the negative-balance scenario under the overlay); verify red before groups 2–3 land
  and green after
- [ ] 5.5 `features/payment/payout_holdback.feature` + steps: the 10 `seller-payout-holdback` scenarios (needs the
  overlay; waits are window + margin, never blind sleeps; the startup scenario runs the `team-payment` image with each
  invalid value on the stack network and asserts exit code and log); verify red before groups 2–3 land and green after
- [ ] 5.6 Update the existing bank-payout flow under the hold: `group_c_steps.py` "a seller has a positive wallet
  balance" waits until the credit has left the hold window (guarded as in 5.1) before "the seller requests a payout",
  and the `payment.payout` acceptance/notes say so; confirm `seller_wallet_access.feature` and
  `fulfillment_and_payout.feature` need no change (no payout is sent); verify those features pass with the overlay
- [ ] 5.7 Flip the three FEATURES entries to `status: automated` with `covered_by`; verify `make features-check` and
  `make check` are green

## 6. Convergence gate

- [ ] 6.1 Full suite against the stack with `-f platform-e2e/compose/order-inventory.override.yaml -f
  platform-e2e/compose/payment-ledger.override.yaml`, `-n 4`, run twice, serial lane separately; verify green both
  times and every flake root-caused
- [ ] 6.2 `make -C platform-e2e spec-check CHANGE=port-payment-ledger-integrity` and `openspec validate
  port-payment-ledger-integrity --strict`; verify both pass
- [ ] 6.3 Run `contract-boundary-reviewer` (new `order.events` consumer, no cross-DB access, no gateway logic) and
  `auth-scope-reviewer` (refund and payout paths keep their access checks) on the diff; verify no blocking finding
- [ ] 6.4 Retire the superseded `openspec/changes/payment-payout-holdback-and-settled-consumer` in its own commit;
  verify `openspec list` no longer shows it
