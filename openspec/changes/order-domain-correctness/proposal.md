## Why

A code-reading audit of the order/inventory path (re-verified against the tip of each repo before writing this) found
eleven correctness and exposure defects, several of them regressions or gaps in our own unpushed work
(`inventory-commit-and-idempotent-release`, `order-integrity-guards`). The worst ones lose or give away money-adjacent
state silently:

1. **Placed orders can have their stock returned.** After the order row is created, `team-order` binds the order to its
   reservations with a separate write whose failure is only logged (`internal/service/order.go`, "failed to commit
   reservation"). The row stays `RESERVED` with no `order_id`; the sweep (`FindReleasable` picks `RESERVED` past TTL)
   then calls `ReleaseStock`, and `team-domain` deliberately allows releasing `committed` reservations, so the stock
   of a live order is restored and can be sold twice.
2. **A checkout retry after a compensated attempt can never succeed, and real out-of-stock is mis-classified.**
   Unkeyed reservation ids are deterministic per buyer + cart item, `team-domain` refuses to re-reserve a released id
   (`FAILED_PRECONDITION`), and `team-order` maps every `ReserveStock` *error* to `ResourceExhausted`. Conversely, the
   real out-of-stock answer (`success=false` with a **nil** error) is ignored because the response is discarded, so
   the failure only surfaces later as a misleading "no longer reserved" (and, against a legacy domain, would place an
   order on stock that was never held). Unit tests hide this because the fake returns both `success=false` and an error.
3. **The stock RPCs are reachable by anyone.** `team-gateway` forwards `ListingService/ReserveStock` and `ReleaseStock`
   and its auth interceptor never rejects (anonymous callers get public scopes); `team-domain` performs no scope check
   on `ReserveStock`/`ReleaseStock`/`CommitReservation`, and an empty `reservation_id` silently falls back to a plain,
   ledger-less decrement. An anonymous caller can therefore drain any listing's stock permanently. (Verified by
   reading; not exercised on a live stack.)
4. **Price-drop notifications are missed because outbox events are published out of order.** `ClaimPending` returns rows
   via `UPDATE ... WHERE event_id IN (subquery ORDER BY ...) RETURNING ...`; `RETURNING` order is unspecified, so a
   `CREATED`/`UPDATED` pair relayed in one batch can land on the partition reversed (listing `3c95b606-...`: UPDATED
   offset 318, CREATED offset 319), and `team-notification`, which diffs against the last seen price, never fires. The
   identical query exists in `team-payment`.
5. **Every logged-in buyer gets 403 on Recommend.** `team-identity` never grants `recommendations:read`, which
   `team-ai` requires, so the recommendations row never renders.

The remaining items are smaller but each misleads an operator or a client (cancel leaves the voucher hold dangling,
multi-seller checkout half-succeeds then duplicates on retry, `GetSagaState` is fabricated and `ForceFailSaga` reports
success when the release was parked, `GetOrder` skips its ownership check for principal-less calls, `DB_MAX_CONns`
typo, `IsProcessed` swallowing DB errors). They share the same owners and are fixed together so the order domain has one
coherent, tested contract.

## What Changes

Item numbers (#1..#11) are the audit numbers used in `design.md` and `tasks.md`.

- **team-order**
  - #1 Place all orders of a checkout and bind their reservations in **one database transaction**; reservations carry
    their intended `order_id` from creation so the sweep repairs (never releases) a reservation whose order exists,
    and an ambiguous commit is reconciled before any compensation.
  - #4 Multi-seller checkout becomes **all-or-nothing**: reserve and domain-commit every seller group first, then persist
    every order atomically; no partial orders, no duplicate on a same-key retry, cart untouched on failure.
    Seller groups are processed in a deterministic order.
  - #2 Unkeyed reservation ids include the saga attempt id (a fresh attempt can always reserve); `ReserveStock`
    outcomes are classified truthfully: `success=false` -> `ResourceExhausted`, `FAILED_PRECONDITION`/`NOT_FOUND` ->
    `FailedPrecondition` (item unavailable), transport errors -> `Unavailable`.
  - #3 `CancelOrder` (and therefore `UpdateOrderStatus -> Cancelled` and `ForceFailSaga`) releases the voucher hold
    after winning the cancel claim. Corrected scope: see design D6 (a hold consumes quota only when committed, and
    team-promotion refuses to release committed holds, so refunding a voucher of a **paid** cancelled order is an
    open question, not part of this change).
  - #5 `GetSagaState` is derived from `order_sagas`/`order_reservations`/order status (no invented steps or
    timestamps); `ForceFailSaga` validates `fail_step` and reports `success=false`/a parked-release message when the
    release could not complete.
  - #6 `GetOrder` and `GetShipmentTracking` require an authenticated principal and ownership (buyer, seller or admin,
    one shared rule; `GetOrder` also admits a `service` principal holding the service-only scope `order.read`, D8a); every order RPC is audited for principal + ownership.
  - #8 Fix the `DB_MAX_CONns` tag, declare the Kafka consumer keys in `internal/config`, keep `.env.example`/README
    in step, and add a real `TestEnvExampleInSync` (as in team-domain). Also make the order-side reservation TTL
    configurable (`RESERVATION_TTL`, default 15m) so the e2e scenarios can exercise the sweep.
  - #9 `IsProcessed` returns `pgx.ErrNoRows` as "not processed" and every other error as an error.
  - Upstream client: stock RPCs are always sent with a **service principal** holding `inventory.write` (never the
    buyer's forwarded principal).
- **team-domain**
  - #7 `ReserveStock`, `ReleaseStock` and `CommitReservation` require scope `inventory.write` (`PERMISSION_DENIED`
    otherwise); `ReserveStock` with an empty `reservation_id` is `INVALID_ARGUMENT` (the ledger-less fallback is removed).
  - #10 Outbox claim is order-stable per aggregate: an insert-order `seq` column (migration), claim ordering and a
    final `ORDER BY`, no claim past an older unpublished event of the same aggregate, and the relayer stops an
    aggregate's batch at its first failure.
