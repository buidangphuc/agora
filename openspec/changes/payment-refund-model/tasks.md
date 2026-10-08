> **Order.** Group 1 (the contract) runs first, serially and alone. The additive proto commit lands in `platform-core`
> and every vendored copy is re-vendored before anything else starts. Then two tracks run in parallel:
>
> - **CODE**, one agent per repo with disjoint write-sets: group 2 `team-payment`, group 3 `team-order`, group 4
>   `team-gateway`, group 5 `team-frontend`, group 6 compose/gitops (confirm no change).
> - **E2E**, group 7: `platform-e2e` plus the `FEATURES.yaml` of `team-payment`, `team-order` and `team-frontend`.
>   It is written from the specs and is red first.
>
> On a shared stack `team-payment` must be deployed before `team-order` emits `ReturnRefunded` (design, Migration
> Plan). Group 8 is the convergence gate. One fix = one commit.
>
> CI-equivalent checks before every commit:
>
> - `platform-core`: `make lint-proto` and `make breaking`.
> - Go repos: `gofmt -l .` (empty), `go vet ./...`, and `go test -race ./...` with and without `TEST_DATABASE_URL`.
>   The database is a throwaway `postgres:16-alpine` on a free high port, removed afterwards. Go runs in Docker.
> - `team-frontend`: `npm run lint`, `npx tsc --noEmit`, `npm test`, `npm run build`.
> - `platform-e2e`: `make check` and `make features-check`.

## 1. Contract (platform-core) — first, alone

- [ ] 1.1 Add the design D11 changes to `payment/v1/payment.proto`:
  - the `PAYMENT_STATUS_PARTIALLY_REFUNDED = 5` enum value;
  - the `PaymentRefundSource` enum;
  - the `PaymentRefund` message;
  - `PaymentTransaction.refunded_amount = 11` and `refunds = 12`;
  - `RefundPaymentRequest.refund_id = 4`, commented as required, idempotency key, 1–64 `[A-Za-z0-9._:-]`;
  - `WalletEntry.reference_id = 7`.

  Add the design D11 changes to `order/v1/order.proto`:
  - the `ListOrderReturns` RPC and its request and response messages;
  - `ReturnRefunded`, with the emission comment;
  - `Order.paid_at = 17` (unset = never paid online).

  Nothing may be renumbered or removed. Verify that `make lint-proto` and `make breaking` pass, and that
  `git diff --stat` touches only those two files.
- [ ] 1.2 Re-vendor `proto/` into all ten consumer repos and regenerate:
  - `team-payment`, `team-order`, `team-gateway`, `team-frontend`;
  - `team-analytics`, `team-chat`, `team-domain`, `team-engagement`, `team-notification`, `team-search`.

  Verify:
  - every vendored `payment.proto` and `order.proto` is byte-identical to platform-core;
  - `go build ./...` passes in each Go repo and `npx tsc --noEmit` passes in `team-frontend`;
  - the diffs are vendored proto only, plus generated code where a repo tracks it, with no hand edits;
  - `team-notification` and `team-analytics` filter `order.events` by envelope type. Confirm by reading their order
    consumers that `ReturnRefunded` is ignored.

## 2. team-payment (code track)

- [ ] 2.1 Add migration `0007_cumulative_refunds` (`.up.sql` and `.down.sql`) per design D9: widen `reference_id`,
  create `payment_refunds`, backfill the `LEGACY` refunds, re-point the legacy deductions, add the two status CHECKs,
  and put the pre-check queries in the header comment. Verify with a `migration_pg_test` on a database migrated to
  `0006` and seeded with four payments:
  - a legacy partial `REFUNDED` payment (500000, refunded 200000, credit, deduction);
  - a legacy full `REFUNDED` payment;
  - a pre-0006 `REFUNDED` payment with `refunded_amount` 0;
  - a `PAID` payment.

  The test asserts:
  - the up migration gives one `LEGACY` row each for the first two, re-pointed deductions, unchanged balances and an
    untouched pre-0006 row;
  - new `status=4` rows with a partial amount and `status=5` rows outside `(0, amount)` are refused;
  - the down migration restores the references and maps 5 to 4;
  - the pre-check returns 0 rows on the local `payment_db`.
