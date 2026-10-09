@buyer @destructive
Feature: Error shell failure path with a real service outage (ui-foundation)
  The scenario genuinely stops team-domain so that a product page's server component throws during
  render, and restores it in teardown. Destructive: serial lane only.

  Scenario: A thrown page error is recoverable
    Given a listing with reviews exists for the failure paths
    When team-domain is stopped to force a real failure
    And a guest opens the listing page
    Then an error alert "Không thể tải sản phẩm" with a "Thử lại" button is shown
    When team-domain is running again and answers through the gateway
    And the guest clicks "Thử lại"
    Then the listing page renders its title again
