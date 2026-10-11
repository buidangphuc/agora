@analytics @seller
Feature: Seller analytics are readable only by their owner or an admin
  GetSellerFunnel, GetRevenueBreakdown and GetDemandForecast are enforced in
  team-analytics from the gateway-forwarded principal: a user principal may read
  only its own seller_id, an admin may read any seller, anonymous callers are
  rejected. The gateway stays a plain forwarder.

  Scenario: A seller reads their own funnel
    Given a signed-in seller
    When the seller requests GetSellerFunnel for their own id
    Then the analytics gateway call returns 200 with a funnel

  Scenario: A seller cannot read another seller's revenue
    Given a signed-in seller
    And another seller exists
    When the seller requests GetRevenueBreakdown for the other seller's id
    Then the analytics gateway call returns 403 and no revenue figures

  Scenario: An anonymous caller is rejected
    Given no one is logged in
    When GetDemandForecast is requested for some seller without a token
    Then the analytics gateway call returns 401

  Scenario: An admin can read any seller
    Given a signed-in seller
    And an admin is logged in
    When the admin requests GetSellerFunnel for that seller's id
    Then the analytics gateway call returns 200 with a funnel
