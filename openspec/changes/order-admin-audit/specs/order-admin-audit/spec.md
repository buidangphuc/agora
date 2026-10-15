## ADDED Requirements

### Requirement: Force-failing a saga is audited

`OrderService.ForceFailSaga` SHALL, after authorization (`admin` and `order.admin`), validation of `order_id` and `fail_step`, and loading the order, write one team-audit event before cancelling the order. The event SHALL have `action=order.force_fail`, `target_type=order`, `target_id` equal to the order id, `actor_id` equal to the caller's user id and `metadata` key `fail_step` ("payment", "shipping", or "unspecified" when empty). If the audit write fails, times out or no audit client is configured, the RPC SHALL return `Unavailable` and the order SHALL be unchanged.

#### Scenario: Order audit: Admin force-fails an order
- **WHEN** an admin holding `admin` and `order.admin` calls ForceFailSaga with fail_step "payment" on a Pending order
- **THEN** the call succeeds, the order is Cancelled, and exactly one `order.force_fail` event exists for the order with actor_id equal to the admin's user id and metadata `fail_step`="payment"
- **VERIFIED BY**: team-order `internal/handler` `TestForceFailSagaWritesAuditEvent`

#### Scenario: Order audit: Empty fail step is recorded as unspecified
- **WHEN** an admin calls ForceFailSaga with an empty fail_step
- **THEN** the audit event metadata `fail_step` equals "unspecified"
- **VERIFIED BY**: team-order `internal/handler` `TestForceFailSagaAuditEmptyStepUnspecified`

#### Scenario: Order audit: Force-fail refused while audit is down
- **WHEN** an admin calls ForceFailSaga while team-audit is unreachable
- **THEN** the call returns `Unavailable` and the order status read afterwards is still Pending
- **VERIFIED BY**: team-order `internal/handler` `TestForceFailSagaAuditDownUnavailableNoChange`

#### Scenario: Order audit: Force-fail refused without an audit client
- **WHEN** ForceFailSaga runs in a team-order started without `UPSTREAM_AUDIT_ADDR`
- **THEN** the call returns `Unavailable` and no order is cancelled
- **VERIFIED BY**: team-order `internal/handler` `TestForceFailSagaNilAuditWriterUnavailable`

#### Scenario: Order audit: Unauthorized force-fail writes no event
- **WHEN** a caller holding `admin` but not `order.admin`, a seller, and an unauthenticated caller each call ForceFailSaga
- **THEN** the calls return `PermissionDenied`, `PermissionDenied` and `Unauthenticated` respectively and zero audit calls are made
- **VERIFIED BY**: team-order `internal/handler` `TestForceFailSagaDeniedWritesNoAudit`

#### Scenario: Order audit: Invalid or unknown input writes no event
- **WHEN** an admin calls ForceFailSaga with fail_step "banana", then with an empty order_id, then with an unknown order id
- **THEN** the calls return `InvalidArgument`, `InvalidArgument` and `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-order `internal/handler` `TestForceFailSagaInvalidInputWritesNoAudit`

#### Scenario: Order audit: Invalid transition keeps the audit row
- **WHEN** an admin force-fails an order that is already Completed and the audit write succeeds
- **THEN** the call returns `FailedPrecondition`, the order stays Completed and the audit call count is 1
- **VERIFIED BY**: team-order `internal/handler` `TestForceFailSagaInvalidStatusAfterAudit`

### Requirement: Admin order status updates are audited

`OrderService.UpdateOrderStatus` SHALL, when the caller holds `order.admin`, write one team-audit event after validation and loading the order and before applying the transition. The event SHALL have `action=order.admin_status_update`, `target_type=order`, `target_id` equal to the order id, `actor_id` equal to the caller's user id and `metadata` keys `seller_id` (the order's seller) and `to_status` (the requested status enum name). A caller without `order.admin` (a pure seller) SHALL cause no audit call and SHALL be unaffected by team-audit availability. When the audit write fails, times out or no client is configured on the admin path, the RPC SHALL return `Unavailable` and the order SHALL be unchanged.

#### Scenario: Order audit: Admin ships a seller's order
- **WHEN** an admin with `order.admin` calls UpdateOrderStatus to Shipped on a Paid order owned by seller "sa"
- **THEN** the call succeeds, the order is Shipped, and exactly one `order.admin_status_update` event exists for the order with metadata `seller_id`="sa" and `to_status`="ORDER_STATUS_SHIPPED"
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateOrderStatusAdminWritesAuditEvent`

