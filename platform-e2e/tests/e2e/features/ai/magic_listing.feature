@ai @seller
Feature: AI magic listing
  As a seller, the AI copilot drafts my listing so I publish faster.

  @needsSeller
  Scenario: AI fills the listing description
    Given a seeded seller is logged in
    When the seller opens the new listing form
    And the seller clicks the AI generate button
    And the seller applies all AI suggestions
    Then the description field is filled by AI

  @needsSeller
  Scenario: Magic Listing response carries request id and model version
    When the seller calls Magic Listing with a title and a category
    Then the Magic Listing response has a 32-character hex request id and the model version "rules-v1"
