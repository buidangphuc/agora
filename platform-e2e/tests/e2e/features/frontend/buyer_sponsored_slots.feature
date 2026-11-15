@buyer @ads
Feature: Buyers see labelled sponsored slots and a click is charged once
  Search page 1 and the home feed show up to two "Tài trợ" slots from the real
  SponsoredService; a click charges the campaign once and never blocks navigation.
  Campaigns are created through the real CreateAdCampaign API and spent is read back through
  ListMyAdCampaigns; nothing is faked in the browser.
  (openspec change buyer-sponsored-slots, initiative monetization; the unit-level scenarios are
  not-testable entries in team-frontend/FEATURES.yaml)

  Scenario: Search shows a sponsored slot in its category
    Given a sponsored-slot seller with a sponsored listing and an organic listing in a fresh category
    And the seller has an ACTIVE sponsored-slot campaign with bid 1000 and budget 5000
    And both listings are searchable in that category
    When a guest opens the search page of that category
    Then the sponsored listing is shown in a "sponsored-slot" labelled "Tài trợ" before the first organic card

  Scenario: Sponsored listing is not duplicated in organic results
    Given a sponsored-slot seller with a sponsored listing and an organic listing in a fresh category
    And the seller has an ACTIVE sponsored-slot campaign with bid 1000 and budget 5000
    And both listings are searchable in that category
    When a guest opens the search page of that category
    Then the sponsored listing appears once on the page, inside "sponsored-slot" and not inside "search-results"

  Scenario: Home feed shows a sponsored slot
    Given a sponsored-slot seller with a sponsored listing and an organic listing in a fresh category
    And the seller has an ACTIVE sponsored-slot campaign with bid 1000 and budget 5000
    When a guest opens the home page
    Then the feed section "Gợi ý hôm nay" has a "sponsored-slot" labelled "Tài trợ" before its first organic card

  Scenario: Click charges the campaign once and opens the listing
    Given a sponsored-slot seller with a sponsored listing and an organic listing in a fresh category
    And the seller has an ACTIVE sponsored-slot campaign with bid 1000 and budget 5000
    And both listings are searchable in that category
    When a guest opens the search page of that category
    And the visitor clicks the sponsored slot
    Then the browser lands on the sponsored listing page
    And the sponsored-slot campaign reads spent 1000 once the click is recorded

  Scenario: Second click on the same slot charges nothing
    Given a sponsored-slot seller with a sponsored listing and an organic listing in a fresh category
    And the seller has an ACTIVE sponsored-slot campaign with bid 1000 and budget 5000
    And both listings are searchable in that category
    When a guest opens the search page of that category
    And the visitor clicks the sponsored slot
    Then the browser lands on the sponsored listing page
    And the sponsored-slot campaign reads spent 1000 once the click is recorded
    When the visitor returns to the results page without reloading and clicks the same slot again
    Then the browser lands on the sponsored listing page
    And the sponsored-slot campaign still reads spent 1000

  Scenario: Rendering the page charges nothing
    Given a sponsored-slot seller with a sponsored listing and an organic listing in a fresh category
    And the seller has an ACTIVE sponsored-slot campaign with bid 1000 and budget 5000
    And both listings are searchable in that category
    When a guest opens the search page of that category
    Then the sponsored listing is shown in a "sponsored-slot" labelled "Tài trợ" before the first organic card
    And the browser sent no click beacon
    And the sponsored-slot campaign still reads spent 0

  Scenario: Seller clicking their own ad is not charged
    Given a sponsored-slot seller with a sponsored listing and an organic listing in a fresh category
    And the seller has an ACTIVE sponsored-slot campaign with bid 1000 and budget 5000
    And both listings are searchable in that category
    When the owning seller opens the search page of that category
    And the visitor clicks the sponsored slot
    Then the browser lands on the sponsored listing page
    And the sponsored-slot campaign still reads spent 0
