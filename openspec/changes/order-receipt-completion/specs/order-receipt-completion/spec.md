## ADDED Requirements

### Requirement: The buyer confirms receipt of a shipped order

`OrderService.ConfirmReceipt` SHALL move an order from SHIPPED to COMPLETED by compare-and-set with `completed_by` BUYER, only when the caller's principal id equals the order's `buyer_id`. It SHALL set `completed_at` to the transition time and `return_window_ends_at` to `completed_at` plus `RETURN_WINDOW` (default 15 days), SHALL clear `auto_complete_at`, and SHALL return the updated order. A return of the order in PENDING or APPROVED SHALL NOT block the buyer's confirm. A caller who is not the buyer (the seller, an admin, another buyer, a service) SHALL get `PERMISSION_DENIED` before the order's status is revealed. An order in any status other than SHIPPED, or one that lost the compare-and-set, SHALL get `FAILED_PRECONDITION` and stay unchanged. An unknown order id SHALL get `NOT_FOUND`.

#### Scenario: Order receipt: Buyer confirms a shipped order
- **WHEN** the buyer calls `ConfirmReceipt` on their SHIPPED order
- **THEN** the order reads COMPLETED through the gateway with `completed_at` set, `return_window_ends_at` equal to `completed_at` plus the configured window, and no `auto_complete_at`
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Buyer confirms a shipped order"

#### Scenario: Order receipt: Another user cannot confirm receipt
- **WHEN** the seller, then a different buyer, calls `ConfirmReceipt` on a SHIPPED order
- **THEN** each call fails with `PERMISSION_DENIED` and the order still reads SHIPPED with no `completed_at`
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Another user cannot confirm receipt"

#### Scenario: Order receipt: Confirming an order that is not shipped is refused
- **WHEN** the buyer calls `ConfirmReceipt` on their PAID order, and again on an order that is already COMPLETED
- **THEN** each call fails with `FAILED_PRECONDITION` and the order status and `completed_at` are unchanged
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Confirming an order that is not shipped is refused"

#### Scenario: Order receipt: Buyer may confirm while their return is open
- **WHEN** the buyer calls `ConfirmReceipt` on their SHIPPED order that has one PENDING return, and on another SHIPPED order that has one APPROVED return
- **THEN** each call succeeds, each order reads COMPLETED with `return_window_ends_at` set, and each return keeps its status
- **VERIFIED BY**: team-order `internal/service` `TestConfirmReceiptAllowedWithOpenReturn`

#### Scenario: Order receipt: Confirming an unknown order is not found
- **WHEN** the buyer calls `ConfirmReceipt` with an order id that does not exist
- **THEN** the call fails with `NOT_FOUND`
- **VERIFIED BY**: team-order `internal/handler` `TestConfirmReceiptUnknownOrderNotFound`

#### Scenario: Order receipt: Anonymous caller cannot confirm receipt
- **WHEN** a caller without a token calls `ConfirmReceipt` through the gateway
- **THEN** the call fails with `UNAUTHENTICATED` and no order changes
- **VERIFIED BY**: team-gateway `internal/edge` `TestConfirmReceiptRequiresToken`

#### Scenario: Order receipt: Confirm is forwarded by the gateway
- **WHEN** a logged-in buyer calls `OrderService/ConfirmReceipt` through the gateway
- **THEN** the gateway forwards it to team-order with the buyer's token and answers with the completed order instead of `unimplemented`
- **VERIFIED BY**: team-gateway `internal/edge` `TestConfirmReceiptForwarded`

#### Scenario: Order receipt: A buyer cannot complete through UpdateOrderStatus
- **WHEN** the buyer calls `UpdateOrderStatus` to COMPLETED on their SHIPPED order
- **THEN** the call fails with `PERMISSION_DENIED` and the order still reads SHIPPED
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: A buyer cannot complete through UpdateOrderStatus"

### Requirement: A shipped order completes by itself after the auto-complete window

When an order moves to SHIPPED, `team-order` SHALL set `shipped_at` to that time and `auto_complete_at` to `shipped_at` plus `AUTO_COMPLETE_AFTER` (default 168 hours). The lifecycle sweep SHALL move every SHIPPED order whose `auto_complete_at` is at or before now to COMPLETED by compare-and-set with actor system and `completed_by` SYSTEM, handling at most 100 orders per run, oldest `auto_complete_at` first. The sweep SHALL skip an order that has a return in status PENDING, or in status APPROVED when the order has a paid time (paid online), and SHALL complete it on the first run after no such return remains. An APPROVED return on a cash-on-delivery order (no paid time) SHALL NOT block the sweep. An order not yet at its `auto_complete_at` SHALL NOT be completed by the sweep. Every move to COMPLETED (buyer, seller or system) SHALL clear `auto_complete_at` in the same compare-and-set, so `auto_complete_at` is unset on an order that is not SHIPPED.

