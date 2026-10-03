@buyer
Feature: UI foundation - design tokens and exception shells
  As a visitor, the storefront resolves its colours from design tokens and shows
  a recoverable 404 page for unknown routes.

  Scenario: Unknown route shows the 404 result
    When the visitor opens the unknown route "/does-not-exist"
    Then the response status is 404
    And a not-found result with a link back to home is shown

  Scenario: The brand colour resolves from the design tokens
    Given the "home" page is open
    Then the header search button renders the brand colour "rgb(238, 77, 45)"
    And the "--color-action-primary" alias resolves to the brand colour "#ee4d2d"

  Scenario: Changing an alias restyles every user of it
    Given the "home" page is open
    When the "--color-action-primary" alias is overridden with "rgb(0, 0, 255)"
    Then every element styled by the action-primary alias renders "rgb(0, 0, 255)"
