@buyer @needsBuyer @needsListing @needsSeller
Feature: Product Q&A threads
  As a buyer, I can ask questions and receive seller answers.

  Scenario: Buyer asks question and seller answers on product listing
    Given a buyer is on a listing page
    When the buyer asks a question and the seller answers
    Then the Q&A thread shows both entries
