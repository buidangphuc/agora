@fintech
Feature: An admin refund is audited before money moves
  A refund made by a principal holding the admin scope writes one team-audit event
  (payment.admin_refund, target payment/<payment id>) and only then refunds; if the audit cannot be
  written the refund is refused with unavailable and nothing changes. A refund by the order's seller
  is neither audited nor dependent on team-audit. Refunds go through the gateway.
  (payment-admin-refund-audit)

  Scenario: Admin refund is audited end to end
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And a logged-in admin
    When the admin refunds 200000 of the payment with a fresh refund id
    Then the call succeeds
    And the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 200000
    And exactly one payment.admin_refund audit event exists for the payment with the admin as actor and metadata amount "200000" and the order id

  Scenario: Seller refund writes no audit event
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller refunds 200000 of the payment with a fresh refund id
    Then the call succeeds
    And the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 200000
    And no audit event exists for the payment

  # @destructive: stops team-audit for the duration of the scenario; serial lane only.
  @destructive
  Scenario: Audit down refuses the admin refund end to end
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And a logged-in admin
    And team-audit is stopped
    When the admin refunds 200000 of the payment with a fresh refund id
    Then the call fails with "unavailable"
    When team-audit is started again
    Then the payment read by "b1" reads PAID with a refunded amount of 0 and no refund
    And no audit event exists for the payment

  Scenario: Non-admin buyer cannot trigger an admin audit
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When "b1" refunds 200000 of the payment with a fresh refund id
    Then the call fails with "permission_denied"
    And the payment read by "b1" reads PAID with a refunded amount of 0 and no refund
    And no audit event exists for the payment

  # payment-refund-audit-id: the audit row carries the idempotency key of the refund.
  # Refund ids are global keys, so a label such as "rf-e2e-1" is expanded to a per-run unique id
  # ("rf-e2e-1-<hex>"); the steps assert against the expanded value.
  Scenario: Refund audit id: Admin refund audit row exposes refund_id end to end
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And a logged-in admin
    When the admin refunds 200000 of the payment with refund id "rf-e2e-1"
    Then the call succeeds
    And exactly one payment.admin_refund audit event exists for the payment with metadata refund id "rf-e2e-1" and amount "200000"

  Scenario: Refund audit id: Replayed refund_id is traceable end to end
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And a logged-in admin
    When the admin refunds 100000 of the payment with refund id "rf-e2e-2"
    And the admin refunds 100000 of the payment with refund id "rf-e2e-2"
    And the admin refunds 100000 of the payment with refund id "rf-e2e-3"
    Then the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 200000
    And every payment.admin_refund audit event for the payment carries a refund id of "rf-e2e-2" or "rf-e2e-3" and both appear

  Scenario: Refund audit id: Non-admin cannot read the refund audit row end to end
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And a logged-in admin
    When the admin refunds 200000 of the payment with refund id "rf-e2e-4"
    Then the call succeeds
    And exactly one payment.admin_refund audit event exists for the payment with metadata refund id "rf-e2e-4" and amount "200000"
    When "b1" queries the audit log for the payment
    Then the audit query is denied with "permission_denied" and returns no events