- [ ] 2.2 Repository: implement `ApplyRefund` per design D1 and D2, Postgres and in-memory. It covers the payment row
  lock, idempotent key lookup, the strict, clamp and remainder modes, the refund row, `refunded_amount` and the
  status, and the deduction keyed by the refund. Add `ListRefunds(paymentID)`. Widen `SettledTransaction` and
  `CreditSettlement` to status 5. Make the credit book one deduction per positive refund row (D3). Make
  `heldCreditsSQL` join `payment_refunds` (D3). Verify unit tests and Postgres tests:
  - two partial refunds give `PARTIALLY_REFUNDED` then `REFUNDED`;
  - a refund above the remainder is refused and writes nothing;
  - a replay of the same key and amount is a no-op;
  - the same key with another amount or payment is a conflict;
  - eight concurrent refunds of 125000 on 500000 give exactly four winners;
  - two concurrent refunds of 150000 with 200000 left give one winner;
  - eight concurrent calls with the same key write one row;
  - clamp gives `min(requested, remaining)` and records an applied 0 with no deduction;
  - remainder mode on `REFUNDED` writes nothing;
  - two refunds before the credit, then the credit, give one credit and two deductions;
  - hold-back nets two partial refunds of a held sale (both under `-race`);
  - the `CHECK (refunded_amount <= amount)` backstop is never hit by the code paths.
- [ ] 2.3 Service: `RefundPayment(paymentID, refundID, amount, reason)` validates `refund_id` and uses strict mode
  with key `rpc:<refund_id>`. `RefundCancelledOrder` uses remainder mode with `cancel:<order_id>` (D4). New
  `RefundReturn(orderID, returnID, amount)` uses clamp mode with `return:<return_id>`, source `RETURN` and reason
  `return_refunded`. `GetPayment` returns the refunds. Verify service tests for each mode, including cancel after a
  partial refund refunding the remainder, cancel after a full refund being a no-op, a redelivered cancel being a
  no-op, and a return after a full refund recording an applied 0.
- [ ] 2.4 Handler: `RefundPayment` maps the design D12 codes (`INVALID_ARGUMENT` for a missing or invalid
  `refund_id`, `ALREADY_EXISTS`, `FAILED_PRECONDITION` with message `refund amount exceeds the refundable remainder`).
  `GetPayment` allows the order's seller through `OrderSellerID` (D8). `toWireTransaction` maps `refunded_amount`
  and `refunds`, and the ledger mapping fills `reference_id`. Verify handler tests:
  - each code;
  - the seller of the order reads the payment;
  - another seller gets `PERMISSION_DENIED`;
  - the buyer and admin still read it;
  - a `team-order` lookup failure is not mapped to success;
  - the wire refunds come oldest first with source and source id.
- [ ] 2.5 Consumer: handle `platform.order.v1.ReturnRefunded` (D7). Validate the return id, the order id and
  `refund_amount > 0`, and treat a violation as `ErrPermanent`. Call `RefundReturn`, with `ErrNotSettled` treated as
  permanent and sent to the DLQ. Add the `Applier.RefundReturn` method. Verify consumer unit tests with fake
  reader/DLQ: a return refunded once, a redelivery as a no-op, a clamp when a direct refund took part of the money, no
  payment going to the DLQ, a malformed payload going to the DLQ after one attempt, and a DB error retried and then
  sent to the DLQ.
- [ ] 2.6 README updates:
  - the refund model, refund ids and keys, statuses, and the RMA flow via `ReturnRefunded`;
  - the over-refund rule;
  - the `0007` pre-check, rollback semantics and deploy order.

  Add `team-payment/FEATURES.yaml` notes of what changed for `payment.refund`. Verify the README statements map to
  spec requirements, then run `gofmt -l .`, `go vet ./...` and `go test -race ./...` with and without
  `TEST_DATABASE_URL`, all green, with the output recorded in the commit message.

## 3. team-order (code track)

- [ ] 3.1 Repository changes:
  - `TransitionReturn(id, from, to)` as a CAS, Postgres and in-memory;
  - a `WithReturnOutbox(builder)` option that writes the outbox row in the CAS transaction only for
    `APPROVED → REFUNDED`;
  - `ListReturnsByOrder` (newest first);
  - a `CreateReturnCapped` transaction (order row `FOR UPDATE`, sum of the non-rejected returns, insert) per design
    D5.

  Verify repository tests (Postgres and in-memory):
  - a won `APPROVED → REFUNDED` writes one outbox row;
  - a lost CAS, a rolled-back transaction and the other transitions write none;
  - eight concurrent refunds of one return give one winner and one row;
  - eight concurrent return requests of 100000 on 500000 give exactly five inserts;
  - a rejected return frees its amount.
- [ ] 3.2 `events.BuildReturnRefundedOutboxRow`:
  - envelope type `platform.order.v1.ReturnRefunded`;
  - `ReturnRefundedEventID(return_id)` as a UUIDv5;
  - key = order id;
  - `refund_amount` from the stored return and the currency from the order.

  Wire it in `cmd/server/main.go`. Verify an outbox test that decodes the row, checks every field, and checks that the
  event id is stable across two builds.
