## Context

See proposal.md, "Why". Current code (feat/ui-system, verified):

- **`team-payment` refunds.** `RefundPayment` (`internal/service/payment.go`) refuses anything not `PAID`, then
  `SettlementLedger.Refund` (`internal/repository/settlement.go`) runs one transaction. It does a CAS
  `UPDATE … SET status = 4, refunded_amount = $2 WHERE id = $1 AND status = 2`, then appends one `REFUND_DEDUCTION`
  with `reference_id = payment id` when an `ORDER_SETTLEMENT` with that reference exists.
- **Credit after refund.** `CreditSettlement` locks the payment row, takes the seller lock, inserts the credit and,
  when the payment is already `REFUNDED`, inserts the deduction from `refunded_amount`.
- **Ledger uniqueness.** `(type, reference_id)` is unique (migration `0006`). `refunded_amount` has the CHECK
  `0 ≤ refunded_amount ≤ amount`.
- **Hold-back.** `heldCreditsSQL` nets a credit with deductions of the same `reference_id`.
- **Cancel.** `RefundCancelledOrder` refunds `tx.Amount` and returns early when the payment is `REFUNDED`.
- **Amount checks.** A refund above `tx.Amount` returns a plain error, which the handler maps to `INTERNAL`.
- **Read access.** `GetPayment` is buyer-or-admin only. `RefundPayment` is order-seller-or-admin, resolving the seller
  through `team-order` `GetOrder`.
- **`team-order` returns.** `CreateReturnRequest` caps a single return at the order total; several returns per order
  are possible and their sum is unchecked.
- **Return status updates.** `UpdateReturnStatus` reads, validates and then runs an unconditional `UPDATE` (no CAS,
  no fact). The handler allows the order's seller or an admin.
- **Return reads.** There is no RPC to list an order's returns. `GetReturnByOrderID` exists in the repo only.
- **`team-order` outbox.** It already writes `OrderPaidEvent`, `OrderCancelled` (stable UUIDv5 id per order) and
  `OrderShipped` in claim transactions. Its relayer publishes to `order.events`.
- **`team-payment` consumer.** `team-payment.settlement` (`internal/consumer/settlement.go`) handles `OrderPaidEvent`
  and `OrderCancelled`, ignores other types and parks `ErrPermanent` on `order.events.payment-settlement.dlq`.
- **`team-frontend` refund stub.** `refundPayment` in `src/lib/gateway/payment.ts` returns `{ok: true}` without a
  call. `mockRefundAction` (`src/features/order/actions.ts`) calls `updateReturnStatus(REFUNDED)` and then the stub.
- **`team-frontend` return UI.** The refund button sits in the buyer's `ReturnRequestSection`, where team-order
  refuses the buyer. The seller's order page (`src/app/(shop)/seller/orders/[id]`) has no returns UI.

## Goals / Non-Goals

**Goals:**

- One refund model for every source: seller or admin RPC, return, cancel, and legacy.
- Each refund is stored once, under a deterministic key, with one deduction for the credited seller.
- The refunded amount never exceeds the payment amount under any interleaving.
- The RMA refund survives a `team-payment` outage and redelivery.

**Non-Goals:**

- Per-order ordering guarantees on `order.events` beyond what the relayer gives today. Totals are order-independent
  (D6); the split between refunds is not.
- Push or poll updates in the UI.

## Decisions

### D1. A refund is a row; the payment row serialises refunds

Migration `0007` creates `payment_refunds`:

```sql
CREATE TABLE payment_refunds (
  id               VARCHAR(96) PRIMARY KEY,                         -- stored refund key (D2)
  payment_id       VARCHAR(64) NOT NULL REFERENCES payment_transactions(id),
  source           VARCHAR(16) NOT NULL
                   CHECK (source IN ('SELLER_OR_ADMIN','RETURN','ORDER_CANCEL','LEGACY')),
  source_id        VARCHAR(64) NOT NULL,
  requested_amount BIGINT NOT NULL CHECK (requested_amount > 0),
  amount           BIGINT NOT NULL,                                 -- applied
  reason           TEXT NOT NULL DEFAULT '',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CHECK (amount >= 0 AND amount <= requested_amount),
  CHECK (amount > 0 OR source = 'RETURN')
);
-- plus an index on (payment_id, created_at)
```

`SettlementLedger.Refund` becomes `ApplyRefund(paymentID, key, source, sourceID, requested, reason, mode)` in one
transaction:

1. `SELECT … FROM payment_transactions WHERE id = $1 FOR UPDATE`. This lock is the serialisation point for every
   refund path. Lock order stays payment row → seller lock, as in `0006`.
