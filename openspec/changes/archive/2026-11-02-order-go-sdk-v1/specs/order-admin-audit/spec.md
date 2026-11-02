## MODIFIED Requirements

### Requirement: Force-failing a saga is audited

`OrderService.ForceFailSaga` SHALL, after authorization (`admin` and `order.admin`), validation of `order_id` and `fail_step`, and loading the order, write one team-audit event before cancelling the order. The event SHALL have `action=order.force_fail`, `target_type=order`, `target_id` equal to the order id, `actor_id` equal to the caller's user id and `metadata` key `fail_step` ("payment", "shipping", or "unspecified" when empty). If the audit write fails or times out, the RPC SHALL return `Unavailable` and the order SHALL be unchanged.

#### Scenario: Order audit: Admin force-fails an order
- **WHEN** an admin holding `admin` and `order.admin` calls ForceFailSaga with fail_step "payment" on a Pending order
- **THEN** the call succeeds, the order is Cancelled, and exactly one `order.force_fail` event exists for the order with actor_id equal to the admin's user id and metadata `fail_step`="payment"
- **VERIFIED BY**: team-order `internal/rpc` `TestForceFailSagaWritesAuditEvent`

#### Scenario: Order audit: Empty fail step is recorded as unspecified
- **WHEN** an admin calls ForceFailSaga with an empty fail_step
- **THEN** the audit event metadata `fail_step` equals "unspecified"
- **VERIFIED BY**: team-order `internal/rpc` `TestForceFailSagaAuditEmptyStepUnspecified`

#### Scenario: Order audit: Force-fail refused while audit is down
- **WHEN** an admin calls ForceFailSaga while team-audit is unreachable
- **THEN** the call returns `Unavailable` and the order status read afterwards is still Pending
- **VERIFIED BY**: team-order `internal/rpc` `TestForceFailSagaAuditDownUnavailableNoChange`

#### Scenario: Order audit: Unauthorized force-fail writes no event
- **WHEN** a caller holding `admin` but not `order.admin`, a seller, and an unauthenticated caller each call ForceFailSaga
- **THEN** the calls return `PermissionDenied`, `PermissionDenied` and `Unauthenticated` respectively and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestForceFailSagaDeniedWritesNoAudit`

#### Scenario: Order audit: Invalid or unknown input writes no event
- **WHEN** an admin calls ForceFailSaga with fail_step "banana", then with an empty order_id, then with an unknown order id
- **THEN** the calls return `InvalidArgument`, `InvalidArgument` and `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestForceFailSagaInvalidInputWritesNoAudit`

#### Scenario: Order audit: Invalid transition keeps the audit row
- **WHEN** an admin force-fails an order that is already Completed and the audit write succeeds
- **THEN** the call returns `FailedPrecondition`, the order stays Completed and the audit call count is 1
- **VERIFIED BY**: team-order `internal/rpc` `TestForceFailSagaInvalidStatusAfterAudit`

#### Scenario: Order audit: Force-fail refused without an audit client
- **WHEN** team-order is started without `UPSTREAM_AUDIT_ADDR`, so that no admin `ForceFailSaga` call could run without an audit connection
- **THEN** startup is refused with an error naming `UPSTREAM_AUDIT_ADDR` (there is no audit-less mode, so the call can never run unaudited)
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

### Requirement: Admin order status updates are audited

`OrderService.UpdateOrderStatus` SHALL, when the caller holds `order.admin`, write one team-audit event after validation and loading the order and before applying the transition. The event SHALL have `action=order.admin_status_update`, `target_type=order`, `target_id` equal to the order id, `actor_id` equal to the caller's user id and `metadata` keys `seller_id` (the order's seller) and `to_status` (the requested status enum name). A caller without `order.admin` (a pure seller) SHALL cause no audit call and SHALL be unaffected by team-audit availability. When the audit write fails or times out on the admin path, the RPC SHALL return `Unavailable` and the order SHALL be unchanged.

#### Scenario: Order audit: Admin ships a seller's order
- **WHEN** an admin with `order.admin` calls UpdateOrderStatus to Shipped on a Paid order owned by seller "sa"
- **THEN** the call succeeds, the order is Shipped, and exactly one `order.admin_status_update` event exists for the order with metadata `seller_id`="sa" and `to_status`="ORDER_STATUS_SHIPPED"
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateOrderStatusAdminWritesAuditEvent`

#### Scenario: Order audit: Seller updates their own order unaudited
- **WHEN** the seller calls UpdateOrderStatus to Shipped on their own Paid order while team-audit is unreachable
- **THEN** the call succeeds, the order is Shipped and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateOrderStatusSellerNotAudited`

#### Scenario: Order audit: Order admin who is also the seller is audited
- **WHEN** a caller holding `order.admin` whose user id equals the order's seller id calls UpdateOrderStatus to Shipped on a Paid order
- **THEN** the call succeeds and exactly one `order.admin_status_update` audit call is made with actor_id equal to that user id
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateOrderStatusOrderAdminSellerIsAudited`

