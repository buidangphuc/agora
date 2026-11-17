@fintech
Feature: GetPayoutObligations reports a seller's wallet money and pending payouts
  PaymentService.GetPayoutObligations is service-only (scope payment.internal.read) and not
  routed at the gateway. Credits are aged past the hold window with the short-window overlay
  platform-e2e/compose/payment-ledger.override.yaml (PAYOUT_HOLD_WINDOW=20s), like the other
  payout features. (change commerce-user-erasure)

  @destructive
  Scenario: PAY-1 Balance, hold and pending payouts are reported
    Given a seller "S" with a wallet credit that has aged past the hold window
    And "S" has a fresh wallet credit inside the hold window
    And "S" has 2 pending bank payouts of 50000
    When the identity service calls GetPayoutObligations for "S"
    Then the payout obligations are held the fresh credit, available the balance minus held, pending payouts 2

  Scenario: PAY-2 A buyer with no wallet reads zeros
    Given a user "N" with no orders, returns, wallet entries or payouts
    When the identity service calls GetPayoutObligations for "N"
    Then the payout obligations are zero

  Scenario: PAY-7 A user principal is refused
    Given a buyer "U"
    When a signed-in user principal calls GetPayoutObligations for "U"
    Then the call fails with "permission_denied"

  Scenario: PAY-8 A service principal without the scope is refused
    Given a buyer "U"
    When a service principal holding only order.read calls GetPayoutObligations for "U"
    Then the call fails with "permission_denied"

  Scenario: PAY-9 An anonymous caller is refused
    Given a buyer "U"
    When a call without a principal reaches GetPayoutObligations for "U"
    Then the call fails with "unauthenticated"

  Scenario: PAY-10 The gateway does not route the RPC
    Given a buyer "U"
    When a browser client calls PaymentService/GetPayoutObligations through the gateway with a token
    Then the gateway answers HTTP 501
    When a browser client calls PaymentService/GetPayoutObligations through the gateway without a token
    Then the gateway answers HTTP 501
