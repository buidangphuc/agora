@buyer @destructive
Feature: Product detail failure paths with a real service outage (ui-phase-product-detail)
  Each scenario genuinely stops or pauses an agora container (team-ai for the AI summary and the
  recommendations, team-domain for the listing read) and restores it in teardown. Destructive:
  serial lane only.

  Scenario: The summary does not block the page
    Given a listing with reviews exists for the failure paths
    When team-ai is paused so that its callers hang
    And a guest opens the listing page without waiting for its streamed sections
    Then the listing header, price and actions are visible while the AI summary shows a skeleton
    When team-ai is running again and answers through the gateway
    And the guest reloads the listing page
    Then the AI review summary card is shown in place of the skeleton

  Scenario: An unavailable AI service hides the block
    Given a listing with reviews exists for the failure paths
    When team-ai is stopped to force a real failure
    And a guest opens the listing page
    Then the reviews section renders with its reviews and no AI summary, error text or leftover skeleton

  Scenario: A slow recommendation call does not block the page
    Given a listing with reviews exists for the failure paths
    When team-ai is paused so that its callers hang
    And a guest opens the listing page without waiting for its streamed sections
    Then the listing header, price and actions are visible while the recommendations show one skeleton row

  Scenario: Unavailable recommendations are hidden
    Given a listing with reviews exists for the failure paths
    When team-ai is stopped to force a real failure
    And a guest opens the listing page
    Then the page has no "Gợi ý cho bạn" row, no recommendations skeleton and no error state

  Scenario: A gateway failure shows a recoverable error
    Given a listing with reviews exists for the failure paths
    When team-domain is stopped to force a real failure
    And a guest opens the listing page
    Then an error alert "Không thể tải sản phẩm" with a "Thử lại" button is shown
    When team-domain is running again and answers through the gateway
    And the guest clicks "Thử lại"
    Then the listing page renders its title again
