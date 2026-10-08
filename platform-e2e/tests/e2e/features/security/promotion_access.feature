@promo @seller @buyer
Feature: Voucher saga, plan subscription and sponsored ads are gated by caller rights
  team-promotion trusts the voucher saga RPCs only from a SERVICE principal. A user may
  only preview a voucher under their own namespace, and plan subscription, sponsored
  ads and entitlements are seller rights. Asserted through the gateway.

  Scenario: A buyer previews a voucher under their own namespace
    Given a commerce buyer
    When the buyer previews the voucher "SAVE10" for a 1000000 subtotal under their own preview namespace
    Then the voucher call returns 200
    And the preview is valid with a discount of 100000

  Scenario: A buyer cannot hold a reservation outside the preview namespace
    Given a commerce buyer
    And a second commerce buyer
    When the buyer reserves the voucher "SAVE10" under the reservation id "hold-e2e-order"
    Then the voucher call returns 403
    When the buyer reserves the voucher "SAVE10" under another buyer's preview namespace
    Then the voucher call returns 403
    When an anonymous caller previews the voucher "SAVE10"
    Then the voucher call returns 401

  Scenario: A voucher checkout still completes through the saga
    Given a commerce seller with a published listing priced 1000000 with stock 10
    And an admin created a 10 percent voucher with a quota of 5
    And a commerce buyer with a saved address
    When the buyer checks out 2 of the listing with the voucher and pays with the mock payment
    Then the order is paid with the 10 percent voucher discount applied
    And the voucher redemption is committed once

  Scenario: A buyer cannot subscribe to a seller plan
    Given a commerce buyer
    When the buyer subscribes to the plan "plan_pro"
    Then the plan call returns 403

  Scenario: A seller cannot advertise another seller's listing
    Given a commerce seller with a published listing priced 1000000 with stock 10
    And a second commerce seller
    When the second seller creates an ad campaign for the first seller's listing
    Then the plan call returns 403

  Scenario: An oversized ad bid is rejected
    Given a commerce seller with a published listing priced 1000000 with stock 10
    When the seller creates an ad campaign for their own listing with a bid of 1000
    Then the plan call returns 200
    When the seller creates an ad campaign for their own listing with a bid of 10000000000000
    Then the plan call returns 400

  Scenario: A seller cannot read another seller's entitlements
    Given a commerce seller with a published listing priced 1000000 with stock 10
    And a second commerce seller
    When the seller reads the entitlements of the second seller
    Then the plan call returns 403
