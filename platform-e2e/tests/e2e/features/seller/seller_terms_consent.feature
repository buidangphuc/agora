@seller @seller_terms_consent
Feature: Seller onboarding links the seller terms and records acceptance (change frontend-legal-pages)

  Scenario: Terms link opens the seller terms page
    Given a freshly registered buyer for the users page
    And the buyer is signed in on the seller app
    When the buyer opens "/seller/new" on the seller app
    Then the seller terms link targets the buyer app "/dieu-khoan-nguoi-ban" in a new tab
    And the seller terms link target answers 200

  Scenario: Accepting seller terms records a ledger row
    Given a freshly registered buyer for the users page
    And the buyer is signed in on the seller app
    When the buyer opens "/seller/new" on the seller app
    And the buyer clicks the onboarding button
    Then the browser ends on "/seller/new" with the new-listing form visible
    And the ledger holds a SELLER_TERMS row with the seller terms version and source "seller_onboarding"

  Scenario: Denied BecomeSeller writes no consent row
    Given a freshly registered buyer for the users page
    And the buyer is signed in on the seller app
    When the buyer opens "/seller/new" on the seller app
    Then the onboarding button is shown
    When an admin sets that buyer to "restricted" after the page loaded
    And the buyer clicks the onboarding button
    Then the onboarding button is not shown
    And the ledger holds no SELLER_TERMS row for the buyer
    And the gateway lists the buyer without the seller role

  @destructive
  Scenario: Consent write failure does not undo seller status
    Given a freshly registered buyer for the users page
    And the consent table is temporarily unavailable to team-identity
    And the buyer is signed in on the seller app
    When the buyer opens "/seller/new" on the seller app
    And the buyer clicks the onboarding button
    Then the browser ends on "/seller/new" with the new-listing form visible
    And the seller app shows no "Cần tài khoản Người Bán"
    And the gateway lists the buyer with the seller role
