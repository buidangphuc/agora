## ADDED Requirements

### Requirement: HasCompletedOrder reports whether a buyer has a completed order

`OrderService.HasCompletedOrder` SHALL return `has_completed = true` iff the buyer named by `buyer_id` has at least
one order in status `ORDER_STATUS_COMPLETED`, whoever completed it (`completed_by` is not filtered).

#### Scenario: Buyer with a completed order

- **WHEN** a SERVICE principal with `order.read` calls `HasCompletedOrder{buyer_id: A}` and A has one COMPLETED order
- **THEN** the system SHALL return `has_completed = true`

#### Scenario: Buyer with a completed and a cancelled order

- **WHEN** a SERVICE principal with `order.read` calls `HasCompletedOrder{buyer_id: A}` and A has one COMPLETED order and one CANCELLED order
- **THEN** the system SHALL return `has_completed = true`

#### Scenario: Buyer with only non-completed orders

- **WHEN** a SERVICE principal with `order.read` calls `HasCompletedOrder{buyer_id: B}` and B's orders are PENDING, PAID, SHIPPED and CANCELLED, none COMPLETED
- **THEN** the system SHALL return `has_completed = false`

#### Scenario: Buyer with no orders

- **WHEN** a SERVICE principal with `order.read` calls `HasCompletedOrder{buyer_id: C}` and C has no order
- **THEN** the system SHALL return `has_completed = false` with OK status, not NOT_FOUND

#### Scenario: Another buyer's completed order does not count

- **WHEN** only buyer A has a COMPLETED order and the service calls `HasCompletedOrder{buyer_id: B}`
- **THEN** the system SHALL return `has_completed = false`

#### Scenario: Completed by seller or system counts

- **WHEN** the service calls `HasCompletedOrder` for a buyer whose only COMPLETED order has `completed_by` SELLER or SYSTEM
- **THEN** the system SHALL return `has_completed = true`

#### Scenario: Status change is visible on the next call

- **WHEN** B's SHIPPED order is completed and the service calls `HasCompletedOrder{buyer_id: B}` again
- **THEN** the second call SHALL return `has_completed = true` while the call before the completion returned `false`

#### Scenario: Empty buyer id

- **WHEN** a SERVICE principal with `order.read` calls `HasCompletedOrder{buyer_id: ""}`
- **THEN** the system SHALL fail with `INVALID_ARGUMENT` and run no query

#### Scenario: Store failure is not reported as false

- **WHEN** the store query fails during `HasCompletedOrder`
- **THEN** the system SHALL fail with `INTERNAL` or `UNAVAILABLE` and no `has_completed` value
- **VERIFIED BY**: `team-order/internal/rpc` `TestQ7HasCompletedOrder_StoreFailureIsNotReportedAsFalse`

#### Scenario: Logs carry no buyer id in any form

- **WHEN** the RPC runs for buyer A, on success and on failure
- **THEN** the system SHALL write no log line containing A's raw id or any fingerprint of it
- **VERIFIED BY**: `team-order/internal/rpc` `TestQ7HasCompletedOrder_LogsCarryNoBuyerIDInAnyForm`

### Requirement: HasCompletedOrder is restricted to SERVICE principals with order.read

`OrderService.HasCompletedOrder` SHALL be callable only by a SERVICE principal holding the `order.read` scope.
Unlike `canViewOrder`, the `order.admin` scope does not grant access here.

#### Scenario: Buyer principal refused

- **WHEN** a USER principal (the buyer themselves or a seller) calls `HasCompletedOrder`
- **THEN** the system SHALL fail with `PERMISSION_DENIED`

#### Scenario: Admin refused

- **WHEN** a USER principal holding `order.admin` calls `HasCompletedOrder`
- **THEN** the system SHALL fail with `PERMISSION_DENIED`

#### Scenario: Service without the scope refused

- **WHEN** a SERVICE principal without the `order.read` scope calls `HasCompletedOrder`
- **THEN** the system SHALL fail with `PERMISSION_DENIED`

#### Scenario: Unauthenticated call refused

- **WHEN** the call carries no principal
- **THEN** the system SHALL fail with `UNAUTHENTICATED`

### Requirement: HasCompletedOrder is not routed at team-gateway

team-gateway SHALL NOT forward `OrderService.HasCompletedOrder` to team-order.

#### Scenario: Not routed at the gateway

- **WHEN** any caller, an admin included, POSTs `/platform.order.v1.OrderService/HasCompletedOrder` to team-gateway
- **THEN** the gateway SHALL answer 501 unimplemented with team-order receiving 0 calls