#### Scenario: Order receipt: Shipping stamps the auto-complete deadline
- **WHEN** the seller ships an order
- **THEN** the order reads SHIPPED with `shipped_at` set and `auto_complete_at` equal to `shipped_at` plus the configured window
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Shipping stamps the auto-complete deadline"

#### Scenario: Order receipt: Unconfirmed shipped order completes after the window
- **WHEN** a SHIPPED order passes its `auto_complete_at` with no confirm and the sweep runs
- **THEN** the order reads COMPLETED with `completed_at` set and no `auto_complete_at`, and `order.events` carries exactly one `OrderCompleted` for it with `completed_by` SYSTEM
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Unconfirmed shipped order completes after the window"

#### Scenario: Order receipt: A shipped order inside its window is not completed
- **WHEN** the sweep runs while a SHIPPED order's `auto_complete_at` is still in the future
- **THEN** the order stays SHIPPED and no `OrderCompleted` exists for it
- **VERIFIED BY**: team-order `internal/service` `TestSweepDoesNotCompleteBeforeDeadline`

#### Scenario: Order receipt: An open return blocks auto-complete
- **WHEN** a SHIPPED order past its `auto_complete_at` has a return in PENDING, and the sweep runs
- **THEN** the order stays SHIPPED, and it moves to COMPLETED only on the first run after that return is REJECTED
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: An open return blocks auto-complete"

#### Scenario: Order receipt: An approved unrefunded return blocks auto-complete
- **WHEN** a SHIPPED order that was paid online, past its `auto_complete_at`, has a return in APPROVED, and the sweep runs
- **THEN** the order stays SHIPPED, and it moves to COMPLETED on the first run after that return is REFUNDED
- **VERIFIED BY**: team-order `internal/service` `TestSweepSkipsOrderWithApprovedReturn`

#### Scenario: Order receipt: An approved return on a cash-on-delivery order does not block auto-complete
- **WHEN** a SHIPPED cash-on-delivery order (no paid time) past its `auto_complete_at` has a return in APPROVED, and the sweep runs
- **THEN** the order moves to COMPLETED with `completed_by` SYSTEM and the return still reads APPROVED
- **VERIFIED BY**: team-order `internal/service` `TestSweepCompletesCODOrderWithApprovedReturn`

#### Scenario: Order receipt: Seller completion clears the auto-complete deadline
- **WHEN** the seller moves a SHIPPED order to COMPLETED through `UpdateOrderStatus`
- **THEN** the stored order has `completed_at` set and `auto_complete_at` NULL, and the sweep never selects it
- **VERIFIED BY**: team-order `internal/repository` `TestCompletionClearsAutoCompleteAt`

#### Scenario: Order receipt: A rejected return does not block auto-complete
- **WHEN** a SHIPPED order past its `auto_complete_at` has only a REJECTED return, and the sweep runs
- **THEN** the order moves to COMPLETED
- **VERIFIED BY**: team-order `internal/service` `TestSweepCompletesOrderWithRejectedReturn`

#### Scenario: Order receipt: Confirm racing the sweep completes once
- **WHEN** the buyer confirms receipt of a SHIPPED order at the same moment the sweep completes it
- **THEN** exactly one call wins, the loser gets `FAILED_PRECONDITION` or skips the row, and exactly one `OrderCompleted` is written for the order
- **VERIFIED BY**: team-order `internal/service` `TestConfirmReceiptRacesSweepOneCompletion`

#### Scenario: Order receipt: The sweep handles at most one batch per run
- **WHEN** 250 SHIPPED orders are past their `auto_complete_at` and the sweep runs once
- **THEN** exactly 100 orders become COMPLETED, oldest deadline first, and the remaining 150 are completed by the next two runs
- **VERIFIED BY**: team-order `internal/service` `TestSweepAutoCompleteBatchBound`

#### Scenario: Order receipt: A repeated sweep changes nothing
- **WHEN** the sweep runs twice with no new deadlines in between
- **THEN** the second run completes no order and writes no outbox row
- **VERIFIED BY**: team-order `internal/service` `TestSweepAutoCompleteIdempotent`

