# order-lifecycle-contract Specification

## Purpose
Order and payment proto contract for the order lifecycle: ConfirmReceipt, lifecycle enums and fields, the new order.events messages and PaymentStatus CANCELLED.

## Requirements

### Requirement: OrderService exposes ConfirmReceipt and the lifecycle enums

`OrderService` in `platform/order/v1/order.proto` SHALL gain `ConfirmReceipt(ConfirmReceiptRequest) returns (ConfirmReceiptResponse)` with `ConfirmReceiptRequest {id=1}` and `ConfirmReceiptResponse {order=1}`. The file SHALL define `enum CancelReason {UNSPECIFIED=0, CHANGED_MIND=1, WRONG_ITEM=2, DAMAGED=3, DELIVERY_DELAY=4, OTHER=5, PAYMENT_EXPIRED=6, ADMIN_FORCE_FAIL=7}` (each value prefixed `CANCEL_REASON_`) and `enum CompletedBy {UNSPECIFIED=0, BUYER=1, SELLER=2, SYSTEM=3}` (prefixed `COMPLETED_BY_`). `CancelOrderRequest` SHALL gain `CancelReason reason_code = 3`. The comments SHALL state: `ConfirmReceipt` is buyer-of-the-order only, SHIPPED to COMPLETED, allowed while the buyer's return is PENDING or APPROVED, `PERMISSION_DENIED` for any other caller and `FAILED_PRECONDITION` for a non-SHIPPED order; `PAYMENT_EXPIRED` is system only and `ADMIN_FORCE_FAIL` is ForceFailSaga only, so a buyer sending either gets `PERMISSION_DENIED`. Existing RPCs, enum values and fields SHALL NOT change.

#### Scenario: Order lifecycle contract: ConfirmReceipt RPC exists
- **WHEN** the compiled `OrderService` descriptor is inspected
- **THEN** it lists every existing RPC unchanged plus `ConfirmReceipt` with request type `ConfirmReceiptRequest` (`id`=1 string) and response type `ConfirmReceiptResponse` (`order`=1 `Order`)
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/order/v1/lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Enums have fixed names and numbers
- **WHEN** the descriptor test reads `CancelReason` and `CompletedBy`
- **THEN** the values are exactly `CANCEL_REASON_UNSPECIFIED=0` through `CANCEL_REASON_ADMIN_FORCE_FAIL=7` and `COMPLETED_BY_UNSPECIFIED=0` through `COMPLETED_BY_SYSTEM=3` as listed, with no other value
- **VERIFIED BY**: Go descriptor test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: CancelOrderRequest carries a reason code
- **WHEN** the descriptor test reads `CancelOrderRequest`
- **THEN** `id`=1 and `reason`=2 (string) are unchanged and `reason_code`=3 has type `CancelReason`
- **VERIFIED BY**: Go descriptor test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: ConfirmReceipt authz and preconditions are documented
- **WHEN** a reader opens the `ConfirmReceipt` RPC comment to learn what a caller who is not the owner (not the buyer) gets
- **THEN** it contains "buyer of the order only", "SHIPPED", "COMPLETED", "return is PENDING or APPROVED", `PERMISSION_DENIED` for any other caller and `FAILED_PRECONDITION` for a non-SHIPPED order
- **VERIFIED BY**: comment-text assertion in `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: System-only cancel reasons are documented as refused for buyers
- **WHEN** a reader opens the comments of `CancelOrderRequest.reason_code`, `CANCEL_REASON_PAYMENT_EXPIRED` and `CANCEL_REASON_ADMIN_FORCE_FAIL`
- **THEN** they state that a buyer sending `PAYMENT_EXPIRED` or `ADMIN_FORCE_FAIL` gets `PERMISSION_DENIED`, that `PAYMENT_EXPIRED` is system only and `ADMIN_FORCE_FAIL` is ForceFailSaga only
- **VERIFIED BY**: comment-text assertion in `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Missing authz note is caught
- **WHEN** the test's comment helper runs against a fixture `ConfirmReceipt` comment lacking `PERMISSION_DENIED`
- **THEN** the helper reports failure (the test expects it), proving a missing note is caught
- **VERIFIED BY**: comment-text assertion in `lifecycle_compat_test.go` (tasks 2.1)

### Requirement: Order, OrderReturn and existing events gain lifecycle fields

`Order` SHALL gain, with exactly these numbers: `payment_due_at=18`, `ship_by=19`, `shipped_at=20`, `completed_at=21`, `return_window_ends_at=22` (Timestamp), `cancel_reason=23` (`CancelReason`), `auto_complete_at=24` (Timestamp); fields 1-17 SHALL be unchanged; numbers 25 and above belong to q11-checkout-coherence and this slice neither adds nor asserts anything about them. `OrderReturn` SHALL gain `bool can_escalate = 10`. `OrderCancelled` SHALL gain `CancelReason reason_code = 8`. `OrderPaidEvent` SHALL gain `string seller_id = 7` and `Timestamp ship_by = 8`. The comments SHALL carry the derivations: `payment_due_at` = created_at + 24h and unset for COD and for orders placed before the deploy; `ship_by` = paid_at (COD: created_at) + 2 days; `return_window_ends_at` = completed_at + 15 days; `auto_complete_at` = shipped_at + 7 days and unset unless SHIPPED; `can_escalate` is server-computed at read, true when the return is REJECTED and the order is SHIPPED or inside its return window, and re-escalation after a closed dispute is allowed while it is true; durations are env config read when the field is snapshotted.

