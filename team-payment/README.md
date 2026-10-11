# team-payment

Go gRPC service for the **payment** bounded context of the Agora marketplace. It owns mock
payment transactions, refunds, seller wallets and payouts, and publishes `PaymentSettled` via a
transactional outbox. Deployed in the root compose as `team-payment` (container
`team-payment-svc`, gRPC `:50056`).

**All money is MOCK** (root `AGENTS.md` §7, financial-safety policy): no real payment rail, bank
or card is ever contacted. `method` values (COD, MoMo, bank, card) are labels only.

## Contract

Proto: `proto/platform/payment/v1/payment.proto`, service `platform.payment.v1.PaymentService`
(10 RPCs). Plus `grpc.health.v1.Health`, and gRPC reflection when `GRPC_REFLECTION_ENABLED=true`.

The service does not verify tokens. The gateway verifies the JWT (ADR-0006) and forwards
`x-principal-id`, `x-principal-type` (`user`/`service`/`anonymous`) and `x-principal-scopes`
(comma separated); `internal/interceptor/auth.go` trusts them. `RequirePrincipal` rejects a missing
principal, id `anonymous` and type ANONYMOUS with `Unauthenticated`.

| RPC | Authorization (as implemented) | Notes |
|---|---|---|
| `CreatePayment` | Authenticated; caller must be the order's buyer as reported by team-order (`PermissionDenied` otherwise) | Calls `team-order.GetOrder`; order must be `PENDING`, else `FailedPrecondition`; unknown order gives `NotFound`. Returns the existing transaction if one exists for the order. `payment_url` is `/checkout/pay/<order_id>`. |
| `GetPayment` | Authenticated; the transaction's buyer (user principal), a principal with scope `admin`, or the order's seller (user principal, resolved via `team-order.GetOrder` only for callers who are neither); else `PermissionDenied`. A failed seller lookup is `NotFound`/`Internal`, never success | By `id` or `order_id`. Carries `refunded_amount` and `refunds` (oldest first). |
| `ProcessMockPayment` | Authenticated; the transaction's buyer or an admin (`requireBuyerOrAdmin`, no seller read) | `simulate_success=true` settles to PAID, `false` sets FAILED. Already PAID returns success without re-settling. Writes no ledger entry (the seller is credited from `OrderPaidEvent`, see Events). |
| `RefundPayment` | Authenticated; the order's seller (user principal, seller id resolved via `team-order.GetOrder`) or a principal with scope `admin`, else `PermissionDenied` | `refund_id` is required (1-64 of `[A-Za-z0-9._:-]`, else `InvalidArgument`) and is the idempotency key, stored as `rpc:<refund_id>`. `amount` > 0 (`InvalidArgument`) and at most the refundable remainder (`FailedPrecondition` `refund amount exceeds the refundable remainder`); a `REFUNDED` or unpaid payment is `FailedPrecondition`; the same id with another payment or amount is `AlreadyExists`; the same id and amount again succeeds and writes nothing. See Refund model. Never blocked by balance or hold (the balance may go negative). `payment_id` may also be an order id. |
| `GetSellerWallet` | `sellerAccess`, admin read allowed | Balance = `SUM(wallet_ledger.amount)`, currency VND. Same data as `GetWalletBalance`. |
| `RequestPayout` | `sellerAccess`, owner only | Requires `bank_code`, `account_number`, `account_name`. Debits the ledger through the same atomic step as `RequestWalletPayout` (same refusals; a refused payout records nothing), then stores the bank details in `payout_requests` linked by `ledger_entry_id`. |
| `ListPayoutHistory` | `sellerAccess`, admin read allowed | Reads `payout_requests`. |
| `GetWalletBalance` | `sellerAccess`, admin read allowed | Ledger model: `SUM(wallet_ledger.amount)`. |
| `ListLedgerEntries` | `sellerAccess`, admin read allowed | Ledger model, newest first, opaque offset cursor, page size default 20 and max 100. `reference_id`: the payment id for an `ORDER_SETTLEMENT`, the refund id (`rpc:`/`return:`/`cancel:`/`legacy:` key) for a `REFUND_DEDUCTION`, empty for a payout. |
| `RequestWalletPayout` | `sellerAccess`, owner only | Ledger model: appends a negative PENDING `PAYOUT` row. Balance check, hold check and debit are one atomic step (Postgres: per-seller `pg_advisory_xact_lock` in one transaction; in-memory: mutex). Above the balance: `FailedPrecondition` `insufficient wallet balance`; within the balance but above the withdrawable amount: `FailedPrecondition` `amount is held until <RFC3339 UTC> (refund window)` (see Payout hold-back). |

