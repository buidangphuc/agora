@buyer @order
Feature: Buyer returns - request Modal, validation, failure and status badges
  OpenSpec change ui-phase-orders. Paid orders use the mock payment; the seller moves the
  return through the gateway. The slow server is simulated by delaying the server action.

  Scenario: Submitting a valid return
    Given a d2 buyer has a completed paid order
    When the d2 buyer opens the order detail
    And the d2 buyer opens the return Modal
    And the d2 buyer chooses the reason "changed_mind" and submits while the server is slow
    Then the submit button is busy and the fields are disabled
    And the Modal closes, a success toast shows and the returns tab shows a pending badge

  Scenario: Validation blocks an invalid form
    Given a d2 buyer has a completed paid order
    When the d2 buyer opens the order detail
    And the d2 buyer opens the return Modal
    And the d2 buyer submits the return form without a reason
    Then the reason form item shows an inline error, no action was sent and the Modal stays open
    When the d2 buyer submits the return form with an amount above the order total
    Then the amount form item shows an inline error, no action was sent and the Modal stays open

  Scenario: A failed submission keeps the Modal open
    Given a d2 buyer has a completed paid order
    When the d2 buyer opens the order detail
    And the d2 buyer opens the return Modal
    And a return for the full amount is filed from another session
    And the d2 buyer chooses the reason "defective" and submits
    Then an error toast shows the error, the Modal stays open and the submit button is enabled

  Scenario: Return statuses use the same component
    Given a d2 buyer has a refunded return and a rejected return on two completed orders
    Then the refunded return shows a success badge and the rejected return a danger badge, both as return-status
