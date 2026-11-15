@promo @ads
Feature: Sponsored slots are served end to end
  A seller's ACTIVE ad campaign is served in the search and home sponsored slots, each slot
  carrying an opaque impression token.

  Scenario: Served slots are visible end to end
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 900 and budget 5000 on that listing
    When a buyer lists the sponsored slots for placement "search" in that category with limit 2
    Then the slots contain that listing with a non-empty impression token
    And the slots expose only the listing id, the campaign id and the impression token
    And the seller's campaign reads impressions 1 and spent 0
    When an anonymous caller lists the sponsored slots for placement "home" with limit 4
    Then the slots hold 1 to 4 entries that expose only the listing id, the campaign id and a non-empty impression token