#### Scenario: Order audit: Seller updates their own order unaudited
- **WHEN** the seller calls UpdateOrderStatus to Shipped on their own Paid order while team-audit is unreachable
- **THEN** the call succeeds, the order is Shipped and zero audit calls are made
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateOrderStatusSellerNotAudited`

#### Scenario: Order audit: Order admin who is also the seller is audited
- **WHEN** a caller holding `order.admin` whose user id equals the order's seller id calls UpdateOrderStatus to Shipped on a Paid order
- **THEN** the call succeeds and exactly one `order.admin_status_update` audit call is made with actor_id equal to that user id
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateOrderStatusOrderAdminSellerIsAudited`

#### Scenario: Order audit: Status update refused while audit is down
- **WHEN** an admin calls UpdateOrderStatus to Shipped on a Paid order while team-audit is unreachable
- **THEN** the call returns `Unavailable` and the order status read afterwards is still Paid
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateOrderStatusAuditDownUnavailableNoChange`

#### Scenario: Order audit: Status update refused without an audit client
- **WHEN** an admin calls UpdateOrderStatus in a team-order started without `UPSTREAM_AUDIT_ADDR`
- **THEN** the call returns `Unavailable` and the order is unchanged
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateOrderStatusNilAuditWriterUnavailable`

#### Scenario: Order audit: Refused status calls write no event
- **WHEN** a buyer calls UpdateOrderStatus, an unauthenticated caller calls it, an admin passes an empty id, an admin passes an unspecified status, and an admin passes an unknown id
- **THEN** the calls return `PermissionDenied`, `Unauthenticated`, `InvalidArgument`, `InvalidArgument` and `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateOrderStatusRefusedWritesNoAudit`

#### Scenario: Order audit: Invalid transition by an admin keeps the audit row
- **WHEN** an admin requests Completed on a Paid order and the audit write succeeds
- **THEN** the call returns `FailedPrecondition`, the order stays Paid and the audit call count is 1
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateOrderStatusAdminInvalidTransitionAfterAudit`

### Requirement: Admin return status updates are audited

`OrderService.UpdateReturnStatus` SHALL, when the caller holds `order.admin`, write one team-audit event after validation and loading the return request and before applying the transition. The event SHALL have `action=return.admin_status_update`, `target_type=return_request`, `target_id` equal to the return id, `actor_id` equal to the caller's user id and `metadata` keys `order_id` and `to_status` (the requested return status enum name). A caller without `order.admin` (a pure seller) SHALL cause no audit call. When the audit write fails, times out or no client is configured on the admin path, the RPC SHALL return `Unavailable` and the return request SHALL be unchanged and no `ReturnRefunded` fact SHALL be emitted.

#### Scenario: Order audit: Admin approves a return
- **WHEN** an admin with `order.admin` calls UpdateReturnStatus to Approved on a Requested return of order "o-1"
- **THEN** the call succeeds, the return is Approved, and exactly one `return.admin_status_update` event exists with target_type `return_request`, metadata `order_id`="o-1" and `to_status`="RETURN_STATUS_APPROVED"
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateReturnStatusAdminWritesAuditEvent`

#### Scenario: Order audit: Seller updates their own return unaudited
- **WHEN** the seller calls UpdateReturnStatus to Approved on a return of their own order while team-audit is unreachable
- **THEN** the call succeeds and zero audit calls are made
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateReturnStatusSellerNotAudited`

#### Scenario: Order audit: Return update refused while audit is down
- **WHEN** an admin calls UpdateReturnStatus to Refunded on an Approved return while team-audit is unreachable
- **THEN** the call returns `Unavailable`, the return stays Approved and no ReturnRefunded event is enqueued
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateReturnStatusAuditDownUnavailableNoChange`

#### Scenario: Order audit: Return update refused without an audit client
- **WHEN** an admin calls UpdateReturnStatus in a team-order started without `UPSTREAM_AUDIT_ADDR`
- **THEN** the call returns `Unavailable` and the return is unchanged
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateReturnStatusNilAuditWriterUnavailable`

#### Scenario: Order audit: Refused return calls write no event
- **WHEN** a buyer calls UpdateReturnStatus, an unauthenticated caller calls it, an admin passes an empty id, an admin passes an unspecified status, and an admin passes an unknown id
- **THEN** the calls return `PermissionDenied`, `Unauthenticated`, `InvalidArgument`, `InvalidArgument` and `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateReturnStatusRefusedWritesNoAudit`

#### Scenario: Order audit: Invalid return transition by an admin keeps the audit row
- **WHEN** an admin requests Refunded on a Requested return and the audit write succeeds
- **THEN** the call returns `FailedPrecondition`, the return stays Requested and the audit call count is 1
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateReturnStatusAdminInvalidTransitionAfterAudit`

