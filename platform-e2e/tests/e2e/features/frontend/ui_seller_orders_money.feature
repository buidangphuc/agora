@seller
Feature: Seller order handover, wallet, plans and bundles with the shared mutation contract
  OpenSpec change ui-phase-seller. Money uses the mock payment only. A slow server is
  simulated in the browser by delaying the Next server-action POST.

  Scenario: Shipping an order gives feedback
    Given a d2 seller has a listing and a paid order
    When the d2 seller opens the detail of their order
    And the d2 seller confirms the handover while the server is slow
    Then the confirm button is pending then disabled, a success toast appears and the tag and stepper show shipped

  Scenario: Shipping failure is reported
    Given a d2 seller has a listing and a paid order
    When the d2 seller opens the detail of their order
    And the order is shipped from another session
    And the d2 seller confirms the handover
    Then an error toast and an alert show the error and the order status is unchanged

  # @destructive: a payout needs proceeds older than the refund window, so it waits the window out
  # (needs payment-ledger.override.yaml, like payment/plp_payout_holdback.feature). Serial lane only.
  @destructive
  Scenario: Payout confirms, pends and toasts
    Given a d2 seller has a settled order and a positive wallet balance
    When the d2 seller opens the wallet
    And the d2 seller activates payout and confirms while the server is slow
    Then the confirm button is pending then disabled, a success toast appears and the balance and ledger refresh

  Scenario: Current plan cannot be re-subscribed
    Given a d2 seller has 0 published listings
    When the d2 seller opens the plans page
    Then the current plan shows a "Gói hiện tại" tag and a disabled button and the other plans an enabled subscribe button

  Scenario: Action errors use the error field
    Given a d2 seller has 2 published listings
    When the d2 seller opens the bundles page
    And the d2 seller submits a bundle "Combo lỗi" priced 90000 with 1 selected products
    Then the message shows in the form and an error toast appears

  Scenario: Bundle creation succeeds
    Given a d2 seller has 2 published listings
    When the d2 seller opens the bundles page
    And the d2 seller submits a bundle "Combo d2 ok" priced 90000 with 2 selected products
    Then a success toast appears, the form resets and the bundle is listed
