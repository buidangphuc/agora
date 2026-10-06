@buyer
Feature: UI core components - state contract and accessibility
  The dev-only /dev/ui catalogue renders every core component in every state.
  These scenarios drive it to prove the shared state contract and a11y rules.
  They need only the frontend (run against `next dev`), no backend services.

  Scenario: A loading button keeps its width and blocks clicks
    Given the UI catalogue is open
    Then the loading button has the same width as the idle button
    And the loading button is busy and shows a spinner
    And activating the loading button does not trigger its click handler

  Scenario: Disabled is announced and inert
    Given the UI catalogue is open
    Then the disabled button is disabled and announced as aria-disabled
    And activating the disabled button does not trigger its click handler

  Scenario: Focus ring shows for keyboard users only
    Given the UI catalogue is open
    When the user tabs to the primary button
    Then a focus ring is drawn around the primary button
    When the primary button is clicked with the mouse
    Then no focus ring is drawn around the primary button

  Scenario: An empty table shows Empty, not a blank area
    Given the UI catalogue is open
    Then the empty table keeps its column headers and shows the "Chưa có sản phẩm" description

  Scenario: A loading statistic does not shift layout
    Given the UI catalogue is open
    When the statistics switch from loading to their values
    Then the height of every statistic card is unchanged

  Scenario: Modal traps focus and restores it
    Given the UI catalogue is open
    When the user opens the modal and presses Tab 8 times
    Then focus stays inside the dialog
    When the user presses Escape
    Then the dialog is closed and focus is back on the opening button

  Scenario: Tabs are keyboard navigable
    Given the UI catalogue is open
    When focus is on the first tab and the user presses ArrowRight
    Then the second tab is selected and its panel is shown

  Scenario: Link tabs keep their state in the URL
    Given the UI catalogue is open
    Then the link tabs are anchors and "Đang giao" is the current page
    When the link tab "Hoàn tất" is clicked and the user goes back
    Then "Đang giao" is the current link tab again

  Scenario: Input errors are announced
    Given the UI catalogue is open
    Then the "Email lỗi" input is invalid and described by "Email không hợp lệ"

  Scenario: QuantityPicker respects its bounds
    Given the UI catalogue is open
    Then the quantity picker at its maximum has a disabled increment button
    When 5 is typed into the quantity picker
    Then the quantity picker shows 3 and its decrement button is enabled

  Scenario: Pagination works without client JavaScript
    Given the UI catalogue is open
    Then pagination links to pages 1 to 5 are shown and page 2 is the current page

  Scenario: Image reserves its box and falls back
    Given the UI catalogue is open
    Then the broken image shows the fallback placeholder inside a square box
