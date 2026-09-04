@buyer @needsBuyer @needsListing @needsSeller
Feature: Buyer-seller chat thread
  As a buyer, I can chat directly with a seller.

  Scenario: Buyer opens chat with seller and sends a message
    Given a buyer is viewing a seller's product
    When the buyer opens chat and sends a message
    Then the message appears in the thread history