- [ ] 3.3 Service and handler:
  - `CreateReturnRequest` uses the capped create: over the remainder → `INVALID_ARGUMENT`, a remainder of 0 →
    `FAILED_PRECONDITION`, an amount ≤ 0 defaults to the remainder.
  - `UpdateReturnStatus` uses `TransitionReturn` for every transition, and a lost CAS → `FAILED_PRECONDITION`.
  - `UpdateReturnStatus` to `REFUNDED` on an order with NULL `paid_at` → `FAILED_PRECONDITION` `order was not paid
    online; cash-on-delivery refunds are handled outside the system`, with no transition and no outbox row (D5).
  - `toWireOrder` maps `paid_at`.
  - New `ListOrderReturns` handler: buyer, seller or admin of the order, otherwise `PERMISSION_DENIED`; an unknown
    order → `NOT_FOUND`.

  Verify handler and service tests for each code. The buyer refunding their own return gets `PERMISSION_DENIED` and
  no outbox row. A COD order (shipped from `Pending`, no `paid_at`) with an `APPROVED` return is refused with the
  message above, keeps `APPROVED` and writes no outbox row, while approve and reject still work on it. Check by grep that `team-order` has no `team-payment` client.
- [ ] 3.4 README: `order.events` now also carries `ReturnRefunded` (consumed by `team-payment`), plus the return cap
  and the RMA transition rules. Verify `gofmt -l .`, `go vet ./...` and `go test -race ./...` with
  `TEST_DATABASE_URL` are green.

## 4. team-gateway (code track)

- [ ] 4.1 Add `OrderForwarder.ListOrderReturns`, which forwards with the caller's principal like `GetReturnRequest`
  and holds no logic. It gets no `policy.go` entry. Verify:
  - the unrouted/route-table test lists it as routed;
  - a forwarder test passes the request through and maps `PERMISSION_DENIED` and `NOT_FOUND` unchanged;
  - `gofmt`, `go vet` and `go test -race ./...` are green;
  - `contract-boundary-reviewer` finds no logic in the forwarder.

## 5. team-frontend (code track)

- [ ] 5.1 Delete `refundPayment` and `ViewRefundResult` from `src/lib/gateway/payment.ts` and `mockRefundAction`
  from `src/features/order/actions.ts`, along with their tests. Map `refundedAmount` and `refunds` in
  `mapTransaction`, and add `PARTIALLY_REFUNDED` to `getPaymentStatusText`. Add `listOrderReturns(orderId)` to
  `src/lib/gateway/orders.ts`. Verify by `rg` that `refundPayment` and `mockRefundAction` have no occurrence under
  `src/` (outside `src/generated`), and that `tsc` and the unit tests pass.
- [ ] 5.2 Buyer page: `ReturnRequestSection` drops the refund button, and `/account/orders/[id]` loads the order's
  returns through `listOrderReturns` and renders them with status labels. Verify the component tests show no
  `Hoàn tiền` button for any status, and that the returns render from server data after a reload (page test).
- [ ] 5.3 Seller page, per design D10:
  - add the `returns` tab to `/seller/orders/[id]`, with a new `features/seller/SellerReturns`;
  - the actions per status: `Duyệt` and `Từ chối` on `PENDING`; `Hoàn tiền` behind a confirm, and `Từ chối`, on
    `APPROVED`;
  - the server actions `approveReturnAction`, `rejectReturnAction` and `refundReturnAction` call `updateReturnStatus`
    only, then revalidate;
  - the payment summary comes from `getPayment(undefined, orderId)`, with an unavailable state;
  - an `APPROVED` return on an order without `paidAt` shows the COD message instead of `Hoàn tiền`;
  - the per-return refund state is `Đang xử lý hoàn tiền`, `Đã hoàn <amount>` or `Chỉ hoàn được <amount>`;
  - add `data-testid`s for the e2e page objects.

  Verify component and action tests for every status, the unavailable payment state, the COD message, the partial-applied label and
  the error toast on `FAILED_PRECONDITION`. Verify that no action imports a payment refund call.
- [ ] 5.4 Update the `team-frontend/FEATURES.yaml` entry for the new capability: one acceptance line per
  `seller-return-refund-ui` scenario, `status: planned`. Run `npm run lint`, `npx tsc --noEmit`, `npm test` and
  `npm run build`. Verify all are green.

## 6. Compose and gitops (code track, confirm no change)

- [ ] 6.1 Confirm the existing setup carries the change without edits:
  - `docker-compose.services.yaml` needs no new key or topic (`ReturnRefunded` rides `order.events`; the
    `team-payment.settlement` group and its DLQ already exist);
  - `team-payment-migrate` picks up `0007` from the mounted `migrations/`.

  Verify that a fresh `docker compose up` applies `0007` (`schema_migrations` version 7 in `payment_db`) and that
  `docker compose config` is unchanged.
