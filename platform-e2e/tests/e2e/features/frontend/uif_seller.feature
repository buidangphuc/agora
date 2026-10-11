@seller @destructive
Feature: Seller workplace failure paths with a real service outage (ui-phase-seller)
  Each scenario genuinely stops an agora container (team-order behind the open-orders KPI,
  team-ai behind Magic Listing, team-analytics behind the funnel, team-domain behind the listings
  reads) and restores it in teardown. Destructive: serial lane only.

  Scenario: A KPI without a source is hidden, not zeroed
    Given a uif seller has 2 published listings
    When team-order is stopped to force a real failure
    And the uif seller opens the workplace
    Then the product KPIs render, the open-orders KPI is absent and no placeholder zero stands in for it

  Scenario: Magic Listing failure is recoverable
    Given a uif seller has 0 published listings
    When team-ai is stopped to force a real failure
    And the uif seller asks for an AI suggestion for a typed title
    Then an alert with a retry button and an error toast are shown and the seller can still submit manually

  Scenario: Analytics failure is recoverable
    Given a uif seller has 1 published listings
    When team-analytics is stopped to force a real failure
    And the uif seller opens the analytics page
    Then the funnel shows an alert with a retry link to the same URL and the rest of the page still renders

  Scenario: Route error offers recovery
    Given a uif seller has 1 published listings
    When team-domain is stopped to force a real failure
    And the uif seller opens the edit page of the listing
    Then the route error result offers a retry button and a link to "/seller"
    When team-domain is running again and answers through the gateway
    And the seller presses the retry button
    Then the edit form of the listing is rendered

  Scenario: List read failure shows an Alert
    Given a uif seller has 1 published listings
    When team-domain is stopped to force a real failure
    And the uif seller opens the workplace
    Then the product list shows an error alert with a retry link above no table rows and no raw error text
