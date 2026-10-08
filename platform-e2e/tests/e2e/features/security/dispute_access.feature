@buyer
Feature: Disputes are tied to a real order and its parties, and shop replies to the listing owner
  CreateDispute verifies the order (caller must be its buyer, defendant must be its
  seller), GetDispute is limited to the parties and admins, and an answer is a shop
  reply only when the caller owns the question's listing. Asserted through the gateway.

  @needsBuyer @needsListing @needsOrder
  Scenario: The buyer opens a dispute against the order's seller
    Given a buyer who placed an order on a seller's listing
    When the buyer of an order opens a dispute naming the order's seller
    Then the dispute is created
    And both the buyer and the seller can read it

  @needsBuyer @needsListing @needsOrder
  Scenario: A buyer cannot name a different defendant
    Given a buyer who placed an order on a seller's listing
    When the buyer of an order opens a dispute naming a user who is not the order's seller
    Then the gateway answers HTTP 403

  @needsBuyer @needsListing @needsOrder
  Scenario: A stranger cannot open a dispute on someone else's order
    Given a buyer who placed an order on a seller's listing
    And a second buyer who did not place the order
    When a buyer who did not place the order opens a dispute on it
    Then the gateway answers HTTP 404

  @needsBuyer @needsListing @needsOrder
  Scenario: A stranger cannot read a dispute
    Given a buyer who placed an order on a seller's listing
    And the buyer has opened a dispute against the order's seller
    And a second buyer who did not place the order
    When a logged-in user who is neither party nor admin calls GetDispute for an existing dispute
    Then the gateway answers HTTP 404

  @needsBuyer @needsListing @needsSeller
  Scenario: A non-owner cannot post a shop reply
    Given a question on a seller's listing
    When a buyer answers a question on another seller's listing with is_shop_reply set
    Then the stored answer is not marked as a shop reply