### Requirement: Every completion emits one OrderCompleted fact

Every move of an order to COMPLETED, by the buyer, the seller (`UpdateOrderStatus` Shipped to Completed, which SHALL keep working) or the system, SHALL write one `OrderCompleted` row to `order.events` through the outbox in the same transaction as the status change, keyed by the order id, with the event id `order-completed:<order id>`. It SHALL carry the order id, buyer id, seller id, `total_amount`, currency, `completed_by` (BUYER, SELLER or SYSTEM), `completed_at`, `return_window_ends_at` and `paid_online` (true when the order has a paid time). A refused or lost transition SHALL write no `OrderCompleted`. If the outbox row cannot be written the status change SHALL roll back.

#### Scenario: Order receipt: Buyer confirm emits OrderCompleted with completed_by BUYER
- **WHEN** the buyer confirms receipt of a SHIPPED order that was paid online
- **THEN** `order.events` carries exactly one `OrderCompleted` for the order with `completed_by` BUYER, `paid_online` true, and the order's `total_amount`, `completed_at` and `return_window_ends_at`
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Buyer confirm emits OrderCompleted with completed_by BUYER"

#### Scenario: Order receipt: Seller completion still works and emits OrderCompleted
- **WHEN** the seller moves their SHIPPED order to COMPLETED through `UpdateOrderStatus`
- **THEN** the order reads COMPLETED and `order.events` carries exactly one `OrderCompleted` for it with `completed_by` SELLER
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Seller completion still works and emits OrderCompleted"

#### Scenario: Order receipt: A cash-on-delivery completion is marked not paid online
- **WHEN** the buyer confirms receipt of a SHIPPED cash-on-delivery order
- **THEN** its `OrderCompleted` carries `paid_online` false
- **VERIFIED BY**: team-order `internal/repository` `TestOrderCompletedPaidOnlineFalseForCOD`

#### Scenario: Order receipt: A refused completion emits nothing
- **WHEN** a non-buyer confirm, a confirm of a PAID order, and a seller move of a PAID order to COMPLETED are each refused
- **THEN** `order.events` carries no `OrderCompleted` for that order
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: A refused completion emits nothing"

#### Scenario: Order receipt: Concurrent confirms emit one OrderCompleted
- **WHEN** the buyer sends 8 concurrent `ConfirmReceipt` calls for one SHIPPED order
- **THEN** exactly one succeeds, the other seven fail with `FAILED_PRECONDITION`, and `order.events` carries exactly one `OrderCompleted` for the order
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Concurrent confirms emit one OrderCompleted"

#### Scenario: Order receipt: Outbox failure rolls the completion back
- **WHEN** the outbox insert fails while an order moves to COMPLETED
- **THEN** the order stays SHIPPED and the caller gets an error
- **VERIFIED BY**: team-order `internal/repository` `TestCompletionRollsBackWhenOutboxFails`

### Requirement: Returns are accepted only inside the return window

`CreateReturnRequest` SHALL accept a return only when the order is SHIPPED, or COMPLETED with now before `return_window_ends_at`. An order in PENDING, PAID or CANCELLED SHALL get `FAILED_PRECONDITION` ("order cannot be returned in its current status"). A COMPLETED order at or after `return_window_ends_at` SHALL get `FAILED_PRECONDITION` with the message "return window closed". One window applies to every category. The cap rules (`CreateReturnCapped`: refund amount within the order total minus the non-rejected returns) and buyer ownership SHALL stay as they are. A created return SHALL write one `ReturnRequested` outbox row in the same transaction as the insert, keyed by the order id, with the event id `return-requested:<return id>`, carrying the return id, order id, buyer id, seller id, the stored `refund_amount`, currency and creation time. A refused request SHALL write no row.

#### Scenario: Order receipt: Return on a shipped order is accepted and announced
- **WHEN** the buyer creates a return of 200000 on their SHIPPED order
- **THEN** the return reads PENDING and `order.events` carries exactly one `ReturnRequested` for it with the buyer, the seller and `refund_amount` 200000
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Return on a shipped order is accepted and announced"

#### Scenario: Order receipt: Return inside the window after completion is accepted
- **WHEN** the buyer creates a return on a COMPLETED order before its `return_window_ends_at`
- **THEN** the return is created PENDING
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Return inside the window after completion is accepted"

#### Scenario: Order receipt: Return after the window closes is refused
- **WHEN** the buyer creates a return on a COMPLETED order after its `return_window_ends_at`
- **THEN** the call fails with `FAILED_PRECONDITION` and the message "return window closed", no return exists and no `ReturnRequested` is written
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Return after the window closes is refused"