- **team-payment**
  - #10 Same outbox claim/relayer fix (same query shape, `payment_outbox_events`).
- **team-identity**
  - #11 Grant `recommendations:read` to buyer, seller and admin; add a test that every scope a service enforces is
    granted to the intended roles and that `inventory.write` is granted to none.
- **platform-core**: comment-only proto edits (ListingService stock RPC scope/`reservation_id` requirement, `GetSagaState`/
  `ForceFailSaga`/`GetOrder` semantics); ADR-0007, ADR-0008 and ADR-0010 amendments. **No message or RPC shape change**
  (`buf breaking` must stay clean).
- **platform-e2e** and the owning repos' `FEATURES.yaml`: API-level scenarios for #1, #2, #3, #7 (added: it is a
  security exposure), #10 and #11.

**Stacking.** The code these fixes touch lives on existing **unpushed** branches: `team-order`
`feat/order-integrity-guards` and `team-domain` `feat/release-requires-reservation-id` (plus `platform-core`
`feat/commit-reservation`). New work is committed **on top of those branches** (no rebase, no force-push, no new base):
it assumes `CommitReservation`, release-by-`reservation_id`, `RELEASE_FAILED` parking, the atomic cancel claim and
the `Idempotency-Key` saga header are all present. `team-identity`, `team-payment` and `team-gateway` (untouched here)
work from their currently checked-out branches.

## Capabilities

### New Capabilities

- `order-checkout-correctness`: atomic all-or-nothing order placement, reservation-to-order binding and sweep repair,
  reserve retry/outcome classification, voucher release on cancel (`team-order`).
- `order-read-access`: truthful saga view, honest `ForceFailSaga`, principal + ownership on order reads (`team-order`).
- `order-runtime-hygiene`: declared/effective configuration and consumer dedupe error semantics (`team-order`).
- `inventory-stock-access`: who may call the stock RPCs and what an unledgered reserve means (`team-domain`, `team-order`).
- `outbox-ordered-delivery`: per-aggregate ordered relay of transactional-outbox events (`team-domain`, `team-payment`).

### Modified Capabilities

- `auth`: adds the requirement that identity grants every scope services enforce (existing capability in
  `openspec/specs/auth`; the change is an ADDED requirement only, no existing requirement changes).

## Impact

- Repos: `team-order`, `team-domain`, `team-payment`, `team-identity`, `platform-core` (ADRs + proto comments),
  `platform-e2e`. `team-gateway`, `team-frontend`, `team-search`, `team-ai` do **not** change.
- Data: `team-domain` migration `0011` and `team-payment` migration `0005` (outbox `seq`); `team-order` migration `0007`
  (`orders.paid_at`, so the saga view can report the real payment time). `order_reservations.order_id` already exists
  and is nullable; only its meaning widens to "intended order".
- Contract: comment-only proto edits; no `buf breaking` impact; consumers need not re-vendor.
- Behaviour visible to clients: multi-seller failures now return the error with **no** order created; unkeyed checkout
  retries succeed; out-of-stock is `ResourceExhausted`; `GetOrder`/`GetShipmentTracking` reject principal-less calls;
  the stock RPCs reject non-service callers.
- Deploy order matters: `team-order` must start sending the service principal **before** `team-domain` enforces the scope
  (design "Migration Plan").
- Architecture rules: Rule 3 (only gRPC across services; the atomic placement stays inside team-order's own DB),
  Rule 4 (no contract shape change), Rule 5 (events still via outbox/Kafka); Rule 2 untouched (gateway unchanged).

## Non-goals

- RMA `APPROVED -> REFUNDED` not calling `team-payment.RefundPayment` or restocking.
- `ProcessMockPayment` returning 504 while `team-order` is down (payment settlement synchronously depending on it).
- `team-domain` `ENV` missing in the gitops overlays.
- `team-search`, `team-gateway` and `team-ai` fixes, including removing the `ReserveStock`/`ReleaseStock` routes from
  the gateway forwarder and adding `recommendations:read` to the anonymous `PUBLIC_SCOPES` (other changes; the
  domain-side scope gate in this change makes the gateway exposure non-exploitable).
- Refunding/un-redeeming a voucher that was already committed by a settled payment (needs a team-promotion semantics
  change); principal/scope gating of team-promotion's own voucher RPCs.
- Service-identity mTLS (ADR-0010 follow-up); the `inventory.write` gate is defense-in-depth on top of it, not a
  replacement.
- No UI work; e2e is API level only.
