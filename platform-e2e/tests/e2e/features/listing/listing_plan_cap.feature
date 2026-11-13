@seller @plan_cap
Feature: Seller plan caps the number of listings
  The seller's plan max_listings limits CreateListing through the gateway. The rejection
  carries the stable reason PLAN_LISTING_LIMIT. Existing listings are never removed or hidden.

  Scenario: FREE seller is blocked at the 11th listing
    Given a plan-cap seller on the plan "plan_free" who owns 10 listings of mixed statuses
    When the plan-cap seller creates another listing
    Then the create is rejected as RESOURCE_EXHAUSTED with reason "PLAN_LISTING_LIMIT"
    And the plan-cap seller's listing list has 10 entries

  Scenario: PRO seller can create beyond the FREE cap
    Given a plan-cap seller on the plan "plan_pro" who owns 10 listings of mixed statuses
    When the plan-cap seller creates another listing
    Then the create succeeds
    And the plan-cap seller's listing list has 11 entries
    When a plan-cap control seller on the plan "plan_free" with 10 listings creates another listing
    Then the control seller's create is rejected with reason "PLAN_LISTING_LIMIT"

  Scenario: Downgraded seller keeps existing listings
    Given a plan-cap seller on the plan "plan_pro" who owns 12 listings of mixed statuses
    And the plan-cap seller moves to the plan "plan_free"
    When the plan-cap seller opens their listings
    Then all 12 listings are returned and none is hidden or removed
    When the plan-cap seller creates another listing
    Then the create is rejected as RESOURCE_EXHAUSTED with reason "PLAN_LISTING_LIMIT"

  Scenario: Capped seller sees only their own listings
    Given a plan-cap seller on the plan "plan_free" who owns 10 listings of mixed statuses
    And another plan-cap seller on the plan "plan_free" who owns 3 listings of mixed statuses
    When the plan-cap seller opens their listings
    Then only the plan-cap seller's own 10 listings are returned and none of the other seller's
    When the plan-cap seller creates another listing
    Then the create is rejected as RESOURCE_EXHAUSTED with reason "PLAN_LISTING_LIMIT"