#### Scenario: Order receipt: Return at the exact window end is refused
- **WHEN** a return is created at the instant equal to `return_window_ends_at`
- **THEN** the call fails with "return window closed"
- **VERIFIED BY**: team-order `internal/service` `TestCreateReturnAtWindowEndRefused`

#### Scenario: Order receipt: Return on a paid unshipped order is refused
- **WHEN** the buyer creates a return on their PAID order
- **THEN** the call fails with `FAILED_PRECONDITION` and no return exists
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Return on a paid unshipped order is refused"

#### Scenario: Order receipt: Return on a pending or cancelled order is refused
- **WHEN** the buyer creates a return on their PENDING order, then on a CANCELLED order
- **THEN** each call fails with `FAILED_PRECONDITION`
- **VERIFIED BY**: team-order `internal/service` `TestCreateReturnPendingOrCancelledRefused`

#### Scenario: Order receipt: Another buyer cannot return the order
- **WHEN** a buyer who does not own the order creates a return on it
- **THEN** the call fails with `PERMISSION_DENIED` and no `ReturnRequested` is written
- **VERIFIED BY**: team-order `internal/service` `TestCreateReturnOtherBuyerDenied`

#### Scenario: Order receipt: Return above the cap is still refused
- **WHEN** the buyer creates a return whose amount exceeds the order total minus the non-rejected returns
- **THEN** the call fails with `INVALID_ARGUMENT` and no `ReturnRequested` is written
- **VERIFIED BY**: team-order `internal/service` `TestCreateReturnCapStillEnforced`

### Requirement: A return the seller does not answer is approved by the system

When a return is created, `team-order` SHALL set its `auto_approve_at` to the creation time plus `RETURN_AUTO_APPROVE_AFTER` (default 72 hours). The lifecycle sweep SHALL move every PENDING return whose `auto_approve_at` is at or before now to APPROVED through the `TransitionReturn` compare-and-set with actor system, at most 100 per run, oldest first. It SHALL NOT approve a return that is REJECTED, APPROVED, or REFUNDED, and SHALL NOT write a refund or a `ReturnRefunded` row: the APPROVED to REFUNDED step stays the existing seller or admin `UpdateReturnStatus` path. The system actor SHALL NOT be reachable through any RPC.

#### Scenario: Order receipt: Unanswered return is approved after the window
- **WHEN** a return stays PENDING past its `auto_approve_at` and the sweep runs
- **THEN** the return reads APPROVED and `order.events` carries no `ReturnRefunded` for it
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Unanswered return is approved after the window"

#### Scenario: Order receipt: A return inside its answer window is not approved
- **WHEN** the sweep runs while a PENDING return's `auto_approve_at` is in the future
- **THEN** the return stays PENDING
- **VERIFIED BY**: team-order `internal/service` `TestAutoApproveBeforeDeadlineNoOp`

#### Scenario: Order receipt: Rejected return is never auto-approved
- **WHEN** the seller rejects a return and its `auto_approve_at` then passes and the sweep runs
- **THEN** the return still reads REJECTED
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Rejected return is never auto-approved"

#### Scenario: Order receipt: Approved return is not touched by the sweep
- **WHEN** the seller approves a return and the sweep runs after `auto_approve_at`
- **THEN** the return still reads APPROVED, its `updated_at` is unchanged, and the sweep reports zero approvals
- **VERIFIED BY**: team-order `internal/service` `TestAutoApproveSkipsApproved`

#### Scenario: Order receipt: Seller answer racing the auto-approve produces one transition
- **WHEN** the seller rejects a return at the same moment the sweep approves it
- **THEN** exactly one transition is applied (the return reads REJECTED or APPROVED, never both) and the loser gets `FAILED_PRECONDITION` or skips the row
- **VERIFIED BY**: team-order `internal/service` `TestAutoApproveRacesSellerAnswerOneTransition`

#### Scenario: Order receipt: Auto-approved return is refunded through the existing path
- **WHEN** the seller moves an auto-approved return to REFUNDED
- **THEN** the return reads REFUNDED and `order.events` carries exactly one `ReturnRefunded` for it
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Auto-approved return is refunded through the existing path"

#### Scenario: Order receipt: A caller cannot request the system approve
- **WHEN** a buyer calls `UpdateReturnStatus` to APPROVED on their PENDING return
- **THEN** the call fails with `PERMISSION_DENIED` and the return stays PENDING
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: A caller cannot request the system approve"

