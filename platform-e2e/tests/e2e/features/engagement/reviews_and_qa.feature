@engagement
Feature: Product Reviews and Community Q&A
  As a buyer
  I want to read verified reviews
  And ask questions directly to the seller

  @smoke @needsBuyer @needsListing
  Scenario: Buyer submits a product question and views reviews
    Given I am logged in as a buyer via API
    When I view the seeded listing detail page
    Then I should see the product rating breakdown and review section
    When I submit a question "San pham co san hang khong shop?" on the listing
    Then the question should be submitted successfully
