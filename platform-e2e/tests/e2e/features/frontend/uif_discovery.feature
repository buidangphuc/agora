@destructive
Feature: Discovery failure paths with a real service outage (ui-phase-discovery)
  Each scenario genuinely stops an agora container (team-domain behind the listings feed, team-ai
  behind the recommendations, team-engagement behind "Vừa xem", team-search behind saved searches)
  and restores it in teardown. Destructive: serial lane only.

  Scenario: Feed failure shows a recovery action
    Given a published listing exists for the discovery failure paths
    When team-domain is stopped to force a real failure
    And a visitor opens the home page
    Then the feed area shows an error alert with a retry link to "/" instead of an empty grid

  Scenario: Block failure degrades locally
    Given a buyer has viewed a listing and sees the "Vừa xem" block on the home page
    When team-ai is stopped to force a real failure
    And team-engagement is stopped to force a real failure
    And the buyer opens the home page again
    Then neither the "Vừa xem" nor the "Gợi ý cho bạn" block is rendered and the feed and the hero still are

  Scenario: Server action failure shows an error toast
    Given a buyer opens the search results for a keyword
    When team-search is stopped to force a real failure
    And the buyer activates the save search button
    Then an error toast is shown, the save button is enabled again and nothing was saved
