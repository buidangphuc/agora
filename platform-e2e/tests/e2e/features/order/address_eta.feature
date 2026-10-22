@order
Feature: Delivery date quote and fail-closed order address
  CalculateShippingFee quotes a delivery date, and CreateOrder resolves the shipping address from
  the calling buyer's own addresses and refuses an address that changed since the buyer confirmed it.
  (change order-address-eta, initiative q11-checkout-coherence)

  Scenario: Address and ETA: The delivery date is quoted through the gateway
    Given a signed-in buyer
    When the buyer asks for a shipping quote for city "Đà Nẵng" and items subtotal 300000
    Then the quote has shipping fee 35000
    And the quote delivery date is 4 calendar days after today in Asia/Ho_Chi_Minh

  Scenario: Address and ETA: Another buyer's address id is refused
    Given a seller who owns a listing with stock 5
    And a buyer "b1" with a default address and 1 of the listing in the cart
    And a buyer "b2" with a default address
    When "b1" places the order with the address id of "b2"
    Then the order is refused with "failed_precondition" and message "address not found"
    And "b1" has 0 orders
    And the cart of "b1" still holds 1 of the listing
    And "b1" can place the order with their own address id

  Scenario: Address and ETA: An edited address is refused end to end and a retry succeeds
    Given a seller who owns a listing with stock 5
    And a buyer "b1" with a default address and 1 of the listing in the cart
    And "b1" has confirmed their address
    When "b1" edits the confirmed address to city "Đà Nẵng"
    And "b1" places the order with the confirmed address
    Then the order is refused with "failed_precondition" and message "address changed"
    And "b1" has 0 orders
    When "b1" refreshes the confirmed address
    And "b1" places the order with the confirmed address
    Then "b1" has 1 order whose shipping address city is "Đà Nẵng"
