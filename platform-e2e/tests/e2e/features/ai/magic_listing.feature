@ai @seller
Feature: AI magic listing
  As a seller, the AI copilot drafts my listing so I publish faster.

  @needsSeller
  Scenario: AI fills the listing description
    Given a seeded seller is logged in
    When the seller opens the new listing form
    And the seller clicks the AI generate button
    Then the description field is filled by AI
