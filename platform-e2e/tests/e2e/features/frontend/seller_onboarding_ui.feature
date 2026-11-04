@seller
Feature: Seller onboarding in the seller app (seller-onboarding-ui, initiative p10-backend-gaps)
  A signed-in buyer who opens the seller app sees an onboarding screen instead of a dead end and can
  accept the seller terms; a restricted, suspended or banned account gets no onboarding. Every
  refusal is paired with a positive control (the same flow shows the onboarding button before the
  admin acts, or an unrestricted buyer sees it) so it cannot pass vacuously. Accounts are fresh per
  scenario and the stored role is read back through the gateway.

  Scenario: Seller onboarding UI: a buyer becomes a seller in the browser
    Given a freshly registered buyer for the users page
    And the buyer is signed in on the seller app
    When the buyer opens "/sell" on the seller app
    Then the onboarding button is shown
    When the buyer clicks the onboarding button
    Then the browser ends on "/seller/new" with the new-listing form visible
    And the seller_session cookie carries listing.write
    And the seller_refresh cookie value is unchanged
    And the seller app shows no "Cần tài khoản Người Bán"
    And the gateway lists the buyer with the seller role

  Scenario: Seller onboarding UI: a returning seller is not asked again
    Given a freshly registered buyer for the users page
    And the buyer is signed in on the seller app
    When the buyer opens "/sell" on the seller app
    Then the onboarding button is shown
    When the buyer clicks the onboarding button
    Then the browser ends on "/seller/new" with the new-listing form visible
    When the buyer opens "/seller" on the seller app
    Then the seller centre renders
    And the onboarding button is not shown

  Scenario: Seller onboarding UI: an anonymous visitor is sent to login
    Given no one is logged in
    When the visitor opens "/sell" on the seller app
    Then the browser lands on the seller login with returnUrl "/seller/new"
    And the onboarding button is not shown
    Given a freshly registered buyer for the users page
    When the buyer signs in on the seller login that opened
    Then the browser ends on "/seller/new" with the onboarding button shown

  Scenario: Seller onboarding UI: a suspended buyer is sent to login in the browser
    Given a freshly registered buyer for the users page
    And the buyer is signed in on the seller app
    When the buyer opens "/seller/new" on the seller app
    Then the onboarding button is shown
    When an admin sets that buyer to "suspended" after the page loaded
    And the buyer clicks the onboarding button
    Then the browser lands on the seller login with returnUrl "/seller/new"
    And the onboarding button is not shown
    And the seller_session cookie does not carry listing.write
    And the gateway lists the buyer without the seller role

  Scenario: Seller onboarding UI: a banned buyer is sent to login in the browser
    Given a freshly registered buyer for the users page
    And the buyer is signed in on the seller app
    When the buyer opens "/seller/new" on the seller app
    Then the onboarding button is shown
    When an admin sets that buyer to "banned" after the page loaded
    And the buyer clicks the onboarding button
    Then the browser lands on the seller login
    And the browser never reached "/seller/new" after the click
    And the seller_session cookie does not carry listing.write
    And the gateway lists the buyer without the seller role

  Scenario: Seller onboarding UI: a restricted seller sees the inline 403 in the browser
    Given a freshly registered buyer for the users page
    And an admin has set that buyer to "restricted"
    When the buyer logs in on the login page and opens "/seller/new"
    Then the browser is still on "/seller/new"
    And the seller page shows "Tài khoản Người Bán đang bị hạn chế"
    And the onboarding button is not shown
    Given a second freshly registered buyer for the users page
    When the buyer logs in on the login page and opens "/seller/new"
    Then the onboarding button is shown

  Scenario: Seller onboarding UI: a buyer restricted after page load is refused
    Given a freshly registered buyer for the users page
    And the buyer is signed in on the seller app
    When the buyer opens "/seller/new" on the seller app
    Then the onboarding button is shown
    When an admin sets that buyer to "restricted" after the page loaded
    And the buyer clicks the onboarding button
    Then the browser lands on the seller login with returnUrl "/seller/new"
    And the onboarding button is not shown
    And the seller_session cookie does not carry listing.write
    And the gateway lists the buyer without the seller role