#### Scenario: Order audit: Status update refused while audit is down
- **WHEN** an admin calls UpdateOrderStatus to Shipped on a Paid order while team-audit is unreachable
- **THEN** the call returns `Unavailable` and the order status read afterwards is still Paid
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateOrderStatusAuditDownUnavailableNoChange`

#### Scenario: Order audit: Refused status calls write no event
- **WHEN** a buyer calls UpdateOrderStatus, an unauthenticated caller calls it, an admin passes an empty id, an admin passes an unspecified status, and an admin passes an unknown id
- **THEN** the calls return `PermissionDenied`, `Unauthenticated`, `InvalidArgument`, `InvalidArgument` and `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateOrderStatusRefusedWritesNoAudit`

#### Scenario: Order audit: Invalid transition by an admin keeps the audit row
- **WHEN** an admin requests Completed on a Paid order and the audit write succeeds
- **THEN** the call returns `FailedPrecondition`, the order stays Paid and the audit call count is 1
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateOrderStatusAdminInvalidTransitionAfterAudit`

#### Scenario: Order audit: Status update refused without an audit client
- **WHEN** team-order is started without `UPSTREAM_AUDIT_ADDR`, so that no admin `UpdateOrderStatus` call could run without an audit connection
- **THEN** startup is refused with an error naming `UPSTREAM_AUDIT_ADDR` (there is no audit-less mode, so the call can never run unaudited)
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

### Requirement: Admin return status updates are audited

`OrderService.UpdateReturnStatus` SHALL, when the caller holds `order.admin`, write one team-audit event after validation and loading the return request and before applying the transition. The event SHALL have `action=return.admin_status_update`, `target_type=return_request`, `target_id` equal to the return id, `actor_id` equal to the caller's user id and `metadata` keys `order_id` and `to_status` (the requested return status enum name). A caller without `order.admin` (a pure seller) SHALL cause no audit call. When the audit write fails or times out on the admin path, the RPC SHALL return `Unavailable` and the return request SHALL be unchanged and no `ReturnRefunded` fact SHALL be emitted.

#### Scenario: Order audit: Admin approves a return
- **WHEN** an admin with `order.admin` calls UpdateReturnStatus to Approved on a Requested return of order "o-1"
- **THEN** the call succeeds, the return is Approved, and exactly one `return.admin_status_update` event exists with target_type `return_request`, metadata `order_id`="o-1" and `to_status`="RETURN_STATUS_APPROVED"
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateReturnStatusAdminWritesAuditEvent`

#### Scenario: Order audit: Seller updates their own return unaudited
- **WHEN** the seller calls UpdateReturnStatus to Approved on a return of their own order while team-audit is unreachable
- **THEN** the call succeeds and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateReturnStatusSellerNotAudited`

#### Scenario: Order audit: Return update refused while audit is down
- **WHEN** an admin calls UpdateReturnStatus to Refunded on an Approved return while team-audit is unreachable
- **THEN** the call returns `Unavailable`, the return stays Approved and no ReturnRefunded event is enqueued
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateReturnStatusAuditDownUnavailableNoChange`

#### Scenario: Order audit: Refused return calls write no event
- **WHEN** a buyer calls UpdateReturnStatus, an unauthenticated caller calls it, an admin passes an empty id, an admin passes an unspecified status, and an admin passes an unknown id
- **THEN** the calls return `PermissionDenied`, `Unauthenticated`, `InvalidArgument`, `InvalidArgument` and `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateReturnStatusRefusedWritesNoAudit`

#### Scenario: Order audit: Invalid return transition by an admin keeps the audit row
- **WHEN** an admin requests Refunded on a Requested return and the audit write succeeds
- **THEN** the call returns `FailedPrecondition`, the return stays Requested and the audit call count is 1
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateReturnStatusAdminInvalidTransitionAfterAudit`

#### Scenario: Order audit: Return update refused without an audit client
- **WHEN** team-order is started without `UPSTREAM_AUDIT_ADDR`, so that no admin `UpdateReturnStatus` call could run without an audit connection
- **THEN** startup is refused with an error naming `UPSTREAM_AUDIT_ADDR` (there is no audit-less mode, so the call can never run unaudited)
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

### Requirement: Admin shipment creation is audited

`OrderService.CreateShipment` SHALL, when the caller holds `order.admin`, write one team-audit event after validation and loading the order and before creating the shipment. The event SHALL have `action=shipment.admin_create`, `target_type=order`, `target_id` equal to the order id, `actor_id` equal to the caller's user id and `metadata` key `carrier` (the effective carrier, "SPX" when empty, truncated to 64 runes). The tracking code SHALL NOT be recorded. A caller without `order.admin` (a pure seller) SHALL cause no audit call. When the audit write fails or times out on the admin path, the RPC SHALL return `Unavailable` and no shipment SHALL be created.

