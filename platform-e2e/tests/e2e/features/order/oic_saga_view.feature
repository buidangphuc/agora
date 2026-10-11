@order
Feature: The saga view and ForceFailSaga describe what actually happened to an order
  GetSagaState is built from persisted order, reservation and payment facts, and ForceFailSaga
  validates its step and answers honestly.
  (port-order-inventory-correctness / order-read-access)

  Scenario: A pending order's saga view shows payment pending
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When "b1" reads the saga view of the order
    Then saga step 1 is SUCCESS
    And saga step 2 is SUCCESS
    And saga step 3 is PENDING
    And saga step 4 is PENDING
    And the saga view is not compensated

  Scenario: A paid order's saga view shows the payment time
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 2 of "L"
    When "b1" reads the saga view of the order
    Then saga step 3 is SUCCESS
    And saga step 3 carries a timestamp
    And the saga view reports no compensation

  Scenario: A cancelled order with released stock shows completed compensation
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When "b1" cancels the order
    And "b1" reads the saga view of the order
    Then saga step 3 is SKIPPED
    And saga step 4 is COMPENSATED
    And the saga view is compensated

  # @destructive: stops team-domain (docker stop) so the stock release is parked for retry; relies on the
  # short-TTL overlay platform-e2e/compose/order-inventory.override.yaml (RESERVATION_TTL=20s,
  # RESERVATION_SWEEP_INTERVAL=2s) so the sweep settles the parked release soon after team-domain is back.
  @destructive
  Scenario: A cancelled order awaiting a stock release shows compensation pending
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    And team-domain is stopped
    When "b1" cancels the order
    And "b1" reads the saga view of the order
    Then saga step 4 is PENDING
    And the saga view is not compensated

  Scenario: An unknown fail_step is rejected
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When the admin calls ForceFailSaga with fail_step "banana" on the order
    Then the call fails with "invalid_argument"
    And the order read by "b1" remains Pending
    And the stock of "L" is unchanged at 8

  Scenario: A clean force-fail reports success
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When the admin calls ForceFailSaga with fail_step "payment" on the order
    Then ForceFailSaga reports success
    And the order read by "b1" is Cancelled
    And the returned saga view is compensated
    And the stock of "L" is back to 10

  # @destructive: stops team-domain (docker stop); relies on the short-TTL overlay
  # platform-e2e/compose/order-inventory.override.yaml (RESERVATION_TTL=20s, RESERVATION_SWEEP_INTERVAL=2s).
  @destructive
  Scenario: A force-fail with a parked release reports failure
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    And team-domain is stopped
    When the admin calls ForceFailSaga with fail_step "" on the order
    Then ForceFailSaga reports failure with a message saying the stock release is pending retry
    And the order read by "b1" is Cancelled

  Scenario: Force-failing a shipped order is refused
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Shipped order for 2 of "L"
    When the admin calls ForceFailSaga with fail_step "payment" on the order
    Then the call fails with "failed_precondition"
    And the order read by "b1" remains Shipped
