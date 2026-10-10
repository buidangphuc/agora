> **Order.** Group 1 (contract) runs first, serially and alone; nothing else starts until 1.1 is merged in
> `platform-core`. Then two tracks run in parallel: **CODE** (groups 2–6, one agent per repo; within CODE, group 2
> `team-domain` must be deployable before group 3 `team-order` is run against the stack, and groups 4–6 are independent)
> and **E2E** (group 7, `platform-e2e` + the owning repos' `FEATURES.yaml`, written from the specs, red first). Group 8 is
> the convergence gate. One fix = one commit; each repo's CI-equivalent checks run before every commit
> (`team-domain`/`team-gateway`: `make check`; `team-order`: `gofmt -l .`, `go vet ./...`, `go test ./...` with
> `TEST_DATABASE_URL`; `team-frontend`: `npm run check`; `platform-core`: `make lint-proto` + `make breaking`). Go and buf
> are installed on the host (Homebrew).

## 1. Contract (platform-core) — serial, lands first

- [x] 1.1 Add `rpc CommitReservation(CommitReservationRequest) returns (CommitReservationResponse)` to `ListingService` in `platform-core/packages/proto/platform/listing/v1/listing.proto`, with `CommitReservationRequest { string reservation_id = 1; }` and an empty `CommitReservationResponse`, the lifecycle comment (`active → committed | released`, only `active` expires) and the release-by-`reservation_id` / required-id comments on `ReserveStock`/`ReleaseStock` (no field renumbered or removed); verify `make lint-proto` and `make breaking` pass and `git diff --stat` touches only `listing.proto`
- [x] 1.2 Re-vendor `proto/` and run `buf generate` in `team-domain`, `team-order` and `team-gateway`; verify `go build ./...` passes in each and `git diff --stat` shows only vendored proto (and committed generated paths where a repo tracks them), no hand edits
- [x] 1.3 Add ADR addenda: ADR-0008 (`committed` status, `CommitReservation`, release by `reservation_id` with stored quantity, released id refuses reserve, stock events, configurable TTL/interval) and ADR-0007 (three-phase all-or-nothing placement in one transaction, attempt-scoped reservation ids, `Idempotency-Key` on the saga, transition table + compare-and-set, cancel claim-first, voucher commit only after `Paid`); verify each statement maps to a scenario in this change's specs

## 2. team-domain (code track)

- [x] 2.1 Add migration `0010` (+ `.down.sql`): `CHECK (status IN ('active','committed','released'))` on `reservations`, `CHECK (stock >= 0)` on `listings` and `listing_variants`, all `NOT VALID`, plus `scripts/validate_0010_constraints.sql`; verify it applies on a fresh DB and on a seeded one, the down migration reverses it, and the validate script runs clean on the local `listing_db`
- [x] 2.2 Add `RESERVATION_TTL` (default `15m`) and `RESERVATION_SWEEP_INTERVAL` (default `1m`) to `internal/config` with fallback-plus-WARN for unparsable/non-positive values, wire them to `ReserveStockIdempotent` and `StartReservationSweeper`, log the effective values at sweeper start, update `.env.example` and README; verify config tests (default, override, invalid → default + warning) and `make check-env`
- [x] 2.3 Implement `CommitReservation` (Postgres + in-memory repo, service, handler gated by `RequireServiceScope("listing.write")`) with outcomes OK / idempotent OK / `FAILED_PRECONDITION` (released) / `NOT_FOUND` / `INVALID_ARGUMENT` (empty id); verify grpcserver tests for every outcome and the scope gate, and a Postgres test that a committed reservation is not restored by `SweepExpiredReservations`
- [x] 2.4 Make `ReleaseStock` release by `reservation_id` with one `UPDATE … WHERE status IN ('active','committed') RETURNING` restoring the stored quantity in the same tx; zero rows = success no-op (WARN); empty id = `INVALID_ARGUMENT`; delete the blind-add path; verify Postgres + in-memory tests: double release restores once, release after sweep is a no-op, unknown id is a no-op, stored quantity wins over request quantity, committed is releasable, concurrent release vs sweep restores once
- [x] 2.5 Make `ReserveStockIdempotent` fail with `FAILED_PRECONDITION` for an existing `released` id, reject an empty id with `INVALID_ARGUMENT` in the handler and delete the ledger-less `ReserveStock` fallback; verify tests: reserve → release → re-reserve same id fails with stock unchanged, empty id rejected, `go build ./...` shows no remaining caller of the removed path
- [x] 2.6 Write one `ListingStockChanged` outbox row per affected listing inside the reserve, release and sweep transactions, only when stock changed (stock after the change; variant stock when a variant changed); verify Postgres tests: one pending row per real change with the right stock, none for idempotent repeats / unknown release / commit, none after a rolled-back tx
- [x] 2.7 Correct existing sweeper and reservation tests to the new lifecycle and run `make check` (with `TEST_DATABASE_URL`); verify green and record the command output in the commit message

## 3. team-order (code track; runs against a team-domain with group 2)

- [x] 3.1 Add migration `0007` (+ down): `order_sagas.idempotency_key TEXT NULL` + partial unique index `(buyer_id, idempotency_key)`, `orders.paid_at TIMESTAMPTZ NULL`, `orders_status_check CHECK (status BETWEEN 1 AND 5) NOT VALID`; verify it applies on a fresh and a seeded `order_db`, reverses, and `VALIDATE CONSTRAINT` runs clean locally after a zero-row count query
- [x] 3.2 Add `RESERVATION_TTL` / `RESERVATION_SWEEP_INTERVAL` to `internal/config` (same rules as 2.2), wire `WithReservationTTL` and `runReservationSweeper`, log the effective values, update `.env.example`/README; verify config tests and the env-drift test
- [x] 3.3 Add `CommitReservation` to `upstream.DomainClient`, mark it in `methodScopes` / the service-stock wrapper so it is sent as `service-team-order` with exactly `listing.write`; verify interceptor tests: a buyer-context commit carries the service principal and scope, `GetListing` still forwards the buyer principal
- [x] 3.4 Derive reservation ids from `saga_id | cart item | listing | variant | qty` (D7) and remove the buyer-only derivation; verify tests: two attempts of one cart item never share an id, an unkeyed checkout → compensated → retry decrements stock once (with a fake domain that refuses released ids, mirroring team-domain)
- [x] 3.5 Add the `OrderPlacer.PlaceOrders` port (Postgres: one tx inserts orders + items, binds `RESERVED → COMMITTED` with `order_id` requiring `RowsAffected == len(ids)`, marks the saga `COMPLETED`; in-memory under one mutex); verify Postgres tests: everything commits together, a forced binding failure leaves zero orders, a reservation already `RELEASED` rolls everything back with `ErrReservationLost`
- [x] 3.6 Restructure `CreateOrdersFromCart` into phases A/B/C with sorted seller groups and pre-generated order ids, compensation of every held reservation + voucher on any failure, ambiguous-outcome reconcile by order-id lookup, error mapping `RESOURCE_EXHAUSTED` / `FAILED_PRECONDITION` (`ErrReservationLost`) / `INTERNAL`; verify service tests: second seller out of stock → no orders, first seller released, cart intact; success → one order per seller; commit `FAILED_PRECONDITION` → no order; ambiguous error with orders present → returned, zero releases
- [x] 3.7 Implement checkout idempotency: read and validate `idempotency-key` metadata in the `CreateOrder` handler (1–255 printable, else `INVALID_ARGUMENT` before reserving), `CreateSaga` insert-or-lookup on `(buyer_id, key)`, replay `COMPLETED` (same orders + cart-clear retry), `PENDING` → `ABORTED`, key cleared on compensation; verify handler and saga tests incl. `go test -race` on concurrent duplicates producing one order set, per-buyer scoping, failed checkout frees the key
- [x] 3.8 Add the stale-saga pass to the sweep (saga `PENDING` past the TTL with no orders → compensate, clear key) and the `Cancelled`-order pass (reservations still `COMMITTED` on a `Cancelled` order → release); verify sweep tests for both and that a `COMPLETED` saga's reservations are never touched
- [x] 3.9 Add the transition table with actor classes (D9, keeps seller `Pending → Shipped`) and `UpdateOrderStatusFrom` (Postgres conditional `UPDATE … status = ANY($3)` inside the tx that writes the `OrderPaid` outbox row; in-memory equivalent; `ErrStatusConflict` vs `ErrOrderNotFound`); remove the unconditional `UpdateOrderStatus` from the interface; route the RPC through it; verify a table-driven test over every (from, to, actor) pair, repository tests (success, conflict, not found, outbox row only on the first `Paid`), handler tests for the spec scenarios
- [x] 3.10 Rewrite `CancelOrder` (claim from `[Pending, Paid]`, winner releases the order's own reservations by id, parks failures `RELEASE_FAILED`, then releases the voucher hold; returns `ReleasePending`); verify tests with a fake domain that dedupes by reservation id: concurrent cancels release once, `Shipped` cancel releases nothing and is `FAILED_PRECONDITION`, failing release is parked and later swept, voucher released once and a voucher error does not fail the cancel
- [x] 3.11 Switch the payment consumer to `UpdateOrderStatusFrom(Paid, [Pending], paid_at=now)`; on conflict acknowledge + log; commit the voucher only when `paid_at` is set; verify consumer tests: late success after cancel leaves `Cancelled` and commits no voucher, redelivery is a no-op, a voucher-commit error redelivers and then commits once, payment-vs-cancel race ends `Cancelled` with stock released once
- [x] 3.12 Make `CreateShipment` claim `Shipped` from `[Pending, Paid]` first and create the shipment only on a won claim; verify tests: cancelled order → `FAILED_PRECONDITION` and no shipment row, paid order → shipment + `OrderShipped` outbox row
- [x] 3.13 Rebuild `GetSagaState` from the order, its reservations and `paid_at` (spec table) and make `ForceFailSaga` validate `fail_step` and report `success=false` on a parked release; verify handler tests for `Pending`, `Paid`, cancelled-all-released, cancelled-with-`RELEASE_FAILED`, unknown `fail_step`, `Shipped` force-fail, and that existing buyer/admin access tests still pass
- [x] 3.14 Add `cmd/resync-commits` (calls `CommitReservation` for every local `COMMITTED` reservation; idempotent; logs and skips `FAILED_PRECONDITION`) and a README runbook section; verify a test over in-memory repos that a second run changes nothing
- [x] 3.15 Audit callers that set order statuses outside the table (`platform-core/tools/seed-marketplace.sh`, `platform-e2e` order helpers, repo tests) and move them to legal paths; verify the seed script completes against the local stack and the existing order e2e features still pass
- [x] 3.16 Run `gofmt -l .`, `go vet ./...`, `go test ./...` (with `TEST_DATABASE_URL`, `-race` on `internal/service`) in `team-order`; verify all green

## 4. team-gateway (code track)

- [x] 4.1 After 1.2, add `ListingService/CommitReservation` to `internal/edge/unrouted_test.go` (anonymous and authenticated → 501, upstream never called); verify `make check` is green and the `contract-boundary-reviewer` reports no business logic added

## 5. team-frontend (code track)

- [x] 5.1 Generate a per-attempt key in `PlaceOrderForm` (renewed after success or when address / payment method / voucher / items change), pass it through `checkoutAction` and `createOrder` (`src/lib/gateway/orders.ts`) as the `Idempotency-Key` call header, and map `aborted` to a retryable "checkout in progress" message; verify vitest tests: two submits of one attempt send the same key, a changed voucher or a new attempt after success sends a new key, `createOrder` sets the header, `aborted` renders the message
- [x] 5.2 Run `npm run check` (biome, `tsc --noEmit`, token check, vitest); verify green (pre-existing biome failures in untouched files, if any, are listed in the PR, not fixed here)

## 6. Compose / gitops (code track)

- [x] 6.1 Add `platform-e2e/compose/order-inventory.override.yaml` setting `RESERVATION_TTL=20s` and `RESERVATION_SWEEP_INTERVAL=2s` on `team-domain` and `team-order`, and document its use in `platform-e2e/README.md`; verify `docker compose -f docker-compose.yaml -f platform-e2e/compose/order-inventory.override.yaml config` shows the values and plain `docker compose config` does not
- [x] 6.2 Confirm no deployed-environment change is needed: `platform-gitops/envs/{staging,prod}` and `envs/services/team-{domain,order}.yaml` leave both keys unset (defaults apply); verify `helm template` renders for both services are byte-identical before and after this change

## 7. E2E track (platform-e2e)

- [x] 7.1 Add every scenario of this change's specs to the owning `FEATURES.yaml` as `planned` (`team-domain`: inventory-reservations; `team-order`: order-checkout-correctness except the storefront requirement, order-lifecycle-guards, order-read-access, order-upstream-principals; `team-frontend`: the storefront idempotency requirement; `team-gateway`: the edge-route-policy scenarios); verify `make -C platform-e2e features-check` passes
- [x] 7.2 Add API helpers: `GetListing` stock reader, `listing.events` consumer filtering `ListingStockChanged` by key, concurrent-call helper with a barrier, `CreateOrder` with an `Idempotency-Key` header, docker stop/start of `team-domain-svc` for the serial lane, container-log reader for the config scenarios; verify each helper with a smoke test against the local stack
- [x] 7.3 Write `.feature` + steps for `inventory-reservations` (checkout holds once, placed order keeps stock after TTL, failed checkout and cancelled order return stock once after the TTL sweep, invalid TTL fallback, overlay cadence, stock events); verify they are red on today's code where the bug exists and green after groups 2–3, then flip to `automated`
- [x] 7.4 Write `.feature` + steps for `order-checkout-correctness` (two-seller all-or-nothing and success, unkeyed retry, same key, concurrent key, freed key, per-buyer key) and the `order-upstream-principals` TTL scenario; verify green after group 3 and flip to `automated`
- [x] 7.5 Write `.feature` + steps for `order-lifecycle-guards` (transition table, concurrent cancels, cancel vs ship, paid cancel, shipped cancel, parked release in the serial lane, late payment, payment vs cancel, voucher quota, cancelled shipment); verify green, `-n 4` repeated twice for the concurrency scenarios, then flip to `automated` (a late-payment scenario blocked by `team-payment` refusing the payment becomes `xfail(strict=True)` naming that check)
- [x] 7.6 Write `.feature` + steps for `order-read-access` (saga views, force-fail cases; the parked cases in the serial lane) and the `edge-route-policy` listing-commit 501 scenario; verify green and flip to `automated`
- [x] 7.7 Write the UI `.feature` + page-object steps for the storefront key (replayed checkout submission via Playwright request capture and resend, new checkout after a completed one) using `BasePage.wait_until_interactive`; verify green and flip to `automated`

## 8. Convergence gate

- [x] 8.1 With the whole stack up with the overlay, run the full suite twice with `-n 4` (serial lane separate); verify green both times and record the runs in the PR description
- [x] 8.2 Run `python scripts/repo_doctor.py --root .`, the `contract-boundary-reviewer` and the `auth-scope-reviewer` on the combined diff; verify no Rule 1–5 violation, `CommitReservation` is service-only, and `git diff --stat platform-core` shows only `listing.proto` and the ADR addenda
- [x] 8.3 Run every touched repo's CI checks (groups 2–6), `make -C platform-e2e features-check`, `make -C platform-e2e spec-check CHANGE=port-order-inventory-correctness` and `openspec validate port-order-inventory-correctness --strict`; verify everything passes before archive
- [x] 8.4 Validate the `NOT VALID` constraints (`team-domain` 0010, `team-order` `orders_status_check`) on the local databases after zero-violation counts; verify `pg_constraint.convalidated = true`

## Evidence (2026-10-08)

- **Contract:** edc2a10 (listing CommitReservation; `make lint-proto` and `make breaking` clean) and 99c5276 (re-vendored into domain, order and gateway).
- **Code:**
  - team-domain: 2.1–2.7.
  - team-order: 3.1–3.16.
  - Other repos: ADR addenda, gateway 501 test, frontend Idempotency-Key, e2e overlay.
  - Seed script follows the transition table (3.15).
- **Deploy:** `resync-commits` ran once on the local stack: scanned 5329, committed 7, already released by the old sweeper 5316, failed 0.
- **NOT VALID constraints** validated on the local DBs: `reservations_status_check`, `listings_stock_nonneg`, `listing_variants_stock_nonneg` and `orders_status_check` (convalidated = t).
- **e2e:** 44/44 scenarios covered (spec_sync --strict).
  - Final gate with the short-TTL overlay: 317 passed twice (`-n 4`), then 15 passed in the destructive lane.
  - The 12 order scenarios and 8 inventory scenarios that were red before the change are green.
- **Defects the e2e track found, each fixed in its own commit:**
  - team-order logged durations as nanoseconds (32cb83f).
  - A cancel with team-domain down outlived the edge deadline (71239f1, inline 2 s release budget).
- **Flake root-caused:** a recreated container is unreachable for about 30 s because callers' gRPC clients keep the old IP. `wait-ready.sh` now waits until every container has been up for 35 s. This is a harness fix, not a code fix.
- **Reviews (no blocking findings):**
  - contract-boundary: clean.
  - auth-scope follow-ups: 991b9ef (consumer uses the table), e804b22 (mismatched reserve retry refused) and 2917d71 (`inventory.write` documented as reserved).
- **Open for later waves (none is a regression):**
  - mTLS or service tokens between services.
  - Moving the stock RPCs to `inventory.write` (authz-hardening-wave2).
  - Whether `ForceFailSaga` stays routed at the edge.
  - Whether `CreateOrder` requires a USER principal.
  - Re-vendoring listing.proto in the other repos.
  - The search stock projection (search-stock-events).
