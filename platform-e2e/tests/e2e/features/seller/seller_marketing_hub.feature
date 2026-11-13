@seller @promo
Feature: Seller marketing hub (seller-marketing-hub)
  A seller manages shop vouchers and flash sales of their own shop on /seller/marketing and never
  sees or touches another seller's. Scenario names equal the spec scenario titles of
  openspec/changes/seller-marketing-hub. Every refusal is paired with a positive control.

  Scenario: Seller opens the hub from the navigation
    Given a seller is signed in the seller app
    And the seller is on the seller centre
    When the seller clicks "Marketing" in the seller navigation
    Then the browser is on the marketing hub
    And the "Voucher của shop" tab is selected

  Scenario: Tab is kept in the URL
    Given a seller is signed in the seller app
    When the seller opens the marketing hub
    And the seller selects the "Flash sale" tab
    And the seller reloads the page
    Then the URL contains "tab=flashsale"
    And the "Flash sale" tab is selected

  Scenario: Buyer account is denied the hub
    Given a buyer is signed in the seller app
    When the visitor opens the marketing hub
    Then the seller access-denied view is shown
    And no marketing tab content is shown

  Scenario: Anonymous visitor is redirected to login
    Given an anonymous visitor
    When the visitor opens the marketing hub
    Then the visitor lands on the login page

  Scenario: Vouchers of every state are listed with used and quota
    Given a seller is signed in the seller app
    And the seller owns an active, a scheduled, an ended and a deactivated shop voucher
    When the seller opens the marketing hub
    Then four voucher rows show the labels Đang chạy, Sắp diễn ra, Đã kết thúc and Đã tắt with used over quota

  Scenario: Other sellers' vouchers are never shown
    Given a seller is signed in the seller app
    And the seller owns an active shop voucher
    And another seller owns a shop voucher and a platform voucher exists
    When the seller opens the marketing hub
    Then the seller's own voucher row is shown
    And no row for the other seller's or the platform voucher is shown

  Scenario: Valid voucher is created and listed
    Given a seller is signed in the seller app
    When the seller opens the marketing hub
    And the seller submits a new voucher with PERCENT 10, quota 100 and a window starting tomorrow
    Then a row for the new voucher with label Sắp diễn ra appears in the list
    And the seller's stored vouchers hold exactly one shop-scope voucher with that code and quota 100

  Scenario: Duplicate code shows the server error
    Given a seller is signed in the seller app
    And the seller owns an active shop voucher
    When the seller opens the marketing hub
    And the seller submits a new voucher reusing that code
    Then the error "Mã voucher đã tồn tại" is shown
    And the voucher form keeps the typed code
    And the seller's stored vouchers are unchanged

  Scenario: Deactivating an active voucher
    Given a seller is signed in the seller app
    And the seller owns an active shop voucher
    And a buyer can preview that voucher
    When the seller opens the marketing hub
    And the seller confirms "Tắt voucher" on that voucher
    Then that voucher row shows Đã tắt
    And the stored voucher is deactivated
    And a buyer can no longer preview that voucher

  Scenario: Another seller's voucher id is denied
    Given a seller is signed in the seller app
    And another seller owns a shop voucher
    When the seller deactivates the other seller's voucher id through the gateway
    Then the gateway answers "permission_denied"
    And the other seller's voucher is still active

  Scenario: Valid campaign is created and listed
    Given a seller is signed in the seller app
    And the seller owns a published listing
    When the seller opens the "Flash sale" tab of the marketing hub
    And the seller creates a flash sale for that listing with sale price 50000, cap 10 and a future window
    Then a campaign row for that listing with label Sắp diễn ra appears in the list
    And the seller's stored campaigns hold exactly one scheduled campaign for that listing with price 50000 and cap 10

  Scenario: Picker searches server-side
    Given a seller is signed in the seller app
    And the seller owns published listings "ao" and "quan" with distinct titles
    When the seller opens the "Flash sale" tab of the marketing hub
    And the seller types the "ao" title token in the listing picker
    Then the picker offers the "ao" listing and not the "quan" listing
    And the picker offers at most 20 options

  Scenario: Hub status filter narrows the list
    Given a seller is signed in the seller app
    And the seller owns a scheduled and a cancelled campaign on separate listings
    When the seller opens the "Flash sale" tab of the marketing hub
    And the seller selects the status filter "Đã huỷ"
    Then the URL contains "status=cancelled"
    And only the cancelled campaign row is shown

  Scenario: Cancelling a scheduled campaign
    Given a seller is signed in the seller app
    And the seller owns a scheduled campaign
    When the seller opens the "Flash sale" tab of the marketing hub
    And the seller confirms "Huỷ" on that campaign
    Then that campaign row shows Đã huỷ
    And the stored campaign is cancelled
    And the listing of the campaign has no active flash sale

  Scenario: Active and ended campaigns offer no cancel control
    Given a seller is signed in the seller app
    And the seller owns a scheduled, an active, an ended and a cancelled campaign
    When the seller opens the "Flash sale" tab of the marketing hub
    Then the scheduled campaign row has a "Huỷ" control
    And the active, ended and cancelled campaign rows have no "Huỷ" control

  Scenario: Cancelling a campaign that started meanwhile is refused
    Given a seller is signed in the seller app
    And the seller owns a scheduled campaign
    When the seller opens the "Flash sale" tab of the marketing hub
    And the campaign starts while the page is open
    And the seller confirms "Huỷ" on that campaign
    Then the message "Flash sale đã bắt đầu, không thể huỷ" is shown
    And that campaign row shows Đang diễn ra
    And the stored campaign is active

  Scenario: Other sellers' campaigns are never shown
    Given a seller is signed in the seller app
    And the seller owns a scheduled campaign
    And another seller owns a campaign
    When the seller opens the "Flash sale" tab of the marketing hub
    Then the seller's own campaign row is shown
    And no row for the other seller's campaign is shown

  Scenario: Another seller's campaign id is denied
    Given a seller is signed in the seller app
    And another seller owns a scheduled campaign
    When the seller cancels the other seller's campaign id through the gateway
    Then the gateway answers "permission_denied"
    And the other seller's campaign is still scheduled

  Scenario: Seller visiting the buyer voucher page sees no create form
    Given a seller is signed in the buyer app
    And a public platform voucher exists
    When the visitor opens the buyer voucher page
    Then the voucher catalogue shows the platform voucher
    And no create-voucher form is rendered

  Scenario: Buyer voucher wallet still renders
    Given a buyer who holds a voucher is signed in the buyer app
    When the visitor opens the buyer voucher page
    Then the held voucher appears on the page

  Scenario: Paging boundary at 21 vouchers
    Given a seller is signed in the seller app
    And the seller owns 21 shop vouchers
    When the seller opens the marketing hub
    Then 20 voucher rows are shown with a next-page control
    And the next page shows 1 voucher row

  Scenario: Empty list shows an empty state
    Given a seller is signed in the seller app
    When the seller opens the marketing hub
    Then the voucher tab shows an empty state with a "Tạo voucher" call to action and no error
    When the seller opens the "Flash sale" tab of the marketing hub
    Then the flash sale tab shows an empty state with a "Tạo flash sale" call to action and no error