### Requirement: Admin shipment creation is audited

`OrderService.CreateShipment` SHALL, when the caller holds `order.admin`, write one team-audit event after validation and loading the order and before creating the shipment. The event SHALL have `action=shipment.admin_create`, `target_type=order`, `target_id` equal to the order id, `actor_id` equal to the caller's user id and `metadata` key `carrier` (the effective carrier, "SPX" when empty, truncated to 64 runes). The tracking code SHALL NOT be recorded. A caller without `order.admin` (a pure seller) SHALL cause no audit call. When the audit write fails, times out or no client is configured on the admin path, the RPC SHALL return `Unavailable` and no shipment SHALL be created.

#### Scenario: Order audit: Admin creates a shipment
- **WHEN** an admin with `order.admin` calls CreateShipment with carrier "SPX" on a Paid order of seller "sa"
- **THEN** the call succeeds and exactly one `shipment.admin_create` event exists for the order with metadata `carrier`="SPX" and no tracking code key
- **VERIFIED BY**: team-order `internal/handler` `TestCreateShipmentAdminWritesAuditEvent`

#### Scenario: Order audit: Seller creates a shipment unaudited
- **WHEN** the seller calls CreateShipment on their own Paid order while team-audit is unreachable
- **THEN** the call succeeds and zero audit calls are made
- **VERIFIED BY**: team-order `internal/handler` `TestCreateShipmentSellerNotAudited`

#### Scenario: Order audit: Shipment refused while audit is down
- **WHEN** an admin calls CreateShipment on a Paid order while team-audit is unreachable
- **THEN** the call returns `Unavailable`, no shipment exists for the order and the order status is unchanged
- **VERIFIED BY**: team-order `internal/handler` `TestCreateShipmentAuditDownUnavailableNoChange`

#### Scenario: Order audit: Shipment refused without an audit client
- **WHEN** an admin calls CreateShipment in a team-order started without `UPSTREAM_AUDIT_ADDR`
- **THEN** the call returns `Unavailable` and no shipment is created
- **VERIFIED BY**: team-order `internal/handler` `TestCreateShipmentNilAuditWriterUnavailable`

#### Scenario: Order audit: Long carrier name is truncated in the audit event
- **WHEN** an admin creates a shipment with a 100-character carrier name
- **THEN** the audit metadata `carrier` holds the first 64 characters and the call succeeds
- **VERIFIED BY**: team-order `internal/handler` `TestCreateShipmentAuditCarrierTruncated`

#### Scenario: Order audit: Refused shipment calls write no event
- **WHEN** a buyer calls CreateShipment, an unauthenticated caller calls it, an admin passes an empty order_id, and an admin passes an unknown order id
- **THEN** the calls return `PermissionDenied`, `Unauthenticated`, `InvalidArgument` and `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-order `internal/handler` `TestCreateShipmentRefusedWritesNoAudit`

#### Scenario: Order audit: Shipment on a Cancelled order keeps the audit row
- **WHEN** an admin creates a shipment for a Cancelled order (not shippable; a Pending order is shippable by COD hand-over) and the audit write succeeds
- **THEN** the call returns `FailedPrecondition`, no shipment exists and the audit call count is 1
- **VERIFIED BY**: team-order `internal/handler` `TestCreateShipmentAdminInvalidStatusAfterAudit`

### Requirement: team-order writes audit events as its own service principal

team-order SHALL call `AuditService.WriteAuditEvent` through `internal/upstream/audit.go` with outgoing metadata `x-principal-id=service-team-order`, `x-principal-type=service` and `x-principal-scopes=audit.write` only, replacing any caller principal, bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS`, without retry. An empty `UPSTREAM_AUDIT_ADDR` SHALL yield no client, and the handler SHALL then fail admin-path calls with `Unavailable` while seller-path calls continue to work.

#### Scenario: Order audit: Audit call carries the service principal only
- **WHEN** the audit client sends WriteAuditEvent from a context carrying an admin's principal metadata
- **THEN** the outgoing metadata holds exactly principal id `service-team-order`, type `service` and scopes `audit.write`, and the admin's principal metadata is absent
- **VERIFIED BY**: team-order `internal/upstream` `TestAuditClientUsesServicePrincipal`

#### Scenario: Order audit: Audit call is bounded by the timeout
- **WHEN** team-audit does not answer within `UPSTREAM_CALL_TIMEOUT_SECONDS`
- **THEN** the client returns an error within the timeout plus 500 ms and makes exactly one attempt
- **VERIFIED BY**: team-order `internal/upstream` `TestAuditClientTimeoutNoRetry`

