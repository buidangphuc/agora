@seller
Feature: Seller cockpit
  As a seller, I work in a consistent cockpit: a collapsible sidebar (a Drawer on a phone), a Workplace
  dashboard, URL-driven tables, a validated listing studio and clear empty / error / not-found states.

  # ── Shell ──────────────────────────────────────────────────────────────
  @needsSeller
  Scenario: Sidebar collapses on desktop
    Given a seeded seller is logged in
    When the seller opens the seller workplace
    And the seller collapses the sidebar
    Then the sidebar is 64px wide and its links keep their accessible names

  @needsSeller
  Scenario: Navigation opens as a Drawer at 375px
    Given a seeded seller is logged in
    When the seller opens the seller workplace at a 375px viewport
    And the seller taps the menu button
    Then a navigation dialog lists the seller links and no sidebar is shown inline
    When the seller chooses "Quản lý đơn hàng" in the drawer
    Then the browser is on "/seller/orders" and the drawer is closed

  @needsSeller
  Scenario: Active route is announced
    Given a seeded seller is logged in
    When the seller opens the seller wallet page
    Then only the "Ví người bán" link is marked as the current page

  @needsSeller
  Scenario: Shop card shows the real shop name
    Given a seeded seller is logged in
    And the seller sets the shop display name to "Cửa hàng Hoa Mai"
    When the seller opens the seller workplace
    Then the shop card shows "Cửa hàng Hoa Mai"

  @needsSeller
  Scenario: Empty shop name falls back
    Given a seeded seller is logged in
    When the seller opens the seller workplace
    Then the shop card shows "Shop #" followed by the first 6 characters of the seller id

  Scenario: Missing scope shows a recovery Result
    Given a logged-in buyer without the seller scope
    When the buyer opens the seller workplace
    Then a result explains the seller role is required and links back to the home page

  # ── Workplace and Table List ───────────────────────────────────────────
  @needsSeller @needsListing
  Scenario: KPI row and quick actions on desktop
    Given a seeded seller is logged in
    When the seller opens the seller workplace
    Then the KPI row shows "Tổng sản phẩm"
    And the KPI row shows "Đang bán"
    And the four KPI cells sit in one row next to the sidebar
    When the seller activates the quick action "Ví người bán"
    Then the browser lands on "/seller/wallet"

  @needsSeller
  Scenario: Seller with no orders sees an Empty in recent orders
    Given a seeded seller is logged in
    When the seller opens the seller workplace
    Then the recent orders block shows an empty state with a link to add a product

  @needsSeller @needsListing
  Scenario: Product search is reflected in the URL
    Given a seeded seller is logged in
    When the seller opens the seller workplace
    And the seller searches for the seeded listing by title
    Then the URL carries the search text and the matching product is listed
    And the URL contains no page parameter after the search

  @needsSeller @needsListing
  Scenario: No match shows Empty with a clear action
    Given a seeded seller is logged in
    When the seller opens the seller workplace
    And the seller types "zzz-no-such-product" in the product search box
    Then a no-results state offers a clear-filter link

  @needsSeller
  Scenario: Invalid page falls back
    Given a seeded seller is logged in
    When the seller opens the workplace with an invalid page and status
    Then the first page renders without an error

  # ── Orders ─────────────────────────────────────────────────────────────
  @needsSeller @needsOrder
  Scenario: Order detail shows the real order and shipping gives feedback
    Given a seeded seller is logged in
    When the seller opens the detail of their order
    Then the detail shows the recipient and the item of that order
    When the seller hands the order to the carrier
    Then a success toast appears and the stepper shows the order as shipped

  @needsSeller @needsOrder
  Scenario: Print hides chrome
    Given a seeded seller is logged in
    When the seller opens the detail of their order
    Then the print layout hides the seller chrome and keeps the packing slip

  @needsSeller
  Scenario: Unknown order is a not-found state
    Given a seeded seller is logged in
    When the seller opens the order "does-not-exist"
    Then a not-found result links back to the seller area

  # ── Magic Listing ──────────────────────────────────────────────────────
  @needsSeller
  Scenario: Magic Listing suggestions are applied only on request
    Given a seeded seller is logged in
    When the seller types the title "Laptop Dell XPS 13" and the description "Mô tả của tôi"
    And the seller runs the AI suggestion
    Then the suggestion is shown and the typed description is unchanged
    When the seller applies all AI suggestions
    Then the description now holds the suggested text

  # ── Shop profile ───────────────────────────────────────────────────────
  @needsSeller
  Scenario: Saving a new shop name is reflected on the storefront
    Given a seeded seller is logged in
    When the seller saves the shop name "Nhà Sách An Nhiên"
    Then a success toast confirms the shop name
    And the public shop page shows "Nhà Sách An Nhiên"

  @needsSeller
  Scenario: An empty or too long name is rejected inline
    Given a seeded seller is logged in
    When the seller submits a blank shop name
    Then the shop name field shows a required error
    When the seller submits a shop name of 81 characters
    Then the shop name field shows a length error

  # ── Empty / recovery ───────────────────────────────────────────────────
  @needsSeller
  Scenario: Empty product list shows a call to action
    Given a seeded seller is logged in
    When the seller opens the seller workplace
    Then the empty product state offers a link to add a product

  # ── Responsive 375px ───────────────────────────────────────────────────
  @needsSeller @needsOrder
  Scenario: No page-level horizontal scroll at 375px
    Given a seeded seller is logged in
    When the seller opens "/seller" at a 375px viewport
    Then the page has no horizontal scroll
    When the seller opens "/seller/orders" at a 375px viewport
    Then the page has no horizontal scroll
    When the seller opens their order detail at a 375px viewport
    Then the page has no horizontal scroll

  @needsSeller
  Scenario: Form actions stay reachable on mobile
    Given a seeded seller is logged in
    When the seller scrolls the new listing form to the middle at 375px
    Then the Save button is still visible in the sticky bottom bar
