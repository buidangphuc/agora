@buyer
Feature: Empty and error UI states
  As a shopper or visitor, edge cases and empty states are handled gracefully.

  Scenario: Searching for a non-existent term displays empty search results with tips
    Given the "home" page is open
    When the buyer searches for "san_pham_khong_ton_tai_xyz_12345"
    Then the search results show an empty state notice
    And suggested search tips are displayed

  Scenario: Signed-in buyer navigating to an empty cart sees clear empty state and shop link
    Given a d2 buyer has an empty cart
    When the d2 buyer opens the cart
    Then the empty cart state is displayed
