## ADDED Requirements

### Requirement: GetUserObligations counts a user's open orders and returns

`OrderService.GetUserObligations` SHALL return, for the requested `user_id`, `open_buyer_orders` (orders with that
buyer in status PENDING, PAID or SHIPPED), `open_seller_orders` (the same statuses with that seller) and
`open_returns` (returns of status PENDING or APPROVED where the user is buyer or seller). It SHALL accept only a
SERVICE principal with scope `order.internal.read` and SHALL NOT be routed at the gateway.

#### Scenario: OBL-1 Open orders and returns are counted
- **GIVEN** user U is buyer of 2 PENDING orders and 1 SHIPPED order, seller of 1 PAID order, and buyer of 1 PENDING return
- **WHEN** a SERVICE principal with scope `order.internal.read` calls `GetUserObligations` for U
- **THEN** the response is `open_buyer_orders` 3, `open_seller_orders` 1, `open_returns` 1

#### Scenario: OBL-2 Closed orders and returns are not counted
- **GIVEN** U has only COMPLETED and CANCELLED orders and returns in REJECTED and REFUNDED status
- **WHEN** the service calls `GetUserObligations` for U
- **THEN** the response is `open_buyer_orders` 0, `open_seller_orders` 0, `open_returns` 0

#### Scenario: OBL-3 A user with no data reads zeros
- **WHEN** the service calls `GetUserObligations` for a user id that appears in no order or return
- **THEN** the response is OK with all three counts 0

#### Scenario: OBL-4 A return counts for both parties
- **GIVEN** an APPROVED return whose buyer is U and whose seller is V
- **WHEN** the service calls `GetUserObligations` for U and then for V
- **THEN** each response has `open_returns` 1
- **VERIFIED BY**: team-order/internal/store TestUserObligationsCounts

#### Scenario: OBL-5 Another user's orders are not counted
- **GIVEN** U has 1 PAID buyer order and W has 4 PENDING buyer orders
- **WHEN** the service calls `GetUserObligations` for U
- **THEN** `open_buyer_orders` is 1
- **VERIFIED BY**: team-order/internal/store TestUserObligationsCounts

#### Scenario: OBL-6 An empty user id is rejected
- **WHEN** the service calls `GetUserObligations` with `user_id` ""
- **THEN** the call fails with `INVALID_ARGUMENT` and no query runs
- **VERIFIED BY**: team-order/internal/rpc TestGetUserObligationsRejectsEmptyUserID

#### Scenario: OBL-7 A user principal is refused
- **WHEN** a signed-in USER principal calls `GetUserObligations` for its own id
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: OBL-8 A service principal without the scope is refused
- **WHEN** a SERVICE principal holding only `order.read` calls `GetUserObligations`
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: OBL-9 An anonymous caller is refused
- **WHEN** a call without a principal reaches `GetUserObligations`
- **THEN** the call fails with `UNAUTHENTICATED`

#### Scenario: OBL-10 The gateway does not route the RPC
- **WHEN** a browser client calls `OrderService/GetUserObligations` through the gateway, with or without a token
- **THEN** the gateway answers HTTP 501 and team-order receives no request

#### Scenario: OBL-11 A store failure never reads as zero obligations
- **GIVEN** the order database is unreachable
- **WHEN** the service calls `GetUserObligations` for U
- **THEN** the call fails with a non-OK gRPC status and the response carries no counts
- **VERIFIED BY**: team-order/internal/rpc TestGetUserObligationsStoreFailure