`sellerAccess` (`internal/handler/wallet_ledger.go`) never trusts the request `seller_id` alone:

- A `user` principal may act on its own wallet; an empty `seller_id` means "my wallet".
- A principal with scope `admin` may read another seller's wallet (read RPCs only).
- Payouts are owner-only: an admin or a service principal cannot pay out. A service principal with
  an empty `seller_id` gets `InvalidArgument`; any other mismatch gets `PermissionDenied`.

**Consumes (upstream):** `team-order` `OrderService.GetOrder` at `UPSTREAM_ORDER_ADDR`, used for
(1) the `PENDING` check and amount/currency in `CreatePayment`, and (2) the seller check in
`RefundPayment` and in `GetPayment` for non-buyer callers. The settlement credit makes no call: seller and amount come from the event and
the own transaction. The
call does not forward the caller's principal: `internal/upstream/order.go` sends the service principal
`service-team-payment` (type `service`, scope `order.read`), which team-order's `GetOrder` requires.

## Events

| Direction | Topic | Type | Key | Notes |
|---|---|---|---|---|
| Produces | `payment.events` | `platform.payment.v1.PaymentSettled` in an `EventEnvelope` | `order_id` | Emitted only when a payment is settled PAID. A failed or refunded payment emits nothing. |
| Consumes | `order.events` | `platform.order.v1.OrderPaidEvent`, `platform.order.v1.OrderCancelled`, `platform.order.v1.ReturnRefunded` (others ignored) | `order_id` | Group `team-payment.settlement`; poison records go to `order.events.payment-settlement.dlq`. Credits the seller; refunds the remainder of a cancelled paid order; refunds an RMA return. |

- `event_id` (the outbox primary key) is the envelope `event_id`. Delivery is at-least-once, so
  consumers must dedupe on it. The consumer is `team-order`, which parks poison records on
  `payment.events.dlq` (owned by team-order, not this service). See ADR-0009.
- The outbox row is written in the same DB transaction as `status = PAID` (`PgTxWriter.SettleTx`).
- Relayer (`internal/events/relayer.go`): polls every 1s, claims up to 100 rows with
  `FOR UPDATE SKIP LOCKED`, exponential backoff 1s doubling to 5m, parks a row as `failed` after 10
  attempts. These relayer values are code defaults and not configurable via env.
- The relayer runs only if `KAFKA_ENABLED=true` and the DB is enabled. Otherwise outbox rows are
  recorded but never relayed.
- Without a DB, settle degrades to a status-only update and emits no event (a warning is logged).

### Seller settlement credit (`internal/consumer`)

- `team-order` writes `OrderPaidEvent` once, in the same transaction as the only `Pending -> Paid`
  compare-and-set. For each one the consumer appends one `ORDER_SETTLEMENT` (`COMPLETED`) for the
  line items' single `seller_id`, of the amount of this service's own `PAID`/`PARTIALLY_REFUNDED`/`REFUNDED` transaction
  for the order (a mismatch with `total_amount` is logged; the transaction wins), with
  `reference_id` = the transaction id. A late payment for an order team-order already cancelled
  produces no `OrderPaidEvent`, so it never credits. The credit appears a few seconds after the
  payment (two outbox hops); with Kafka off there is no credit.
- At-least-once and idempotent: the unique `(type, reference_id)` index makes redelivery and
  replays no-ops (no dedupe table). Up to 5 attempts (200 ms x attempt) for DB errors, then the
  DLQ; malformed envelopes/payloads, missing ids, no or mixed seller and an order with no
  settled (`PAID`/`PARTIALLY_REFUNDED`/`REFUNDED`) transaction go straight to the DLQ. An offset is committed only after the
  write or a successful DLQ produce; if the DLQ produce fails the same record is retried and
  nothing later on the partition is committed. A DLQ'd record is replayed by producing it again.
- Runs only with `KAFKA_ENABLED=true`, `PAYMENT_SETTLEMENT_CONSUMER_ENABLED=true` and the DB.
- **First deploy:** a new group starts at the **latest** offset, because credits written by the
  old inline binary carry no reference and re-consuming their `OrderPaidEvent`s would credit
  them again. In the seconds-wide deploy window an order credited inline by the old binary whose
  event is consumed after the group exists is credited twice; and a paid order credited by the
  old binary (unreferenced) is not deducted when it is later refunded. Local/e2e stacks are rebuilt.

