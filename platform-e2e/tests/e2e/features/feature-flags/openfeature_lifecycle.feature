@featureflags @openfeature @buyer
Feature: OpenFeature + Flipt lifecycle and checkout kill-switch
  team-order wires an OpenFeature client backed by the Flipt provider into its
  bootstrap/lifecycle, evaluating the checkout-enabled kill-switch against an
  in-memory snapshot streamed from Flipt. Flipping the flag toggles checkout
  within seconds with no redeploy; the flag defaults to on so a Flipt outage
  never disables checkout.

  Scenario: The service starts with a working flag client
    Given FLIPT_ADDR points at the Flipt server
    When team-order boots
    Then its bootstrap constructs an OpenFeature client with the Flipt provider
    And the client is available to handlers with an in-memory flag snapshot streamed from Flipt

  Scenario: The flag client shuts down cleanly
    Given team-order is running with the OpenFeature Flipt provider open
    When team-order shuts down
    Then the OpenFeature provider and Flipt stream are closed as part of resource teardown

  @needsBuyer @needsListing @killswitch
  Scenario: Kill-switch off blocks checkout end-to-end
    Given a buyer has an item in the cart
    When "checkout-enabled" is turned OFF in Flipt
    And the buyer opens the cart page
    Then the checkout entry point is hidden in the UI and a checkout-unavailable notice is shown
    And a CreateOrder request through the gateway is rejected as checkout-unavailable

  @needsBuyer @needsListing @killswitch
  Scenario: Kill-switch on allows checkout
    Given a buyer has an item in the cart
    When "checkout-enabled" is on or Flipt is unreachable and the default applies
    And the buyer opens the cart page
    Then the checkout entry point is shown in the UI
    And the buyer can place the order through the gateway to team-order

  @needsBuyer @needsListing @killswitch
  Scenario: Toggling takes effect without a redeploy
    Given the services and frontend are running unchanged
    When an operator flips "checkout-enabled" in the Flipt UI
    Then the new state takes effect within seconds through the streamed in-memory snapshot
    And no code change, image rebuild, ArgoCD sync, or pod restart is required