2. If a row with the same key exists:
   - same payment and same requested amount → return the current transaction (idempotent replay);
   - otherwise → `ErrRefundIDConflict` (`ALREADY_EXISTS`).
3. If the status is not `PAID` or `PARTIALLY_REFUNDED`, return `ErrNotRefundable`. The `RETURN` mode instead records
   an applied 0 (D6).
4. Compute `remaining = amount − refunded_amount` and apply the mode:
   - `strict` mode (RPC): `requested > remaining` → `ErrExceedsRemainder` (`FAILED_PRECONDITION`).
   - `clamp` mode (return): `applied = min(requested, remaining)`.
   - `remainder` mode (cancel): `applied = remaining`. Nothing is written when it is 0.
5. Insert the refund row. Then `refunded_amount += applied`, and set the status to 4 (`REFUNDED`) when
   `refunded_amount = amount`, otherwise to 5 (`PARTIALLY_REFUNDED`).
6. If `applied > 0` and an `ORDER_SETTLEMENT` references the payment, take the seller lock and
   `insertLedgerOnce(REFUND_DEDUCTION, −applied, reference = key)`.

Alternatives considered:

- Keep the refunded amount only as a column with an optimistic CAS on `refunded_amount`. Rejected: it gives no
  per-refund identity, so idempotency and per-refund deductions are impossible.
- A ledger-only model where the refund rows are the deductions. Rejected: refunds of uncredited payments write no
  deduction, but the refund must still be recorded.

### D2. Refund ids: caller-chosen for the RPC, derived for facts, namespaced when stored

`RefundPaymentRequest.refund_id` is required (decided by the human; a missing id is `INVALID_ARGUMENT`): 1–64 chars of `[A-Za-z0-9._:-]`. Stored keys are:

| Source | Stored key | `source_id` |
|---|---|---|
| RPC | `rpc:<refund_id>` | `refund_id` |
| Return | `return:<return_id>` | `return_id` |
| Cancel | `cancel:<order_id>` | `order_id` |
| Migration | `legacy:<payment_id>` | `payment_id` |

The stored key is `PaymentRefund.id` and the deduction's `reference_id`. Namespacing makes a seller-chosen RPC id
unable to pre-empt a return's or a cancel's refund, which would otherwise make the later fact a silent no-op.

A return's refund is still keyed only by the return id, as decided. `wallet_ledger.reference_id` widens to
`VARCHAR(96)`. RPC keys are global, so reusing one on another payment is `ALREADY_EXISTS` (spec).

Alternatives considered:

- Raw ids for everything. Rejected: the collision described above.
- Keys scoped per payment. Rejected: a retried RPC with the wrong payment would then refund twice.

### D3. Deductions reference the refund; credit and hold-back follow the refund rows

`CreditSettlement`:

- Its settled set widens to status `IN (2, 4, 5)`.
- After inserting the credit, it loops over the payment's refund rows with `amount > 0` and calls
  `insertLedgerOnce(REFUND_DEDUCTION, −amount, reference = refund.id)`. This replaces the single
  `refunded_amount`-based deduction and covers N refunds before the credit.

Hold-back's `heldCreditsSQL` nets each credit with:

```sql
SELECT SUM(d.amount) FROM wallet_ledger d
JOIN payment_refunds r ON r.id = d.reference_id
WHERE d.type = 'REFUND_DEDUCTION' AND r.payment_id = c.reference_id
```

Both tables are in `payment_db` (rule 3 holds). Unique `(type, reference_id)` stays, now meaning one deduction per
refund.

Alternative considered: a `payment_id` column on `wallet_ledger`. Rejected: it duplicates `payment_refunds` and needs
its own backfill.

### D4. Cancel refunds the remainder, once

`RefundCancelledOrder` calls `ApplyRefund(key = cancel:<order_id>, source = ORDER_CANCEL, mode = remainder,
reason = order_cancelled)`.

- `REFUNDED`, or a remainder of 0 → no-op, nothing written.
- A redelivered cancel finds its key and stops.
- A replay after the payment closed stops at the status check.

### D5. `ReturnRefunded`: a CAS transition with the fact in the same transaction

`team-order` `ReturnRepository` gets `TransitionReturn(id, from, to)`, an `UPDATE … WHERE id = $1 AND status = $2
RETURNING`. A lost CAS returns `ErrInvalidReturnStatus` (`FAILED_PRECONDITION`).