#### Scenario: Order lifecycle contract: Order lifecycle fields have fixed numbers
- **WHEN** the descriptor test reads `Order`
- **THEN** fields 1 to 17 equal the committed expectations and fields 18 to 24 are `payment_due_at`, `ship_by`, `shipped_at`, `completed_at`, `return_window_ends_at`, `cancel_reason` (`CancelReason`), `auto_complete_at` with the types above (fields numbered 25 or higher are not asserted; q11-checkout-coherence owns them)
- **VERIFIED BY**: Go descriptor test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Return and event additions are fixed
- **WHEN** the descriptor test reads `OrderReturn`, `OrderCancelled` and `OrderPaidEvent`
- **THEN** `OrderReturn.can_escalate`=10 (bool), `OrderCancelled.reason_code`=8 (`CancelReason`), `OrderPaidEvent.seller_id`=7 (string) and `OrderPaidEvent.ship_by`=8 (Timestamp) exist, and every previously existing field of the three messages keeps its name and number
- **VERIFIED BY**: Go descriptor test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Order with lifecycle fields round-trips
- **WHEN** an `Order` with `payment_due_at` 2026-10-17T00:00:00Z, `cancel_reason` `CANCEL_REASON_PAYMENT_EXPIRED` and `auto_complete_at` 2026-10-24T00:00:00Z is serialised and parsed
- **THEN** every field parses back equal, and an `Order` with none of fields 18-24 set parses back with all seven unset (COD has no `payment_due_at`)
- **VERIFIED BY**: Go round-trip test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Old readers skip the new Order fields
- **WHEN** bytes of an `Order` carrying fields 18-24 are parsed with a schema that knows only fields 1-17
- **THEN** parsing succeeds, fields 1-17 are intact and 18-24 are skipped without error
- **VERIFIED BY**: Go test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Unknown cancel reason value parses
- **WHEN** `Order` bytes whose `cancel_reason` varint is 99 are parsed
- **THEN** parsing succeeds and the stored enum number is 99 (proto3 open enum), so a consumer built later can still read the order
- **VERIFIED BY**: Go test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Derivation rules are documented
- **WHEN** a reader opens the comments of the `Order` fields 18-24 and of `OrderReturn.can_escalate`
- **THEN** they contain the derivations listed in the requirement (created_at + 24h, unset for COD, before the deploy, 2 days, 15 days, 7 days, REJECTED and SHIPPED-or-inside-window, server-computed)
- **VERIFIED BY**: comment-text assertion in `lifecycle_compat_test.go` (tasks 2.1)

### Requirement: order.events gains four lifecycle messages

`order.proto` SHALL define, with exactly these field numbers:
- `OrderPlaced {order_id=1, buyer_id=2, seller_id=3 (string), payment_method=4 (platform.payment.v1.PaymentMethod), total_amount=5 (int64), currency=6, placed_at=7, payment_due_at=8, ship_by=9 (Timestamp)}`
- `OrderPaymentReminder {order_id=1, buyer_id=2, total_amount=3 (int64), currency=4, payment_due_at=5 (Timestamp)}`
- `OrderCompleted {order_id=1, buyer_id=2, seller_id=3, total_amount=4 (int64), currency=5, completed_by=6 (CompletedBy), completed_at=7, return_window_ends_at=8 (Timestamp), paid_online=9 (bool)}`
- `ReturnRequested {return_id=1, order_id=2, buyer_id=3, seller_id=4, refund_amount=5 (int64), currency=6, requested_at=7 (Timestamp)}`

The comments SHALL state: events go through the order outbox to `order.events` keyed by order id with `EventEnvelope.type` equal to the full message name; `OrderPlaced` is emitted for every placed order, `payment_due_at` is unset for COD and `ship_by` is set for COD only; `OrderPaymentReminder` is emitted once per order at `payment_due_at - 2h`; `OrderCompleted` is emitted once per order and is consumed by the referral reward and by dispute escalation; `ReturnRequested` is emitted once per created return.

#### Scenario: Order lifecycle contract: New event messages have fixed fields
- **WHEN** the descriptor test reads `OrderPlaced`, `OrderPaymentReminder`, `OrderCompleted` and `ReturnRequested`
- **THEN** every field name, number and type equals the list above and no other field exists
- **VERIFIED BY**: Go descriptor test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Completed event round-trips
- **WHEN** an `OrderCompleted` with `completed_by` `COMPLETED_BY_SYSTEM`, `paid_online` true, `total_amount` 9223372036854775807 and both timestamps set is serialised and parsed
- **THEN** every field parses back equal and the int64 is unchanged (no float or narrowing)
- **VERIFIED BY**: Go round-trip test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: COD placed event round-trips without a deadline
- **WHEN** an `OrderPlaced` with `payment_method` `PAYMENT_METHOD_COD`, `ship_by` set and `payment_due_at` unset is serialised and parsed
- **THEN** `ship_by` parses back equal and `payment_due_at` is unset (`has` is false)
- **VERIFIED BY**: Go round-trip test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Reminder and return events round-trip
- **WHEN** an `OrderPaymentReminder` and a `ReturnRequested` with every field set are serialised and parsed
- **THEN** every field parses back equal
- **VERIFIED BY**: Go round-trip test `lifecycle_compat_test.go` (tasks 2.1)

