@seller
Feature: Seller Workplace - KPIs, quick actions, pagination and links
  OpenSpec change ui-phase-seller, /seller. Sellers, listings and orders are seeded through
  the gateway; the browser is logged in as the seller.

  Scenario: KPI row reflects the seller's data
    Given a d2 seller has 3 published listings, one with stock 2
    When the d2 seller opens the workplace
    Then the KPI row shows total 3, published 3 and low stock 1 derived from the listings

  Scenario: No hard-coded metric values
    Given a d2 seller has 0 published listings
    When the d2 seller opens the workplace
    Then every statistic is derived from the empty response and shows zero listings

  Scenario: Quick actions navigate
    Given a d2 seller has 1 published listings
    When the d2 seller opens the workplace
    And the d2 seller activates the quick action "Ví người bán"
    Then the seller browser lands on "/seller/wallet"

  Scenario: Desktop shows a four-up KPI row
    Given a d2 seller has 1 published listings
    When the d2 seller opens the workplace
    Then the four KPI cells appear in one row next to the sidebar

  Scenario: Pagination moves between pages
    Given a d2 seller has 45 published listings
    When the d2 seller opens "/seller?page=2"
    Then rows 21 to 40 are shown, page 2 is current and the previous and next links point at pages 1 and 3

  Scenario: Row links keep their targets
    Given a d2 seller has 1 published listings
    When the d2 seller opens the workplace
    And the d2 seller activates the title link of the first product row
    Then the browser opens the public listing page of that product

  Scenario: /sell redirects
    Given a d2 seller has 0 published listings
    When the d2 seller opens "/sell"
    Then the browser lands on the new listing studio
