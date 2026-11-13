@shop @needsSeller
Feature: Storefront customisation
  As a seller
  I want to customise my storefront banner, featured listings, tagline and theme
  So that buyers see a branded shop without unsafe or foreign content

  Scenario: Seller saves a banner uploaded through the image bucket
    When the seller uploads a banner image through the image bucket
    And the seller saves the storefront with the uploaded banner
    Then the storefront API returns the uploaded banner url

  Scenario: Seller saves six featured listings in order
    Given the seller owns 6 published listings
    When the seller features the listings in the order "C,A,F,B,E,D"
    Then the storefront API returns the featured listings in the order "C,A,F,B,E,D"

  Scenario: Seller cannot feature another seller's listing
    Given the seller owns 6 published listings
    And the seller features the listings in the order "A,B"
    And a second seller owns a published listing
    When the seller features the second seller's listing
    Then the upsert fails with invalid_argument without naming the owner
    And the storefront API still returns the featured listings in the order "A,B"

  Scenario: Unpublished featured listing does not break reading the storefront
    Given the seller owns 6 published listings
    And the seller features the listings in the order "A,B,C"
    When the seller moves listing "B" to draft
    Then an anonymous buyer reading the storefront still gets the featured listings in the order "A,B,C"

  Scenario: Changing only the tagline keeps banner, featured listings and theme
    Given the seller owns 6 published listings
    And the seller has a stored storefront with an uploaded banner, featured listings "B,A", tagline "Hoa tuoi" and theme "festive"
    When the seller resends the same storefront with the tagline "Hoa tuoi moi ngay"
    Then the storefront API returns tagline "Hoa tuoi moi ngay", the same banner, featured listings "B,A" and theme "festive"