#### Scenario: Order audit: Admin creates a shipment
- **WHEN** an admin with `order.admin` calls CreateShipment with carrier "SPX" on a Paid order of seller "sa"
- **THEN** the call succeeds and exactly one `shipment.admin_create` event exists for the order with metadata `carrier`="SPX" and no tracking code key
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateShipmentAdminWritesAuditEvent`

#### Scenario: Order audit: Seller creates a shipment unaudited
- **WHEN** the seller calls CreateShipment on their own Paid order while team-audit is unreachable
- **THEN** the call succeeds and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateShipmentSellerNotAudited`

#### Scenario: Order audit: Shipment refused while audit is down
- **WHEN** an admin calls CreateShipment on a Paid order while team-audit is unreachable
- **THEN** the call returns `Unavailable`, no shipment exists for the order and the order status is unchanged
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateShipmentAuditDownUnavailableNoChange`

#### Scenario: Order audit: Long carrier name is truncated in the audit event
- **WHEN** an admin creates a shipment with a 100-character carrier name
- **THEN** the audit metadata `carrier` holds the first 64 characters and the call succeeds
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateShipmentAuditCarrierTruncated`

#### Scenario: Order audit: Refused shipment calls write no event
- **WHEN** a buyer calls CreateShipment, an unauthenticated caller calls it, an admin passes an empty order_id, and an admin passes an unknown order id
- **THEN** the calls return `PermissionDenied`, `Unauthenticated`, `InvalidArgument` and `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateShipmentRefusedWritesNoAudit`

#### Scenario: Order audit: Shipment on a Cancelled order keeps the audit row
- **WHEN** an admin creates a shipment for a Cancelled order (not shippable; a Pending order is shippable by COD hand-over) and the audit write succeeds
- **THEN** the call returns `FailedPrecondition`, no shipment exists and the audit call count is 1
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateShipmentAdminInvalidStatusAfterAudit`

#### Scenario: Order audit: Shipment refused without an audit client
- **WHEN** team-order is started without `UPSTREAM_AUDIT_ADDR`, so that no admin `CreateShipment` call could run without an audit connection
- **THEN** startup is refused with an error naming `UPSTREAM_AUDIT_ADDR` (there is no audit-less mode, so the call can never run unaudited)
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

### Requirement: team-order writes audit events as its own service principal

team-order SHALL call `AuditService.WriteAuditEvent` through an `auditx.Writer` in `internal/upstream/audit.go` over the connection from `auditx.Dial(s.Audit, "service-team-order", "audit.write")`, with outgoing metadata `x-principal-id=service-team-order`, `x-principal-type=service` and `x-principal-scopes=audit.write` only, replacing any caller principal, bounded by the SDK write timeout of 2 seconds, without retry. `UPSTREAM_AUDIT_ADDR` SHALL be required in every ENV: team-order has no mode without an audit connection, so the audit gate always has a writer and an admin-path call fails with `Unavailable` and the message `audit trail unavailable` only when the audit write fails or times out.

#### Scenario: Order audit: Audit call carries the service principal only
- **WHEN** the audit writer sends WriteAuditEvent from a context carrying an admin's principal metadata
- **THEN** the outgoing metadata holds exactly principal id `service-team-order`, type `service` and scopes `audit.write`, and the admin's principal metadata is absent
- **VERIFIED BY**: team-order `internal/upstream` `TestAuditWriterUsesServicePrincipal`

#### Scenario: Order audit: Audit call is bounded by the timeout
- **WHEN** team-audit does not answer
- **THEN** the gate returns `Unavailable` within 3 seconds and the writer made exactly one attempt
- **VERIFIED BY**: team-order `internal/upstream` `TestAuditWriterTimeoutNoRetry`

#### Scenario: Order audit: Unset audit address yields no client
- **WHEN** team-order loads settings with `UPSTREAM_AUDIT_ADDR` unset in any ENV
- **THEN** the load returns an error naming `UPSTREAM_AUDIT_ADDR`, so no audit writer-less process can start
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

#### Scenario: Order audit: Config exposes the audit address
- **WHEN** team-order loads settings with `ENV=local` and `UPSTREAM_AUDIT_ADDR` unset, then with `ENV=local` and the address set to "team-audit-svc:50066"
- **THEN** the first load returns an error naming `UPSTREAM_AUDIT_ADDR` and the second returns `Audit.Addr` equal to "team-audit-svc:50066"
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAuditConfig`

#### Scenario: Order audit: Audit proto copy is current
- **WHEN** the team-order proto copy of `platform/audit/v1/audit.proto` is compared with `platform-core/packages/proto`
- **THEN** the two files are byte-identical and `buf generate` produces `AuditServiceClient` with `WriteAuditEvent`
- **VERIFIED BY**: team-order `internal/upstream` `TestAuditProtoCopyMatchesCore`