### Refund model (`payment_refunds`)

A payment can be refunded several times, up to its amount. Each refund is one `payment_refunds`
row under a deterministic key, with its source, source id, requested and applied amount and
reason. `refunded_amount` is the sum of the applied amounts. Status: `PAID` (nothing refunded),
`PARTIALLY_REFUNDED` (5: `0 < refunded_amount < amount`), `REFUNDED` (4: fully refunded, or a
legacy single refund, see `0007`).

| Source | Stored key (= `PaymentRefund.id` = deduction `reference_id`) | `source_id` | Mode |
|---|---|---|---|
| `RefundPayment` (seller/admin) | `rpc:<refund_id>` | `refund_id` | strict |
| `ReturnRefunded` (RMA) | `return:<return_id>` | `return_id` | clamp |
| `OrderCancelled` from `PAID` | `cancel:<order_id>` | `order_id` | remainder |
| migration `0007` | `legacy:<payment_id>` | `payment_id` | (backfill) |

The prefixes keep a seller-chosen `refund_id` from pre-empting a return's or a cancel's refund.
RPC keys are global: reusing one on another payment is `AlreadyExists`.

`ApplyRefund` (`internal/repository/settlement.go`) runs in one transaction: lock the payment row
(`SELECT ... FOR UPDATE`, the serialisation point of every refund path; lock order payment row,
then seller lock); an existing key with the same payment and requested amount is a replay that
returns the current state (any amount for the remainder mode), otherwise `AlreadyExists`; then the
mode against `remaining = amount - refunded_amount`:

- **strict** (RPC): `PAID`/`PARTIALLY_REFUNDED` only (else `FailedPrecondition`); above
  `remaining` is refused and writes nothing.
- **clamp** (return): applies `min(requested, remaining)` and records both. On a `REFUNDED`
  payment it records an applied 0 with no deduction.
- **remainder** (cancel): applies `remaining`; writes nothing when 0 remains or the payment is
  `REFUNDED`.

It writes the refund row, `refunded_amount`, the status and, when the applied amount is positive
and the payment was credited, one `REFUND_DEDUCTION` of `-applied` referencing the refund key, for
the credited seller. Concurrent refunds never push `refunded_amount` above `amount`.

**Over-refund rule (returns).** Two caps. `team-order` refuses a return above the order's
returnable remainder at request time. At refund time the payment's remainder is authoritative,
because seller/admin refunds and cancels take from the same payment: a `ReturnRefunded` refunds
`min(refund_amount, remaining)`, records requested vs applied, and with nothing left records an
applied 0. Such a fact is never dead-lettered. The RPC stays strict because its caller can retry
with a smaller amount.

### Refund deduction

Credit and refunds serialise on the payment row, then take the seller lock payouts use. A refund
writes its `REFUND_DEDUCTION` only when a credit for that payment exists. If refunds land first,
the credit path writes one deduction per refund row with a positive applied amount together with
the credit. Either order ends with one credit and one deduction per positive refund, and a
never-credited payment is never deducted.

### Automatic refund of a cancelled paid order

`team-order` writes `OrderCancelled` (with `previous_status`) through its outbox in the same
transaction as the cancel claim. The same consumer, when `previous_status` is `PAID`, refunds
whatever is still refundable on the order's payment as one refund `cancel:<order_id>` (reason
`order_cancelled`), so after the cancel the payment reads `REFUNDED`. A `REFUNDED` payment, a
redelivered cancel (its key exists) and a cancel from `PENDING` write nothing; an order with no
settled transaction goes to the DLQ. A cancel published while `team-payment` is down is applied
when it restarts (committed offsets).

### RMA refund (`ReturnRefunded`)

`team-order` writes `ReturnRefunded` through its outbox in the same transaction as the return's
`APPROVED -> REFUNDED` compare-and-set, keyed by the order id, with the stored `refund_amount`.
The consumer applies it as one refund `return:<return_id>` (source `RETURN`, reason
`return_refunded`, clamp mode), whichever of the return and the credit arrives first, after an
outage, and once per return under redelivery; several returns of one order refund cumulatively.
A fact without `return_id` or `order_id`, with `refund_amount <= 0`, or whose order has no settled
payment is parked on `order.events.payment-settlement.dlq` and writes nothing. `team-order` never
calls this service to refund.