- `ApplyReturnStatus` uses it for every transition, which fixes the read-then-write race.
- Only `APPROVED → REFUNDED` writes an outbox row, built by `events.BuildReturnRefundedOutboxRow` in the same
  transaction (option `WithReturnOutbox`, mirroring `WithShipmentOutbox`).
- The envelope type is `platform.order.v1.ReturnRefunded`, the key is the order id, and the event id is a UUIDv5 of
  the return id (namespace `agora/team-order/order.events/ReturnRefunded`).
- `refund_amount` comes from the row the CAS returned. The currency comes from the order.

The existing relayer publishes it.

Returns of orders not paid online (decided by the human): before the CAS, `ApplyReturnStatus` refuses `to = REFUNDED`
when the order's `paid_at` is NULL with `ErrNotPaidOnline` → `FAILED_PRECONDITION` `order was not paid online;
cash-on-delivery refunds are handled outside the system`. `paid_at` is written only by the `Pending → Paid` CAS, so
it is exactly "paid through `team-payment`". The return keeps its status and no outbox row is written. Approve and
reject are unaffected. `paid_at` never goes from set to NULL, so a check outside the CAS transaction cannot be raced.
Alternative considered: keep emitting and let `team-payment` park the fact. Rejected by the human: the return would
read `REFUNDED` while nothing was refunded.

The return cap (spec):

- `CreateReturnRequest` runs in one transaction. It locks the order row with `SELECT … FROM orders WHERE id = $1
  FOR UPDATE`, sums `refund_amount` over the order's returns with `status <> 3`, applies the remainder rule, and then
  inserts.
- The in-memory repo does the same under its mutex.
- `refund_amount ≤ 0` means "the remainder".

`ListOrderReturns(order_id)`:

- It reads the order for buyer/seller/admin authorisation, then calls `ListReturnsByOrder`, which orders by
  `created_at DESC, id DESC`.
- `team-gateway` adds a forwarder `OrderForwarder.ListOrderReturns` like `GetReturnRequest`. It has no policy entry
  and is not added to the unrouted list.

### D6. Over-refund rule for returns: two caps, clamp at refund time, never park

- **Cap 1 (`team-order`, at request time).** `Σ refund_amount` of the order's non-rejected returns never exceeds the
  order total. `team-order` owns returns, so this is a local, synchronous check with a clear `INVALID_ARGUMENT` for
  the buyer.
- **Cap 2 (`team-payment`, at refund time).** The payment's remainder is authoritative, because other paths (seller
  or admin RPC, cancel) refund the same payment. For a `ReturnRefunded`, `team-payment` refunds `min(refund_amount,
  remaining)` and records requested vs applied. An applied 0 is recorded with no deduction.

Why clamp:

- The fact is a past event from the owner of returns. It cannot be refused back to the producer, and nobody would
  act on it if it were parked.
- A parked record would leave the return `REFUNDED` in `team-order` with the buyer refunded nothing, until a human
  replays it, and the replay would hit the same remainder.
- Clamping gives the buyer the most the payment can still return and keeps `refunded_amount ≤ amount`. The recorded
  requested/applied pair keeps the shortfall visible: the seller UI shows `Chỉ hoàn được …`.

Why not a synchronous check from `team-order` against `team-payment` at `REFUNDED` time: it couples return handling
to `team-payment` availability (the "RMA refund while team-payment is stopped" scenario) and still races with a
concurrent seller refund.

The RPC path stays strict (`FAILED_PRECONDITION`) because its caller is present and can retry with a smaller amount.
Cap 1 also means the clamp can only trigger when a non-return path took part of the money. Totals are
order-independent: whichever of a return and a cancel is applied first, the sum of applied amounts is the payment
amount.

### D7. Facts that cannot be applied go to the DLQ

The following are `ErrPermanent`, so they go to the DLQ and write nothing:

- a `ReturnRefunded` whose order has no settled payment (`ErrNotSettled`). `team-order` no longer emits one for an
  order not paid online (D5), so this is a defensive fallback;
- one missing `return_id` or `order_id`;
- one with `refund_amount ≤ 0`.

An applied 0 is not an error (D6). The consumer's type switch adds `platform.order.v1.ReturnRefunded`, and the
`Applier` interface gains `RefundReturn(ctx, orderID, returnID string, amount int64)`.

### D8. Seller read of the payment

`GetPayment` keeps buyer-or-admin. When neither applies, it resolves the order's seller with the existing
`OrderSellerID` (team-order `GetOrder`) and allows a match. Lookup failures map as `RefundPayment` does today
(`NOT_FOUND` / `INTERNAL`), and a mismatch is `PERMISSION_DENIED`.

The extra hop happens only for non-buyer callers. The handler maps `refunded_amount` and `refunds` (D11), and
`toWireLedgerEntry` maps `reference_id`.

### D9. Migration `0007_cumulative_refunds` and the existing single-refund rows

Pre-check, which must return 0 rows before apply on a long-lived DB:

```sql
SELECT id FROM payment_transactions WHERE status = 5;
SELECT d.id FROM wallet_ledger d
WHERE d.type = 'REFUND_DEDUCTION'
  AND d.reference_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM payment_transactions t WHERE t.id = d.reference_id);
