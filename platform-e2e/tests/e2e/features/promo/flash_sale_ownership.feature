@promo @seller
Feature: A seller can only run a flash sale on their own listings
  CreateCampaign verifies listing ownership against team-domain, using the
  gateway-forwarded principal: the owner may create a campaign, another seller is
  rejected with PermissionDenied (403), and an unknown listing is a bad request.

  Scenario: A seller creates a flash-sale campaign on their own listing
    Given a flash-sale seller with a published listing
    When that seller creates a flash-sale campaign for the listing
    Then the campaign call returns 200

  Scenario: A seller cannot put another seller's listing in a flash-sale campaign
    Given a flash-sale seller with a published listing
    And a second flash-sale seller
    When the second seller creates a flash-sale campaign for the first seller's listing
    Then the campaign call returns 403

  Scenario: A flash-sale campaign for an unknown listing is rejected
    Given a second flash-sale seller
    When the second seller creates a flash-sale campaign for an unknown listing
    Then the campaign call returns 400