### Payout hold-back

Withdrawable = `max(0, balance - held)`; `held` is the unrefunded part (credit plus the
deductions of every refund of that payment, joined through `payment_refunds`, never below 0) of
every `COMPLETED` `ORDER_SETTLEMENT` created inside the window. A
refund of a held sale consumes its held amount; a refund of an older sale lowers the balance.
Computed in the payout's seller-locked transaction. The window starts at the credit's
`created_at`. `GetWalletBalance` / `GetSellerWallet` / `ListLedgerEntries` still show the full
ledger sum. The refusal message names the instant (oldest credits released first) and no amounts.

## Data

PostgreSQL database `payment_db` (compose: shared `postgres:5432`, user `payment_svc`).
Migrations in `migrations/` (golang-migrate `NNNN_name.up/down.sql`). Compose applies them with the
one-shot `team-payment-migrate` job, which the service depends on. The service does not migrate at
startup.

| Migration | Tables | Used by |
|---|---|---|
| `0001_initial` | `payment_transactions` (status 1 PENDING, 2 PAID, 3 FAILED, 4 REFUNDED; 5 PARTIALLY_REFUNDED since `0007`) | payment RPCs |
| `0002_seller_wallet` | `payout_requests` (bank details), plus legacy `seller_wallets` and `wallet_transactions` (no longer read or written) | `RequestPayout`, `ListPayoutHistory` |
| `0003_payment_outbox` | `payment_outbox_events` | settle + relayer |
| `0004_wallet_ledger` | `wallet_ledger` (single-entry signed rows: `type`, `amount`, `status`) | `GetSellerWallet`, `GetWalletBalance`, `ListLedgerEntries`, `RequestPayout`, `RequestWalletPayout`, settlement credit |
| `0005_payout_ledger_link` | `payout_requests.ledger_entry_id` (nullable) | links a payout request to its ledger debit |
| `0006_wallet_ledger_integrity` | `wallet_ledger.reference_id` + unique `(type, reference_id)`, type/sign CHECK, reference required for new settlement/refund rows (`NOT VALID`: legacy rows kept); `payment_transactions.refunded_amount` | settlement credit, refund deduction |
| `0007_cumulative_refunds` | `payment_refunds`; `wallet_ledger.reference_id` widened to 96; LEGACY backfill and re-pointed legacy deductions; CHECKs `status <> 4 OR refunded_amount = amount` (`NOT VALID`) and `status <> 5 OR 0 < refunded_amount < amount` | refunds, refund deduction, hold-back |

The **wallet ledger is the single source of truth** for seller money:

- **Ledger** (`wallet_ledger`): append-only, single-entry signed rows (not double-entry, no bank
  fields). Balance is computed as `SUM(amount)`. Entry types: `ORDER_SETTLEMENT`, `PAYOUT`,
  `REFUND_DEDUCTION`; statuses `PENDING`, `COMPLETED`, `REJECTED`.
- **`payout_requests`** only records bank details and status for a payout, linked to the ledger
  debit that funded it (`ledger_entry_id`). If recording fails after the debit, a compensating
  credit is appended.
- **Legacy** `seller_wallets` and `wallet_transactions` are no longer read, written or seeded.
  The tables and their data are left in place.

The ledger store enforces: one entry per `(type, reference_id)`; `ORDER_SETTLEMENT > 0`,
`REFUND_DEDUCTION < 0`, `PAYOUT < 0` except a `REJECTED` compensating reversal `> 0`; a reference
on every new settlement/refund row. A seller has no balance until a paid order is credited.
Before applying `0006` to a long-lived DB, run the sign pre-check in the migration header and
expect zero rows.

**`0007_cumulative_refunds`.**

- *Pre-check* (in the migration header; both must return zero rows on a long-lived DB): no
  payment already has status 5, and no `REFUND_DEDUCTION` references something that is not a
  payment.
- *Up*: each `REFUNDED` payment with `refunded_amount > 0` gains one `LEGACY` refund
  `legacy:<payment_id>` of that amount and its deduction is re-pointed to it. Status, refunded
  amount and every ledger amount stay as they were, so balances and hold-back are unchanged. A
  legacy partial refund therefore stays `REFUNDED` and closed: further refunds are refused, a
  return records an applied 0 and a replayed cancel does nothing, exactly as under the old model.
  `REFUNDED` rows with `refunded_amount = 0` (refunded before `0006`) are left untouched.
