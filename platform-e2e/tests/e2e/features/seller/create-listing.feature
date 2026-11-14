@seller
Feature: Seller listing management
  As a seller, I can publish a listing so that buyers can find my product.

  @needsSeller
  Scenario: Seller creates a new listing
    Given a seeded seller is logged in
    When the seller creates a new listing
    Then the new listing appears in the seller's listings

  @needsSeller
  Scenario: A required-field error blocks the save
    Given a seeded seller is logged in
    When the seller submits the empty new listing form
    Then the title field shows a required error and no listing is created

  @needsSeller
  Scenario: A saved listing shows a success toast and a link to the list
    Given a seeded seller is logged in
    When the seller creates a new listing
    Then a success toast confirms the save
    And a success state offers a link to the seller's listings

  Scenario: Seller at the plan cap sees the upgrade link
    Given a plan-ui FREE seller owns 10 published listings
    When the plan-ui seller submits a new listing
    Then the plan-ui form shows the cap message and an upgrade link to the plans page
    And the plan-ui seller owns 10 listings

  Scenario: Seller below the plan cap creates a listing
    Given a plan-ui FREE seller owns 9 published listings
    When the plan-ui seller submits a new listing
    Then the plan-ui listing is created and no upgrade link is shown
    And the plan-ui seller owns 10 listings
