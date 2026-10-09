@buyer @order
Feature: Buyer order detail - anatomy, tabs, cancel, timeline and exception pages
  OpenSpec change ui-phase-orders, /account/orders/[id]. Orders are seeded through the
  gateway; shipments and saga failures come from the real order service.

  Scenario: The detail shows header, progress, descriptions, items and tabs
    Given a d2 buyer has a single shipped order
    When the d2 buyer opens the order detail
    Then the detail shows the breadcrumb, id, shipped badge and total, the progress step, the descriptions, the items table and the active timeline tab

  Scenario: The active tab is held in the URL
    Given a d2 buyer has a single completed order
    When the d2 buyer opens the order detail
    And the d2 buyer selects the "Trả hàng / Hoàn tiền" tab
    Then the URL gains tab=returns and reloading shows the same tab

  Scenario: A cancelled order shows an Alert instead of the Stepper
    Given a d2 buyer has a single cancelled order
    When the d2 buyer opens the order detail
    Then an alert says the order was cancelled, no stepper is rendered and cancel is not offered

  Scenario: Cancelling from the detail really cancels
    Given a d2 buyer has a single pending order
    When the d2 buyer opens the order detail
    And the d2 buyer confirms the cancellation of the detail Modal
    Then the cancel action was sent once, a success toast shows and the header badge reads "Đã hủy"

  Scenario: Shipment checkpoints are listed newest first
    Given a d2 buyer has a single pending order
    And the order has a shipment
    When the d2 buyer opens the order detail
    Then the shipment checkpoint is marked current with the carrier and tracking code in the header

  Scenario: A pending saga step is distinguished from failure
    Given a d2 buyer has a single pending order
    When the d2 buyer opens the order detail
    Then the payment step renders as pending, no failure item exists and no error alert is shown

  Scenario: The return is not offered for non-eligible orders
    Given a d2 buyer has a single pending order
    When the d2 buyer opens the order detail
    Then the return request button is not offered

  Scenario: A missing order shows 404
    Given a d2 buyer has no orders
    When the d2 buyer opens the detail of an order that does not exist
    Then a 404 result links back to the order list
