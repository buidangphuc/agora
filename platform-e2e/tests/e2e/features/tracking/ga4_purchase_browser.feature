@tracking @analytics @buyer
Feature: Browser no longer sends purchase to GA4
  The purchase reaches GA4 only through the server-side relay. The frontend keeps the
  first-party /api/track beacon (and so the warehouse row) but pushes no purchase to
  window.dataLayer. The "Purchase with missing transaction id" scenario cannot be produced from
  the public UI (every created order has an id) and is not covered here.

  Scenario: dataLayer has no purchase
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer starts recording tracking beacons
    And the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order and lands on the order list
    And the tracking queue idle window elapses
    Then window.dataLayer holds 0 purchase entries
    And the /api/track requests carry exactly 1 purchase beacon with the id of the created order

  Scenario: Other events unchanged
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer moves through the four checkout steps
    Then exactly one "begin_checkout" event was pushed with the cart items and value

  Scenario: Warehouse purchase still recorded
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer starts recording tracking beacons
    And the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order and lands on the order list
    And the tracking queue idle window elapses
    Then tracking_events has one purchase row with transaction_id equal to the created order id
