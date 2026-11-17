@seller
Feature: Seller bulk ship and batch print on /seller/orders (seller-bulk-ship, initiative seller-centre)
  Scenario names equal the spec scenario titles of openspec/changes/archive/2026-11-17-seller-bulk-ship. Orders are
  seeded through the real gateway API and the buyer cancel goes through the API.

  Scenario: Bulk ship: a seller ships several paid orders in the browser
    Given a bulk-ship seller has 3 shippable orders
    When the seller opens the order list
    And the seller selects all the bulk-ship orders
    And the seller types the tracking number "T1" for the first order and confirms
    Then the bulk-ship result list shows no failure reason
    And 3 of the bulk-ship orders show "Đang giao" on the order list
    And the first bulk-ship order shows the typed tracking number on its detail page

  Scenario: Bulk ship: an order cancelled meanwhile fails only its row in the browser
    Given a bulk-ship seller has 3 shippable orders
    When the seller opens the order list
    And the seller selects all the bulk-ship orders
    And the buyer of the second bulk-ship order cancels it
    And the seller confirms the bulk ship
    Then the bulk-ship result list shows the cancelled-order reason once
    And the cancelled bulk-ship order is not shipped and the others show "Đang giao"

  Scenario: Bulk ship: batch print shows one slip per selected order in the browser
    Given a bulk-ship seller has 3 shippable orders
    When the seller opens the order list
    And the seller selects the first two bulk-ship orders
    And the seller opens the batch print for the selection
    Then the print page shows 2 packing slips with the recipient names of the selection

  Scenario: Bulk ship: another seller cannot ship or print a foreign order in the browser
    Given a bulk-ship seller has 1 shippable orders and another seller is signed in
    When the other seller opens the batch print for the first bulk-ship order
    Then the print page shows no packing slip and names the order in the "Không in được" notice
