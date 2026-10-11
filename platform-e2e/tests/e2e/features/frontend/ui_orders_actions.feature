@buyer @order
Feature: Buyer order row actions - reorder and cancel with pending feedback
  OpenSpec change ui-phase-orders. A slow server is simulated in the browser by delaying the
  Next server-action POST; failures are provoked by real stack state changes.

  Scenario: Reordering shows pending then success
    Given a d2 buyer has a single pending order
    When the d2 buyer opens the orders list
    And the d2 buyer presses "Mua lại" on the first order while the server is slow
    Then the reorder button is busy, disabled and keeps its width
    And a success toast shows and the buyer lands on the cart holding the order's item

  Scenario: Reorder failure is reported and recoverable
    Given a d2 buyer has a single pending order
    And the order's listing was deleted by its seller
    When the d2 buyer opens the orders list
    And the d2 buyer presses "Mua lại" on the first order while the server is slow
    Then an error toast shows the reason, the reorder button is enabled and the buyer stays on the list

  Scenario: Cancelling an order uses a confirmation Modal
    Given a d2 buyer has a single pending order
    When the d2 buyer opens the orders list
    And the d2 buyer opens the cancel Modal of the first order
    And the d2 buyer confirms the cancellation while the server is slow
    Then the confirm button is busy and the Modal cannot be dismissed
    And a success toast shows, the Modal closes and the order badge reads "Đã hủy"

  Scenario: Cancel failure keeps the Modal open
    Given a d2 buyer has a single pending order
    When the d2 buyer opens the orders list
    And the d2 buyer opens the cancel Modal of the first order
    And the order is completed by its seller behind the buyer's back
    And the d2 buyer confirms the cancellation
    Then an error toast shows, the Modal stays open and the confirm button is enabled