- *Down* (rollback): `PARTIALLY_REFUNDED` becomes `REFUNDED`, which closes those payments to the
  old code; legacy deductions reference the payment id again; `payment_refunds` and the CHECKs are
  dropped; `reference_id` narrows to 64 only if no value is longer. Deductions written by new
  refunds keep their `rpc:`/`return:`/`cancel:` references, which the old hold-back does not net:
  it holds more than necessary, never less.
- *Deploy order*: `team-payment` (pre-check, `0007`, new binary) before `team-gateway` and
  `team-order`; `team-order` must not emit `ReturnRefunded` to an old `team-payment`, which would
  ignore and commit it (replay by resetting the group offset; facts are idempotent). Rollback runs
  in reverse: `team-order` first, then the down migration. The new `RefundPayment` requires
  `refund_id`.

## Configuration

Read in `internal/config/config.go` (declared env keys) and `internal/bootstrap/kafka.go` (Kafka,
read directly from the environment, not part of `Settings`).

| Variable | Default | Purpose |
|---|---|---|
| `ENV` | `local` | Environment name (`prod`/`production` count as prod) |
| `LOG_LEVEL` | `info` | Only `debug` changes behaviour; anything else is info |
| `LOG_JSON` | `true` | JSON logs, else text |
| `GRPC_HOST` | `0.0.0.0` | Bind host |
| `GRPC_PORT` | `50056` | Bind port (1-65535) |
| `GRPC_REFLECTION_ENABLED` | `true` | gRPC reflection |
| `SHUTDOWN_GRACE_SECONDS` | `10` | Graceful-stop timeout before forced stop |
| `DATABASE_ENABLED` | `true` | `false` switches to in-memory repositories |
| `DATABASE_URL` | empty | Required when `DATABASE_ENABLED=true` (startup fails otherwise) |
| `DB_MAX_CONNS` | `10` | pgx pool size |
| `UPSTREAM_ORDER_ADDR` | `localhost:50055` | team-order gRPC address (insecure channel) |
| `MOCK_PAYMENTS` | `false` | Enables `ProcessMockPayment`; startup is refused when true and `ENV` is staging/stage/prod/production |
| `OTEL_ENABLED` | `false` | Loaded but unused; see Known gaps |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Loaded but unused; see Known gaps |
| `OTEL_SERVICE_NAME` | `team-payment` | Loaded but unused; see Known gaps |
| `PAYOUT_HOLD_DAYS` | `7` | Payout hold window in days, 0-3650; `0` disables the hold. Invalid refuses startup |
| `PAYOUT_HOLD_WINDOW` | empty | Optional Go duration (0 to 3650 days, e.g. `20s`) overriding `PAYOUT_HOLD_DAYS`; invalid refuses startup. The effective window is logged at startup |
| `KAFKA_ENABLED` | `false` | Starts the outbox relayer and (with the DB) the settlement consumer |
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated brokers |
| `PAYMENT_EVENTS_TOPIC` | `payment.events` | Topic the relayer produces to |
| `ORDER_EVENTS_TOPIC` | `order.events` | Topic the settlement consumer reads |
| `PAYMENT_SETTLEMENT_CONSUMER_ENABLED` | `true` | Turns the settlement consumer off when `false` |
| `PAYMENT_SETTLEMENT_CONSUMER_GROUP` | `team-payment.settlement` | Consumer group (new group starts at the latest offset) |
| `PAYMENT_SETTLEMENT_DLQ_TOPIC` | `order.events.payment-settlement.dlq` | Dead-letter topic (not `order.events.dlq`, which is team-notification's) |

`.env.example` documents every key; `internal/bootstrap/env_drift_test.go` fails when a
`Settings` key or a Kafka key is missing from it. The `.env.example` `DATABASE_URL` uses port
`5438`, a standalone Postgres of your own, not the compose one.

## Run locally

Root compose (from the repo root; `docker-compose.yaml` includes `docker-compose.services.yaml`):

```bash
docker compose up -d --build team-payment
```

This brings up Postgres, Redpanda (`redpanda-init` creates `payment.events` and
`order.events.payment-settlement.dlq`), the
`team-payment-migrate` job and the service. Compose sets `UPSTREAM_ORDER_ADDR=team-order-svc:50055`,
so `CreatePayment` and the wallet credit need `team-order` running. Compose leaves the hold at
its 7-day default; the e2e overlay `platform-e2e/compose/payment-ledger.override.yaml` sets
`PAYOUT_HOLD_WINDOW=20s` so payout scenarios can wait the hold out:

```bash
docker compose -f docker-compose.yaml -f platform-e2e/compose/payment-ledger.override.yaml up -d team-payment
```

Standalone (needs your own Postgres with migrations applied, and `generated/` present):

```bash
cd team-payment
cp .env.example .env     # the service does not load .env itself; export the variables
go run ./cmd/server
```

Smoke test (grpcurl, reflection on). The gateway normally sets the principal headers; set them by
hand here. Wallet calls need `x-principal-type: user`:

```bash
grpcurl -plaintext -H 'x-principal-id: buyer_1' -H 'x-principal-type: user' \
  -d '{"order_id":"<pending order id>","method":4}' \
  localhost:50056 platform.payment.v1.PaymentService/CreatePayment

grpcurl -plaintext -d '{"transaction_id":"<tx id>","simulate_success":true}' \
  localhost:50056 platform.payment.v1.PaymentService/ProcessMockPayment

grpcurl -plaintext -H 'x-principal-id: seller_1' -H 'x-principal-type: user' \
  -d '{}' localhost:50056 platform.payment.v1.PaymentService/GetWalletBalance

grpcurl -plaintext localhost:50056 grpc.health.v1.Health/Check
```

## Build, test and lint

There is no Makefile and no CI workflow inside this repository. Run the equivalent by hand:

```bash
cd team-payment
buf generate            # only if generated/ is missing or proto/ changed
gofmt -l .              # must print nothing
go vet ./...
go build ./cmd/server
go test -race ./...
# Postgres-backed tests (fresh schema per test, all migrations applied); skipped without it:
TEST_DATABASE_URL=postgres://postgres:pg@127.0.0.1:<port>/postgres?sslmode=disable go test -race ./...
```

Docker build: `Dockerfile` (Go 1.22, static binary, distroless nonroot, exposes 50056). It builds
from `COPY . .`, so `generated/` must exist in the build context.

## Spec and verification

- Feature manifest: `FEATURES.yaml` (`payment.mock-pay`, `payment.seller-wallet`, `payment.payout`,
  `payment.refund`, and four wallet-access authz features), each with a `covered_by` pointing at a
  scenario in `platform-e2e`.
- Verify e2e coverage with `make -C platform-e2e features-check`, and for an OpenSpec change with
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Behaviour changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC; do
  not change behaviour without a spec delta and matching e2e.

## Gotchas

- `proto/` is vendored from `platform-core` (all platform protos, not just payment). Never edit it
  here; change `platform-core` and re-sync (ADR-0001).
- `generated/` is gitignored. Regenerate with `buf generate` (`buf.gen.yaml` uses remote plugins,
  so it needs network access).
- The seller credit lags the payment (outbox -> team-order -> outbox -> consumer). With the
  default 7-day hold a seller cannot withdraw a sale right after it settles.
- `ProcessMockPayment`'s status write is not a compare-and-set: a racing failure call can
  overwrite `PAID` with `FAILED`, and that order's credit then goes to the DLQ (no settled
  transaction).
- With `DATABASE_ENABLED=false` everything is in memory and lost on restart, and there is no
  outbox, so no events.
- `coverage.out` and `coverage.summary` in the directory are local test artifacts.

## Known gaps

- `RefundPayment` emits no event.
- `ProcessMockPayment` re-settles any payment that is not `PAID`, including a refunded one.
- `payment.events` per-order ordering is not guaranteed (a backed-off outbox row can be overtaken);
  harmless while it carries only `PaymentSettled`, but it must be fixed before a second event
  type is added.
- Ledger payouts stay `PENDING`; nothing transitions them.
- `OTEL_*` settings are loaded but nothing in `cmd/` or `internal/bootstrap` reads them, so no
  trace exporter is initialised; only the otelgrpc stats handler and request-id interceptors are
  wired.
- The outbox payload carries ids and status only, not the amount.

## Links

- Root rules: `AGENTS.md` (§3 boundaries, §7 mock financial policy).
- ADRs (`platform-core/docs/ADR/`): 0001 proto distribution, 0002 async broker, 0006 RS256 JWKS auth,
  0009 payment-order event integration, 0010 service zero-trust.
