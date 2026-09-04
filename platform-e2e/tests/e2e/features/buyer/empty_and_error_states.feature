@buyer
Feature: Empty and error UI states
  As a shopper or visitor, edge cases and empty states are handled gracefully.

  Scenario: Searching for a non-existent term displays empty search results with tips
    Given the "home" page is open
    When the buyer searches for "san_pham_khong_ton_tai_xyz_12345"
    Then the search results show an empty state notice
    And suggested search tips are displayed

  Scenario: Visitor navigating to empty cart sees clear empty state and shop link
    Given the "home" page is open
    When the buyer opens the cart
    Then the empty cart state is displayed
