@buyer @recommendations @destructive
Feature: The cart shows a cross-sell row (recsys-cart-cross-sell)
  The /cart page shows a "Thường được mua cùng" row from placement cart_cross_sell built from the cart
  lines, hides it when the cart is empty, and its beacons carry the recommendation identity. Shops,
  listings, the buyer and the cart are seeded through the gateway; the similar listings are indexed with
  `visible=true` directly in the serving collection (everything else hidden and restored in teardown).

  Scenario: Row shows for a cart with items
    Given a buyer has a cart line and two similar listings are indexed for recommendations
    When the buyer opens the cart
    Then the cross-sell row is shown with between 1 and 8 listings
    And none of the row listings is a cart line and every one is a similar listing

  Scenario: Row is hidden for an empty cart
    Given a buyer has a cart line and two similar listings are indexed for recommendations
    And the buyer opens the cart and the cross-sell row is shown
    When the buyer empties the cart and opens the cart again
    Then no cross-sell row is rendered on the empty cart
    And team-ai served no Recommend call since the cart was emptied

  Scenario: Beacons carry the recommendation identity
    Given a buyer has a cart line and two similar listings are indexed for recommendations
    When the buyer opens the cart recording beacons and clicks a cross-sell listing
    Then the impression and click beacons carry the request_id of the row as impressionId and the model version the gateway serves