```

Up:

1. `ALTER wallet_ledger ALTER reference_id TYPE VARCHAR(96)`.
2. `CREATE TABLE payment_refunds` (D1).
3. Backfill:

   ```sql
   INSERT INTO payment_refunds (id, payment_id, source, source_id, requested_amount, amount, reason, created_at)
   SELECT 'legacy:' || id, id, 'LEGACY', id, refunded_amount, refunded_amount, provider_reference, updated_at
   FROM payment_transactions
   WHERE status = 4 AND refunded_amount > 0;
   ```

4. Re-point the legacy deductions:

   ```sql
   UPDATE wallet_ledger SET reference_id = 'legacy:' || reference_id
   WHERE type = 'REFUND_DEDUCTION' AND reference_id IN (SELECT payment_id FROM payment_refunds WHERE source = 'LEGACY');
   ```

5. Add the CHECK `status <> 4 OR refunded_amount = amount` as `NOT VALID`. Legacy partial `REFUNDED` rows and
   pre-0006 `REFUNDED` rows with `refunded_amount = 0` are exempt. They are terminal and never updated again, because
   every refund path stops at `REFUNDED`.
6. Add the CHECK `status <> 5 OR (refunded_amount > 0 AND refunded_amount < amount)`, validated.

Legacy partial refunds stay `REFUNDED` and closed. Reopening them as `PARTIALLY_REFUNDED` would let a replay of a
historical `OrderCancelled` (consumer group reset) refund the remainder years later. Under the old model that cancel
was a no-op, so replaying history would change the ledger. Keeping them closed preserves exactly what buyers and
sellers already saw. No ledger amount changes, so balances and hold-back are unchanged: the legacy deduction nets
against its credit through the legacy refund row (D3).

Down:

1. Set `status = 4` where `status = 5`. Old code does not know 5, and its credit path would dead-letter such
   payments.
2. Strip the `legacy:` prefix from deduction references.
3. Drop the CHECKs and `payment_refunds`.
4. Narrow `reference_id` only if no value exceeds 64 characters; otherwise leave it at 96.

Deductions written by new refunds keep their `rpc:`/`return:`/`cancel:` references. Old hold-back then does not net
them, which holds more than necessary: safe, never less.

### D10. Storefront

- `refundPayment` (the stub) and `mockRefundAction` are deleted.
- `ReturnRequestSection` (buyer) loses the refund button and loads `ListOrderReturns` server-side, so it lists
  returns after a reload.
- New `src/lib/gateway/orders.ts` `listOrderReturns(orderId)`. `getPayment` maps `refundedAmount` and `refunds`.
- The seller order page gets a `returns` tab (`?tab=returns`) rendering a new `features/seller/SellerReturns`.
  - It lists the returns with server actions `approveReturnAction`, `rejectReturnAction` and `refundReturnAction`.
  - Each action calls `updateReturnStatus` only and then `revalidatePath`.
  - A payment summary comes from `getPayment(undefined, orderId)`. It renders an "unavailable" state on failure.
  - An `APPROVED` return on an order without `paidAt` shows the COD message in place of `Hoàn tiền`. A refusal from
    the server for a stale page is shown as the error toast.
  - Each `REFUNDED` return's state is derived from `payment.refunds` by `source = RETURN && sourceId = return.id`.
- `getPaymentStatusText` adds `PARTIALLY_REFUNDED` → `Đã hoàn một phần`.
- No polling: the seller sees the applied amount on reload (proposal Non-goals).

### D11. Contract (platform-core, one commit, additive)

`payment/v1/payment.proto`:

```proto
enum PaymentStatus { … PAYMENT_STATUS_PARTIALLY_REFUNDED = 5; }
enum PaymentRefundSource {
  PAYMENT_REFUND_SOURCE_UNSPECIFIED = 0;
  PAYMENT_REFUND_SOURCE_SELLER_OR_ADMIN = 1;
  PAYMENT_REFUND_SOURCE_RETURN = 2;
  PAYMENT_REFUND_SOURCE_ORDER_CANCEL = 3;
  PAYMENT_REFUND_SOURCE_LEGACY = 4;
}
message PaymentRefund {
  string id = 1; PaymentRefundSource source = 2; string source_id = 3;
  int64 requested_amount = 4; int64 amount = 5; string reason = 6;
  google.protobuf.Timestamp created_at = 7;
}
message PaymentTransaction { …; int64 refunded_amount = 11; repeated PaymentRefund refunds = 12; }
message RefundPaymentRequest { …; string refund_id = 4; }   // required by the server
message WalletEntry { …; string reference_id = 7; }
```

`order/v1/order.proto`:

```proto
rpc ListOrderReturns(ListOrderReturnsRequest) returns (ListOrderReturnsResponse);
message Order { …; google.protobuf.Timestamp paid_at = 17; }   // unset = never paid online
message ListOrderReturnsRequest { string order_id = 1; }
message ListOrderReturnsResponse { repeated OrderReturn returns = 1; }
// emitted via the outbox in the APPROVED→REFUNDED CAS; event id stable per return
message ReturnRefunded {
  string return_id = 1; string order_id = 2; string buyer_id = 3; string seller_id = 4;
  int64 refund_amount = 5; string currency = 6; google.protobuf.Timestamp refunded_at = 7;
}
```

`PaymentSettled` is unchanged: it is still emitted only as `PAID`, and `team-order`'s consumer ignores other
statuses.

### D12. Error mapping (edge-observable)

| Condition | Code |
|---|---|
| `refund_id` missing or invalid | `INVALID_ARGUMENT` |
| amount ≤ 0 | `INVALID_ARGUMENT` |
| refund id reused with another payment or amount | `ALREADY_EXISTS` |
| payment `REFUNDED`, or not paid | `FAILED_PRECONDITION` |
| amount above the remainder | `FAILED_PRECONDITION` `refund amount exceeds the refundable remainder` |
| return above the order remainder | `INVALID_ARGUMENT` |
| return requested with an order remainder of 0 | `FAILED_PRECONDITION` |
| lost or invalid return transition | `FAILED_PRECONDITION` |
| return refund on an order not paid online | `FAILED_PRECONDITION` `order was not paid online; cash-on-delivery refunds are handled outside the system` |

## Risks / Trade-offs

- [Return and cancel arrive in either order] → The split between the `RETURN` and `ORDER_CANCEL` refunds depends on
  the arrival order, but the totals do not. The spec asserts totals only (scenario "A return refund and a cancel of
  the same paid order …").
- [An old `team-payment` ignores `ReturnRefunded`, commits it, and loses it] → The deploy order puts `team-payment`
  first (Migration Plan). If a fact was lost anyway, resetting the group offset replays it safely, because facts are
  idempotent.
- [The clamp hides a shortfall] → The requested and applied amounts are stored and shown to the seller. Cap 1 keeps
  returns alone within the order total.
- [Seller `GetPayment` adds a sync hop to `team-order`] → It happens only for non-buyer callers. The UI degrades to
  "payment unavailable" while still listing returns.
- [The order-row lock in `CreateReturnRequest` contends with the order's CAS writes] → The lock is short and held
  per order. Returns are rare.
- [Callers of `RefundPayment` without `refund_id` break] → No production caller exists: the storefront never called
  it. The e2e helpers are updated in the e2e track.
- [Pre-0006 `REFUNDED` rows with `refunded_amount = 0` have an unknown refunded amount] → They are left closed and
  untouched, and were never deducted (as before).

## Migration Plan

1. `platform-core` proto commit alone (`make lint-proto`, `make breaking`). Re-vendor all copies.
2. Deploy `team-payment`:
   - Run the D9 pre-check, then migration `0007`.
   - The new code understands `PARTIALLY_REFUNDED`, refund rows, `ReturnRefunded`, and the seller read.
   - The new `RefundPayment` requires `refund_id`.
3. Deploy `team-gateway` (the `ListOrderReturns` route).
4. Deploy `team-order`, which starts emitting `ReturnRefunded` and enforces the return cap and the CAS.
5. Deploy `team-frontend`.
6. Rollback runs in reverse: `team-order` first, so no new facts are emitted, then `team-payment`'s down migration
   (D9: `PARTIALLY_REFUNDED` becomes `REFUNDED`, which closes those payments; this is documented in the README).

No compose, topic, env or gitops change. `platform-gitops` `helm template` output stays byte-identical.
