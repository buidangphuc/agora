@ai @buyer
Feature: Listing screening is internal to services
  ScreenListing is a service-to-service RPC; the gateway must never route it.

  Scenario: The edge does not route ScreenListing
    When an anonymous caller and a signed-in buyer call ScreenListing through the gateway
    Then both calls fail with unimplemented and no verdict is returned
