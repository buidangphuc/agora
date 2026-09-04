@featureflags @buyer
Feature: Checkout emergency kill-switch (OpenFeature + Flipt)
  As an operator, the boolean flag checkout-enabled gates the checkout path.
  OFF hides the UI entry and makes team-order reject CreateOrder; ON restores
  checkout — flipped in Flipt within seconds, no redeploy. Default is ON so a
  Flipt outage never disables checkout.

  @needsBuyer @needsListing @killswitch
  Scenario: The service starts with a working flag client
    Given "checkout-enabled" is turned ON in Flipt
    And a buyer has an item in the cart
    When the buyer places the order through the gateway
    Then the order is accepted by team-order

  @needsBuyer
  Scenario: The browser never evaluates flags
    Given a buyer is logged in
    When the buyer opens the cart page
    Then the browser receives resolved markup with no Flipt address or flag SDK

  @needsBuyer @needsListing @killswitch
  Scenario: Kill-switch off blocks checkout end-to-end
    Given a buyer has an item in the cart
    When "checkout-enabled" is turned OFF in Flipt
    And the buyer opens the cart page
    Then the checkout entry point is hidden in the UI and a checkout-unavailable notice is shown
    And a CreateOrder request through the gateway is rejected as checkout-unavailable
    And checkout is restored by turning "checkout-enabled" back ON in Flipt

  @needsBuyer @needsListing @killswitch
  Scenario: Kill-switch on allows checkout
    Given a buyer has an item in the cart
    When "checkout-enabled" is turned ON in Flipt
    And the buyer opens the cart page
    Then the checkout entry point is shown in the UI
    And the buyer can place the order through the gateway to team-order

  @needsBuyer @needsListing @killswitch
  Scenario: Toggling takes effect without a redeploy
    Given a buyer has an item in the cart
    When "checkout-enabled" is turned OFF in Flipt
    Then a CreateOrder request through the gateway is rejected as checkout-unavailable
    When "checkout-enabled" is turned ON in Flipt
    Then the buyer can place the order through the gateway to team-order
