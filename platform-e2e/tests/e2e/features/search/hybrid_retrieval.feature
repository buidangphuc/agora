@search @buyer
Feature: Multi-strategy Hybrid Retrieval Platform
  As a buyer
  I want search queries to retrieve products using both lexical keyword matching and semantic nearest neighbors
  So that I get relevant products even when terms differ or are phrasing variations

  @needsBuyer @needsListing
  Scenario: Buyer searches catalog with multi-strategy hybrid retrieval
    Given a buyer is logged in
    When the buyer searches for "sneakers"
    Then the search results page displays matching products
