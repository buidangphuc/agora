@buyer @order
Feature: Buyer order screens - images, layout shift, responsive layout, tokens and server boundaries
  OpenSpec change ui-phase-orders. Server markup is read over HTTP with the buyer's session;
  layout shift comes from the browser's layout-shift entries.

  Scenario: Thumbnails reserve their box
    Given a d2 buyer has a single pending order
    When the d2 buyer opens the orders list
    Then every thumbnail keeps a fixed 1:1 box and shows the placeholder when the picture fails

  Scenario: Below-the-fold images are lazy
    Given a d2 buyer has 4 orders from one shop with thumbnails
    Then the first order card's thumbnail is eager and the thumbnails of the later cards are lazy

  Scenario: The detail does not jump when the timeline resolves
    Given a d2 buyer has a single shipped order
    Then the cumulative layout shift of the detail is 0

  Scenario: Mobile layout at 375px
    Given a d2 buyer has a single pending order
    When the d2 buyer opens the order list and the detail at 375 by 812
    Then there is no horizontal page scroll, the tab bar scrolls inside its own container and the action buttons are full width
    And the detail at 375px has no horizontal scroll, full width actions and a vertical stepper

  Scenario: Desktop layout at 1280px
    Given a d2 buyer has a single shipped order
    When the d2 buyer opens the order detail at 1280 by 800
    Then the stepper is horizontal, the items show as table columns and the content is at most 960px wide

  Scenario: No arbitrary type sizes
    Then the token lint reports no violation under "src/features/order,src/app/(shop)/account/orders"

  Scenario: Pages are not client components
    Given a d2 buyer has a single shipped order
    Then neither order page source contains a use client directive and the order data is in the server HTML

  Scenario: Test ids and data attributes are preserved
    Given a d2 buyer has a pending, a shipped, a completed and a cancelled order
    Then the existing timeline and return test ids are present in the rendered states
