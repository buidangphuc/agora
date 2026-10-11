@shop @needsSeller
Feature: Shop display name
  As a buyer
  I want shops to show their real name instead of an id
  So that I can recognise a shop in its page and my cart

  Scenario: The name is returned for a storefront
    Given the seller sets the shop display name to "  Tiem Hoa Nho  "
    When a visitor opens the seller's shop page
    Then the shop header shows "Tiem Hoa Nho"
    And the storefront API returns the display name "Tiem Hoa Nho"

  Scenario: A name change is reflected
    Given the seller sets the shop display name to "Tiem Hoa Nho"
    And the seller sets the shop display name to "Tiem Hoa Lon"
    When a visitor opens the seller's shop page
    Then the shop header shows "Tiem Hoa Lon"
    And the batch lookup returns the display name "Tiem Hoa Lon" for the seller

  Scenario: A cart with two sellers returns both names in one batch
    Given the seller sets the shop display name to "Shop Alpha"
    And a second seller sets the shop display name to "Shop Beta"
    And a buyer has one listing from each seller in the cart
    When the cart sellers are resolved with a single batch lookup
    Then the batch lookup returns "Shop Alpha" and "Shop Beta" for the two cart sellers
    When the buyer opens the cart with the seeded session
    Then the cart shows two shop groups headed "Shop Alpha" and "Shop Beta"

  Scenario: Fallback when the seller has no name
    Given the seller has no storefront
    When a visitor opens the seller's shop page
    Then the shop header shows the fallback label "Shop #" followed by the first 6 characters of the seller id

  Scenario: Invalid names are rejected
    Given the seller sets the shop display name to "Tiem Hoa Nho"
    When the seller upserts a storefront whose display name is 81 characters long
    And the seller upserts a storefront whose display name contains a control character
    Then both upserts fail with invalid_argument and the stored name is still "Tiem Hoa Nho"

  Scenario: A seller cannot set another seller's name
    Given the seller sets the shop display name to "Shop Alpha"
    And a second seller sets the shop display name to "Shop Beta"
    When the first seller sends an UpsertStorefront naming the second seller with the display name "Hijacked"
    Then the first seller's storefront is named "Hijacked" and the second seller's is still "Shop Beta"

  Scenario: An unknown or deleted seller gives an empty name
    Given the seller sets the shop display name to "Shop Alpha"
    When BatchGetStorefronts is called with the seller's id and an id that has no storefront
    Then the call succeeds with an entry for the seller only and none for the unknown id

  Scenario: Duplicates and oversize requests
    Given the seller sets the shop display name to "Shop Alpha"
    When BatchGetStorefronts is called with the seller's id three times
    Then the call succeeds with a single entry for the seller
    When BatchGetStorefronts is called with 101 distinct ids
    Then the call fails with invalid_argument

  Scenario: Following list resolves names in one batch
    Given a buyer follows three sellers, two with a display name and one without
    When the buyer opens the following page
    Then each followed shop row shows its display name or the "Shop #" fallback

  Scenario: Product detail shop header shows the name
    Given the seller sets the shop display name to "Tiem Hoa Nho"
    And the seller has a published listing
    When a visitor opens that listing
    Then the product page shop header shows "Tiem Hoa Nho" and links to the seller's shop

  # Needs a real BatchGetStorefronts failure: team-domain is stopped after the cart is seeded
  # (the cart itself is served by team-order), then restored in teardown.
  @destructive
  Scenario: Lookup failure does not break the page
    Given the seller sets the shop display name to "Shop Alpha"
    And a second seller sets the shop display name to "Shop Beta"
    And a buyer has one listing from each seller in the cart
    When team-domain is stopped so the batch lookup fails
    And the buyer opens the cart with the seeded session
    Then the cart still renders both items with the "Shop #" fallback header of each seller
