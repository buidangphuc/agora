@order @buyer @seller
Feature: A buyer adds a seller bundle to the cart and pays its combo price
  team-order adds one tagged cart line per bundle listing with the discount allocated pro rata,
  drops the bundle when a line is changed, and at checkout prices a valid bundle at its combo
  price or refuses it with the price-changed error. Everything goes through the gateway; bundles
  and listings are seeded through the real API (stored-before-the-rules bundles are inserted into
  team-domain's table because CreateBundle refuses them now).
  (openspec change order-bundle-checkout, initiative seller-centre; the migration idempotency
  scenario is a not-testable entry in team-order/FEATURES.yaml)

  Scenario: Adding a bundle creates one tagged line per listing
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    When the buyer adds the bundle to the cart with quantity 1
    Then the buyer's cart holds 2 lines tagged with the bundle
    And the bundle line discounts are 20000 and 10000
    And the cart bundle discount total is 30000 and the subtotal is 120000

  Scenario: The discount is allocated exactly with the remainder on the last line
    Given a seller with a published bundle of 3 listings priced 10000 each at bundle price 20000
    And a buyer with a saved address
    When the buyer adds the bundle to the cart with quantity 3
    Then the buyer's cart holds 3 lines tagged with the bundle
    And the bundle line discounts sum to 30000
    And the cart bundle discount total is 30000 and the subtotal is 60000

  Scenario: Adding the same bundle again increases every line
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    When the buyer adds the bundle to the cart with quantity 1
    And the buyer adds the bundle to the cart with quantity 2
    Then every bundle line has quantity 3
    And the cart bundle discount total is 90000 and the subtotal is 360000

  Scenario: A bundle line stays separate from a plain line of the same listing
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has 1 of the first listing as a plain line
    When the buyer adds the bundle to the cart with quantity 1
    Then the cart has a plain line of the first listing with quantity 1 and price 100000
    And the cart has a bundle line of the first listing

  Scenario: An unknown bundle is rejected
    Given a buyer with a saved address
    When the buyer adds an unknown bundle to the cart with quantity 1
    Then the bundle call fails with not_found
    And the buyer's cart is empty

  Scenario: A quantity out of range is rejected
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    When the buyer adds the bundle to the cart with quantity 0
    Then the bundle call fails with invalid_argument
    When the buyer adds the bundle to the cart with quantity 100
    Then the bundle call fails with invalid_argument
    And the buyer's cart is empty

  Scenario: A bundle with an unavailable listing is refused
    Given a seller with a published bundle "unpublished" of listings priced 100000 and 50000 at bundle price 120000
    And a seller bundle "scarce" of listings priced 100000 and 50000 at bundle price 120000 where the second listing has stock 1
    And a stored bundle "foreign" of one listing of the seller and one listing of another seller at bundle price 120000
    And a buyer with a saved address
    And the second listing of bundle "unpublished" is unpublished
    When the buyer adds the bundle "unpublished" to the cart with quantity 1
    Then the bundle call fails with failed_precondition
    When the buyer adds the bundle "scarce" to the cart with quantity 2
    Then the bundle call fails with failed_precondition
    When the buyer adds the bundle "foreign" to the cart with quantity 1
    Then the bundle call fails with failed_precondition
    And the buyer's cart is empty

  Scenario: A bundle with a variant listing is refused
    Given a stored bundle "variant" of one listing of the seller and one listing with variants at bundle price 120000
    And a buyer with a saved address
    When the buyer adds the bundle "variant" to the cart with quantity 1
    Then the bundle call fails with failed_precondition
    And the buyer's cart is empty

  Scenario: A bundle priced at or above the sum of its listings is refused
    Given a stored bundle "dear" of two listings of the seller priced 100000 and 50000 at bundle price 150000
    And a buyer with a saved address
    When the buyer adds the bundle "dear" to the cart with quantity 1
    Then the bundle call fails with failed_precondition
    And the buyer's cart is empty

  Scenario: An unauthenticated caller is rejected
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    When a caller without a session adds the bundle to the cart with quantity 1
    Then the bundle call fails with unauthenticated

  Scenario: The cart of another buyer is never touched
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer "A" with a saved address
    And a buyer "B" with a saved address
    When buyer "A" adds the bundle to the cart with quantity 1
    Then buyer "A" has 2 lines tagged with the bundle
    And buyer "B" has no line tagged with the bundle

  Scenario: Changing one line quantity drops the tag from all lines
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has the bundle in the cart with quantity 1
    When the buyer changes the quantity of the first bundle line to 2
    Then no line of the bundle keeps a bundle tag or discount
    And the cart bundle discount total is 0 and the subtotal is 250000

  Scenario: Removing one line drops the tag from the others
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has the bundle in the cart with quantity 1
    When the buyer removes the first bundle line
    Then the cart holds only a plain line of the second listing at price 50000 with quantity 1
    And the cart bundle discount total is 0 and the subtotal is 50000

  Scenario: A dropped line merges into the plain line of the same listing
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has 1 of the first listing as a plain line
    And the buyer has the bundle in the cart with quantity 2
    When the buyer changes the quantity of the second bundle line to 3
    Then the cart holds one line of the first listing with quantity 3 and no bundle tag

  Scenario: Another buyer's bundle line cannot be changed
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer "A" with a saved address
    And a buyer "B" with a saved address
    And buyer "A" has the bundle in the cart with quantity 1
    When buyer "B" changes the quantity of buyer "A"'s first bundle line to 2
    Then the bundle call fails with not_found
    When buyer "B" removes buyer "A"'s first bundle line
    Then the bundle call fails with not_found
    And buyer "A" has 2 lines tagged with the bundle

  Scenario: The order total reflects the bundle price
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has the bundle in the cart with quantity 1
    When the buyer previews the checkout
    And the buyer places the order
    Then the preview items subtotal is 120000
    And the order total is the bundle price 120000 plus its shipping fee

  Scenario: The seller's order shows the bundle on its lines
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has the bundle in the cart with quantity 1
    When the buyer places the order
    And the seller reads the order
    Then every item of the order carries the bundle id and the discounts sum to 30000

  Scenario: A bundle that became invalid is untagged before payment
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has the bundle in the cart with quantity 1
    When the second listing of the bundle is unpublished
    And the buyer previews the checkout
    Then the checkout is refused with failed_precondition "price changed"
    And the cart lines of the bundle are plain at regular prices
    And the stock of both listings is unchanged

  Scenario: A changed bundle price is untagged at checkout
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has the bundle in the cart with quantity 1
    When the seller changes the price of the first listing to 110000
    And the buyer places the order
    Then the checkout is refused with failed_precondition "price changed"
    And the buyer has no orders

  Scenario: Checking out only part of a bundle drops the discount
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has the bundle in the cart with quantity 1
    When the buyer places an order for only the first bundle line
    Then the checkout is refused with failed_precondition "price changed"
    And the cart lines of the bundle are plain at regular prices

  Scenario: A seller cannot check out their own bundle
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And the seller has a saved address
    When the seller adds the bundle to their own cart and places the order
    Then the order is refused as a purchase of the seller's own listing
    And the seller has no orders

  Scenario: A return refunds the allocated bundle price
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has the bundle in the cart with quantity 1
    And the buyer has paid the bundle order
    When the buyer requests a return without an amount
    Then the return refund amount equals the order total
    When the buyer requests a return above the order total
    Then the bundle call fails with invalid_argument

  Scenario: A plain cart is priced as before
    Given a seller with a published bundle of listings priced 100000 and 50000 at bundle price 120000
    And a buyer with a saved address
    And the buyer has 2 of the first listing as a plain line
    When the buyer previews the checkout
    Then the cart bundle discount total is 0 and the subtotal is 200000
    And the preview items subtotal is 200000