#### Scenario: Order receipt: The auto-approve pass handles at most one batch per run
- **WHEN** 250 PENDING returns are past `auto_approve_at` and the sweep runs once
- **THEN** exactly 100 returns become APPROVED, oldest first, and the rest follow in the next two runs
- **VERIFIED BY**: team-order `internal/service` `TestAutoApproveBatchBound`

### Requirement: Every OrderReturn read carries a server-computed can_escalate

`CreateReturnRequest`, `GetReturnRequest`, `UpdateReturnStatus` and `ListOrderReturns` SHALL set `OrderReturn.can_escalate` to true only when the return is REJECTED and its order is SHIPPED, or COMPLETED with now before `return_window_ends_at`; in every other case it SHALL be false. It SHALL be computed from the order at the time of the read, never stored, and any value sent by a client SHALL be ignored.

#### Scenario: Order receipt: A rejected return on a shipped order can escalate
- **WHEN** the seller rejects a return on a SHIPPED order and the buyer reads it with `GetReturnRequest`
- **THEN** the return reads REJECTED with `can_escalate` true
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: A rejected return on a shipped order can escalate"

#### Scenario: Order receipt: A rejected return inside the window after completion can escalate
- **WHEN** the order is COMPLETED, the window is open, and the buyer reads its REJECTED return
- **THEN** `can_escalate` is true
- **VERIFIED BY**: team-order `internal/handler` `TestCanEscalateRejectedInsideWindow`

#### Scenario: Order receipt: A rejected return cannot escalate after the window closes
- **WHEN** the order is COMPLETED, its `return_window_ends_at` has passed, and the buyer reads its REJECTED return
- **THEN** `can_escalate` is false
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: A rejected return cannot escalate after the window closes"

#### Scenario: Order receipt: A return that is not rejected cannot escalate
- **WHEN** the buyer reads a PENDING, an APPROVED and a REFUNDED return on a SHIPPED order
- **THEN** each reads `can_escalate` false
- **VERIFIED BY**: team-order `internal/handler` `TestCanEscalateFalseUnlessRejected`

#### Scenario: Order receipt: ListOrderReturns computes can_escalate per return
- **WHEN** an order has one REJECTED and one PENDING return and the buyer calls `ListOrderReturns`
- **THEN** the REJECTED return reads `can_escalate` true and the PENDING one false
- **VERIFIED BY**: team-order `internal/handler` `TestListOrderReturnsCanEscalatePerReturn`

#### Scenario: Order receipt: A client-sent can_escalate is ignored
- **WHEN** a buyer sends `CreateReturnRequest` with `can_escalate` true in the request body
- **THEN** the created PENDING return reads `can_escalate` false
- **VERIFIED BY**: team-order `internal/handler` `TestCreateReturnIgnoresClientCanEscalate`

### Requirement: Seller ship-by deadline is stamped at payment or COD placement

When an order moves to PAID, `team-order` SHALL set `ship_by` to `paid_at` plus `SHIP_BY_AFTER` (default 48 hours), and the `OrderPaidEvent` row written in the same transaction SHALL carry `seller_id` and `ship_by`. When a cash-on-delivery order is placed, `ship_by` SHALL be `created_at` plus `SHIP_BY_AFTER` and the `OrderPlaced` event SHALL carry it; an online-pay order SHALL have no `ship_by` until paid. A cancelled order SHALL keep whatever `ship_by` it had. `ship_by` SHALL NOT change after it is set, and nothing SHALL cancel or alter an order when `ship_by` passes.

#### Scenario: Order receipt: Payment stamps ship_by and the paid event carries it
- **WHEN** an online order is paid at time T
- **THEN** the order reads `ship_by` equal to T plus the configured value and its `OrderPaidEvent` carries the same `ship_by` and the order's `seller_id`
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: Payment stamps ship_by and the paid event carries it"

#### Scenario: Order receipt: A cash-on-delivery order gets ship_by at placement
- **WHEN** a buyer places a COD order
- **THEN** the order reads `ship_by` equal to its creation time plus the configured value and its `OrderPlaced` carries the same `ship_by`
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: A cash-on-delivery order gets ship_by at placement"

#### Scenario: Order receipt: An unpaid online order has no ship_by
- **WHEN** a buyer places an online-pay order that is still PENDING
- **THEN** the order reads no `ship_by` and its `OrderPlaced` carries none
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: An unpaid online order has no ship_by"