- [ ] 6.2 Confirm there is no `platform-gitops` change. Verify that `helm template` for `team-payment`, `team-order`,
  `team-gateway` and `team-frontend` is byte-identical before and after (the images change, the values do not).

## 7. E2E track (platform-e2e + FEATURES.yaml)

- [ ] 7.1 Stack helpers and flows:
  - extend `tests/e2e/support/payment_ledger_stack.py` with a scratch-database helper. It creates a throwaway
    database on the payment postgres, runs the `team-payment-migrate` image to a given version or down one step, runs
    `psql` against it, and starts a throwaway `team-payment` container on the stack network with Kafka off and calls
    it over gRPC with an admin principal;
  - add an `rpk` consume helper that finds records on `order.events` by envelope type and return id;
  - add flows: pay an order for a seller and wait for its credit; create, approve and refund a return through the
    gateway; refund with an explicit refund id.

  Verify `make check` passes, and a smoke run that refunds one return observes its `RETURN` refund.
- [ ] 7.2 Update `FEATURES.yaml` with one `acceptance` line per spec scenario, named 1:1, `status: planned`, notes
  naming the overlay and the serial lane:
  - `team-payment/FEATURES.yaml`: update the `seller-refund-deduction` entry (the renamed and new scenarios) and the
    `seller-payout-holdback` entry (one new scenario); add `payment.cumulative-refunds`.
  - `team-order/FEATURES.yaml`: add `order.return-refund-settlement`, and rewrite `order.rma-return` to point at the
    new UI feature (the stub-based acceptance is removed).
  - `team-frontend/FEATURES.yaml`: add `frontend.seller-return-refund-ui`.

  Verify `make features-check` validates all three manifests.
- [ ] 7.3 `features/payment/cumulative_refunds.feature` and its steps cover the 15 `payment-cumulative-refunds`
  scenarios:
  - the refunds go through the gateway as the order's seller, with explicit refund ids;
  - the concurrency scenarios use a thread pool of gateway calls;
  - the migration scenarios use the scratch-database helper, in the serial lane.

  Verify they are red before groups 2–4 land and green after.
- [ ] 7.4 Update `features/payment/plp_refund_deduction.feature` and its steps for the 13 modified
  `seller-refund-deduction` scenarios (every refund step passes a refund id; the renamed scenarios are replaced, not
  duplicated). Keep `A refund after the proceeds were paid out takes the balance negative` working with a refund id.
  Add `Two partial refunds of a held sale both reduce its held amount` to `plp_payout_holdback.feature` (under the
  overlay). Leave the `seller-settlement-credit` scenarios unchanged. Verify those files are green after group 2, and
  that the unchanged settlement and holdback scenarios stay green.
- [ ] 7.5 `features/order/return_refund_settlement.feature` and its steps cover the 18 `return-refund-settlement`
  scenarios:
  - the facts are observed on `order.events` with `rpk`;
  - the stopped-`team-payment`, redelivery and DLQ scenarios run in the serial lane;
  - the concurrent return and transition scenarios use thread pools.

  Delete `features/order/rma_return.feature` and its stub-based steps in `group_a_steps.py` (`the payment is refunded
  and stock restored`). Verify red before groups 2–4 land and green after.
- [ ] 7.6 `features/seller/return_refund_ui.feature`, its steps and page objects cover the 8 `seller-return-refund-ui`
  scenarios:
  - add a `SellerOrderReturnsPage` with the returns tab, row status, action buttons, confirm, payment summary and
    per-return refund state;
  - the buyer order page object gets its returns list;
  - a readiness wait via `BasePage.wait_until_interactive` comes before every click, with no sleeps;
  - the stopped-`team-payment` scenario runs in the serial lane.

  Verify red before group 5 lands and green after.
- [ ] 7.7 Flip the new and updated FEATURES entries to `status: automated` with `covered_by`. Verify
  `make features-check` and `make check` are green.

## 8. Convergence gate

- [ ] 8.1 Run the full suite against the stack with the existing `order-inventory` and `payment-ledger` overlays,
  `-n 4`, twice, with the serial lane separately. Verify green both times, with every flake root-caused.
- [ ] 8.2 Run `make -C platform-e2e spec-check CHANGE=payment-refund-model` and `openspec validate
  payment-refund-model --strict`. Verify both pass.
- [ ] 8.3 Run `contract-boundary-reviewer` and `auth-scope-reviewer` on the diff. Verify no blocking finding.
  - Contract boundary: no cross-DB access, `team-order` does not call `team-payment`, the gateway forwarder is pure,
    the storefront talks only to the gateway.
  - Auth scope: `GetPayment` seller read, `ListOrderReturns` parties, the refund transition seller/admin only,
    `RefundPayment` unchanged.
- [ ] 8.4 Run `repo-doctor`. Verify there is no proto drift across the ten vendored copies, and that every change
  scenario has e2e coverage.
