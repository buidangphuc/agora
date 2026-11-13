@order
Feature: team-order erases a user's contact data on UserErased
  A UserErased event on identity.events masks the buyer's recipient fields and return reasons and
  empties the cart, keeping every accounting field. The event is produced to the real topic;
  everything is read back through GetOrder, GetReturnRequest and GetCart.
  (change commerce-user-erasure)

  Scenario: ORD-1 Buyer contact data is masked and accounting data is kept
    Given a seller "S" with two stocked listings
    And a buyer "U" with 3 orders, one return with reason "Hàng bị vỡ" and 2 cart items
    When a UserErased event for "U" is consumed
    Then every order of "U" reads recipient name, phone, street, ward, district and city "Đã xóa"
    And the return reason of "U" reads "Đã xóa"
    And "U" has 0 cart items
    And the order ids, buyer, seller, totals, items and status of "U" are unchanged

  Scenario: ORD-2 Another user's data is untouched
    Given a seller "S" with two stocked listings
    And a buyer "U" with 3 orders, one return with reason "Hàng bị vỡ" and 2 cart items
    And a buyer "W" with 3 orders, one return with reason "Hàng bị vỡ" and 2 cart items
    When a UserErased event for "U" is consumed
    Then every order of "U" reads recipient name, phone, street, ward, district and city "Đã xóa"
    And the orders, return and cart of "W" are unchanged

  Scenario: ORD-3 A redelivered event changes nothing
    Given a seller "S" with two stocked listings
    And a buyer "U" with 3 orders, one return with reason "Hàng bị vỡ" and 2 cart items
    When a UserErased event for "U" is consumed
    Then every order of "U" reads recipient name, phone, street, ward, district and city "Đã xóa"
    When the same UserErased event for "U" and then one with a new event id are consumed
    Then the order erasure consumer completes both without error and "U" is unchanged

  Scenario: ORD-6 A malformed event is parked, not applied
    Given a seller "S" with two stocked listings
    And a buyer "U" with 3 orders, one return with reason "Hàng bị vỡ" and 2 cart items
    When a UserErased event with an empty user_id and a record that does not decode are produced
    Then both records are written to the order DLQ and nothing is applied to "U"

  Scenario: ORD-9 The masked order stays readable by its parties
    Given a seller "S" with two stocked listings
    And a buyer "U" with 3 orders, one return with reason "Hàng bị vỡ" and 2 cart items
    When a UserErased event for "U" is consumed
    Then seller "S" reads an order of "U" through GetOrder with recipient name "Đã xóa" and its original total and items