#### Scenario: Order receipt: A passed ship_by changes nothing
- **WHEN** a PAID order's `ship_by` passes without a shipment and the sweep runs
- **THEN** the order stays PAID and no event is written for it
- **VERIFIED BY**: team-order `internal/service` `TestPassedShipByDoesNotChangeOrder`

#### Scenario: Order receipt: A changed config does not move an existing ship_by
- **WHEN** `SHIP_BY_AFTER` is changed after an order was paid
- **THEN** that order still reads its original `ship_by`
- **VERIFIED BY**: team-order `internal/repository` `TestShipByIsSnapshottedAtPayment`

### Requirement: Lifecycle durations are env config, validated at startup

`team-order` SHALL read `AUTO_COMPLETE_AFTER` (default 168h), `RETURN_WINDOW` (default 360h), `RETURN_AUTO_APPROVE_AFTER` (default 72h) and `SHIP_BY_AFTER` (default 48h) as Go durations at startup. A value that does not parse or is not positive SHALL stop startup with an error naming the variable. A stamped deadline SHALL be computed from the value in force at the moment of the stamp and SHALL NOT be recomputed.

#### Scenario: Order receipt: Defaults apply when nothing is set
- **WHEN** team-order starts with none of the four variables set
- **THEN** the settings read 168h, 360h, 72h and 48h
- **VERIFIED BY**: team-order `internal/config` `TestLifecycleDurationDefaults`

#### Scenario: Order receipt: A malformed duration stops startup
- **WHEN** team-order starts with `RETURN_WINDOW=abc`
- **THEN** startup fails with an error naming `RETURN_WINDOW`
- **VERIFIED BY**: team-order `internal/config` `TestLifecycleDurationMalformedRejected`

#### Scenario: Order receipt: A zero or negative duration stops startup
- **WHEN** team-order starts with `AUTO_COMPLETE_AFTER=0s`, then with `SHIP_BY_AFTER=-1h`
- **THEN** each start fails with an error naming the variable
- **VERIFIED BY**: team-order `internal/config` `TestLifecycleDurationNonPositiveRejected`

### Requirement: The status table lists the completion actors

`transitions.go` SHALL list `Shipped -> Completed` for the actors seller, buyer and system, and no other row SHALL change for this slice. `UpdateOrderStatus` SHALL keep resolving only seller and admin classes, so the buyer and system rows are reachable only through `ConfirmReceipt` and the sweep. A COMPLETED or CANCELLED order SHALL stay terminal.

#### Scenario: Order receipt: The table allows buyer and system to complete only from shipped
- **WHEN** `AllowedFrom(Completed, buyer)` and `AllowedFrom(Completed, system)` are evaluated, and `AllowedFrom(Completed, ...)` for each actor is checked against PAID
- **THEN** buyer and system get exactly `[Shipped]` and no actor gets PAID or PENDING
- **VERIFIED BY**: team-order `internal/service` `TestTransitionsCompletedActors`

#### Scenario: Order receipt: A completed order cannot be reopened or cancelled
- **WHEN** the buyer calls `CancelOrder` on a COMPLETED order
- **THEN** the call fails with `FAILED_PRECONDITION` and the order still reads COMPLETED
- **VERIFIED BY**: platform-e2e `features/order/order_receipt_completion.feature` "Order receipt: A completed order cannot be reopened or cancelled"

### Requirement: Existing orders are backfilled without a retroactive burst

The schema migration SHALL give every existing SHIPPED order `shipped_at` and an `auto_complete_at` of the migration time plus 7 days, every existing COMPLETED order `completed_at` equal to its `updated_at` and `return_window_ends_at` 15 days after that, every existing PAID order and COD PENDING order a `ship_by`, and every existing PENDING return an `auto_approve_at` of the migration time plus 3 days. It SHALL NOT complete or approve anything itself, and its down migration SHALL drop the new columns and indexes.

#### Scenario: Order receipt: Migration backfills deadlines without completing anything
- **WHEN** the migration runs on a database holding a SHIPPED order, a COMPLETED order, a PAID order and a PENDING return
- **THEN** each gets its deadline column set as stated, no status changes, and no outbox row is written
- **VERIFIED BY**: team-order `internal/repository` `TestMigrationBackfillsLifecycleDeadlines`

#### Scenario: Order receipt: The down migration restores the old schema
- **WHEN** the migration is applied and then rolled back
- **THEN** the new columns and indexes are gone and the previous order and return queries still work
- **VERIFIED BY**: team-order `internal/repository` `TestMigrationDownDropsLifecycleColumns`