#### Scenario: Order audit: Unset audit address yields no client
- **WHEN** `NewAuditClient` is called with an empty address
- **THEN** it returns a nil client and a nil error, and `Close` on the nil client returns nil
- **VERIFIED BY**: team-order `internal/upstream` `TestNewAuditClientEmptyAddrNil`

#### Scenario: Order audit: Config exposes the audit address
- **WHEN** team-order loads settings with `UPSTREAM_AUDIT_ADDR` unset, then set to "team-audit-svc:50066"
- **THEN** `Upstream.AuditAddr` equals "" and then "team-audit-svc:50066", and `Upstream.CallTimeoutSeconds` defaults to 2
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAuditConfig`

#### Scenario: Order audit: Audit proto copy is current
- **WHEN** the team-order proto copy of `platform/audit/v1/audit.proto` is compared with `platform-core/packages/proto`
- **THEN** the two files are byte-identical and `buf generate` produces `AuditServiceClient` with `WriteAuditEvent`
- **VERIFIED BY**: team-order `internal/upstream` `TestAuditProtoCopyMatchesCore`

### Requirement: Admin order actions are visible in the audit log end to end

Through the gateway with a real team-audit, each of the four audited admin actions SHALL produce exactly one event retrievable by `AuditService.QueryAuditLog` filtered by `target_type` and `target_id`, a pure seller performing the same call SHALL produce none, and while team-audit is stopped the admin action SHALL return `Unavailable` and leave the order unchanged.

#### Scenario: Order audit: Admin force-fail appears in the audit log end to end
- **WHEN** the seeded admin force-fails a buyer's Pending order and then queries the audit log for target_type `order` and that order id
- **THEN** exactly one event has action `order.force_fail`, actor_id equal to the admin's user id and metadata `fail_step`
- **VERIFIED BY**: platform-e2e `features/order/admin_audit.feature` "Admin force-fail is audited"

#### Scenario: Order audit: Admin status update appears in the audit log end to end
- **WHEN** the seeded admin moves a seller's Paid order to Shipped and then queries the audit log for that order
- **THEN** exactly one event has action `order.admin_status_update` with metadata `seller_id` equal to the seller's user id and `to_status`="ORDER_STATUS_SHIPPED", and the order reads as Shipped
- **VERIFIED BY**: platform-e2e `features/order/admin_audit.feature` "Admin order status update is audited"

#### Scenario: Order audit: Admin return update appears in the audit log end to end
- **WHEN** the seeded admin approves a buyer's Requested return and then queries the audit log for target_type `return_request` and the return id
- **THEN** exactly one event has action `return.admin_status_update` with metadata `to_status`="RETURN_STATUS_APPROVED" and `order_id`
- **VERIFIED BY**: platform-e2e `features/order/admin_audit.feature` "Admin return status update is audited"

#### Scenario: Order audit: Admin shipment appears in the audit log end to end
- **WHEN** the seeded admin creates a shipment with carrier "SPX" on a Paid order and then queries the audit log for that order and action `shipment.admin_create`
- **THEN** exactly one event is returned with metadata `carrier`="SPX"
- **VERIFIED BY**: platform-e2e `features/order/admin_audit.feature` "Admin shipment creation is audited"

#### Scenario: Order audit: Seller actions are not audited end to end
- **WHEN** the seller ships their own Paid order through UpdateOrderStatus and then queries the audit log for target_type `order` and that order id
- **THEN** the order reads as Shipped and zero events are returned
- **VERIFIED BY**: platform-e2e `features/order/admin_audit.feature` "A seller's own order update is not audited"

#### Scenario: Order audit: Admin action is refused while team-audit is down end to end
- **WHEN** the scenario (tagged `@destructive`) calls `stop_audit`, then the seeded admin calls UpdateOrderStatus to Shipped on a seller's Paid order
- **THEN** the call fails with `unavailable`; after `start_audit` the order read by the buyer is Paid and `assert_no_event` holds for the order
- **VERIFIED BY**: platform-e2e `features/order/admin_audit.feature` "Admin status update is refused while audit is down"

#### Scenario: Order audit: Buyer cannot use the admin path end to end
- **WHEN** a buyer calls UpdateOrderStatus to Shipped on their own Paid order
- **THEN** the call fails with `permission_denied`, the order remains Paid and zero audit events exist for the order
- **VERIFIED BY**: platform-e2e `features/order/admin_audit.feature` "A buyer cannot ship an order and nothing is audited"
