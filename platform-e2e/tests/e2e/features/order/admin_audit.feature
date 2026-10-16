@order
Feature: Admin actions on orders are written to the audit log
  ForceFailSaga, UpdateOrderStatus, UpdateReturnStatus and CreateShipment on the order.admin path each
  write one team-audit event (queried back through the gateway), a pure seller's own calls write none,
  and the admin path fails closed (unavailable) while team-audit is down.
  (openspec change order-admin-audit, initiative q3-audit-log; scenario names echo the spec titles)

  Scenario: Order audit: Admin force-fail appears in the audit log end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 1 of "L"
    And the seeded order admin
    When the admin calls ForceFailSaga with fail_step "payment" on the order
    Then the call succeeds
    And the order read by "b1" is Cancelled
    And exactly one "order.force_fail" audit event exists for the order with metadata fail_step="payment"

  Scenario: Order audit: Admin status update appears in the audit log end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 1 of "L"
    And the seeded order admin
    When "admin" calls UpdateOrderStatus to Shipped on the order
    Then the call succeeds
    And the order read by "b1" is Shipped
    And exactly one "order.admin_status_update" audit event exists for the order with metadata seller_id=<seller "sa" id>, to_status="ORDER_STATUS_SHIPPED"

  Scenario: Order audit: Admin return update appears in the audit log end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 1 of "L"
    And "b1" has a Requested return on the order
    And the seeded order admin
    When the admin approves the return
    Then the call succeeds
    And exactly one "return.admin_status_update" audit event exists for the return with metadata order_id=<order id>, to_status="RETURN_STATUS_APPROVED"

  Scenario: Order audit: Admin shipment appears in the audit log end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 1 of "L"
    And the seeded order admin
    When the admin creates a shipment with carrier "SPX" for the order
    Then the call succeeds
    And exactly one "shipment.admin_create" audit event exists for the order with metadata carrier="SPX"
    And that audit event carries only the metadata key "carrier"

  Scenario: Order audit: Seller actions are not audited end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 1 of "L"
    When "sa" calls UpdateOrderStatus to Shipped on the order
    Then the call succeeds
    And the order read by "b1" is Shipped
    And no audit event exists for the order

  # @destructive: stops team-audit (docker stop); the world cleanup starts it again. Run only in the
  # destructive lane, never in parallel with other audit-reading scenarios.
  @destructive
  Scenario: Order audit: Admin action is refused while team-audit is down end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 1 of "L"
    And the seeded order admin
    And team-audit is stopped
    When "admin" calls UpdateOrderStatus to Shipped on the order
    Then the call fails with "unavailable"
    When team-audit is running again
    Then the order read by "b1" is Paid
    And no audit event exists for the order

  Scenario: Order audit: Buyer cannot use the admin path end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 1 of "L"
    When "b1" calls UpdateOrderStatus to Shipped on the order
    Then the call fails with "permission_denied"
    And the order read by "b1" remains Paid
    And no audit event exists for the order