#### Scenario: Order lifecycle contract: Event semantics are documented
- **WHEN** a reader opens the comments of the four new messages
- **THEN** they state the outbox/`order.events` key and envelope type rule, "every placed order", "unset for COD" for `payment_due_at`, "COD only" for `ship_by`, "once per order" for reminder and completed, and "once per created return"
- **VERIFIED BY**: comment-text assertion in `lifecycle_compat_test.go` (tasks 2.1)

### Requirement: PaymentStatus gains CANCELLED

`PaymentStatus` in `platform/payment/v1/payment.proto` SHALL gain `PAYMENT_STATUS_CANCELLED = 6`, commented as: order cancelled before payment, `ProcessMockPayment` refuses it. Values 0-5 SHALL NOT change.

#### Scenario: Order lifecycle contract: PAYMENT_STATUS_CANCELLED exists
- **WHEN** the descriptor test reads `PaymentStatus`
- **THEN** values 0 to 5 are unchanged and `PAYMENT_STATUS_CANCELLED` is 6 with a comment containing "ProcessMockPayment refuses it"
- **VERIFIED BY**: Go descriptor test `lifecycle_compat_test.go` (tasks 2.1)

### Requirement: The change is backward compatible and generates in every language

The change SHALL pass `buf lint` and `buf breaking` against the committed base, and the generated code SHALL expose the new RPC, enums and messages in every generated language.

#### Scenario: Order lifecycle contract: Change is backward compatible
- **WHEN** `buf breaking` against the base commit the slice branches from (main) runs on the proto before the slice commit
- **THEN** it exits 0 with no findings
- **VERIFIED BY**: `make -C platform-core breaking` (tasks 1.3)

#### Scenario: Order lifecycle contract: Lint passes
- **WHEN** `buf lint` runs on the proto
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core lint-proto` (tasks 1.3)

#### Scenario: Order lifecycle contract: Wire-breaking edit is rejected
- **WHEN** after the slice commit, a mutated copy renumbers or removes any new field, enum value or RPC
- **THEN** `buf breaking` against the slice's own commit (HEAD) exits non-zero on the mutated copy, and the committed descriptor test, which pins every number, fails on the same edit
- **VERIFIED BY**: `make -C platform-core breaking` (compares against HEAD = the slice commit) run on a mutated copy plus `lifecycle_compat_test.go` (tasks 1.3, 2.1)

#### Scenario: Order lifecycle contract: New contract in every generated language
- **WHEN** `buf generate` runs on the proto
- **THEN** the Go, Python, JVM and TS outputs each expose `ConfirmReceipt`, `CancelReason`, `CompletedBy`, `OrderCompleted` and `PAYMENT_STATUS_CANCELLED`
- **VERIFIED BY**: `make -C platform-core proto` followed by a grep of each generated tree (tasks 2.2)

### Requirement: Order and payment proto copies are identical across repos

The updated `order.proto` SHALL be copied byte-identical to `proto/platform/order/v1/order.proto` in team-frontend, team-order, team-engagement, team-gateway, team-domain, team-notification, team-chat, team-payment, team-search and team-analytics, and `payment.proto` to `proto/platform/payment/v1/payment.proto` in team-chat, team-gateway, team-frontend, team-payment, team-search, team-analytics, team-notification, team-order, team-domain and team-engagement. Consumers SHALL NOT edit their copy, and each consumer slice SHALL re-run the `cmp` check before it is done.

#### Scenario: Order lifecycle contract: Copies match the source
- **WHEN** each of the twenty copies is compared with its source under `platform-core/packages/proto/platform/`
- **THEN** all comparisons report identical content (exit 0) and every `order.proto` copy contains `ConfirmReceipt` and every `payment.proto` copy contains `PAYMENT_STATUS_CANCELLED`
- **VERIFIED BY**: one-off slice-done `cmp` of the twenty copies against the sources (tasks 2.3)

#### Scenario: Order lifecycle contract: Drifted or missing copy is rejected
- **WHEN** any consumer copy differs from its source by one byte, or the file is missing
- **THEN** the `cmp` check exits non-zero (a missing file is a failure) and the contract slice is not done
- **VERIFIED BY**: one-off slice-done `cmp` of the twenty copies against the sources (tasks 2.3)

#### Scenario: Order lifecycle contract: Consumer copies lint
- **WHEN** `buf lint` runs in the `proto` directory of each copy owner
- **THEN** each exits 0
- **VERIFIED BY**: `buf lint` per consumer proto directory (tasks 2.3)
