@seller @needsSeller @needsListing
Feature: Delete a listing
  As a seller, I can delete my listing from the catalog.

  Scenario: Seller deletes an existing listing
    Given a seller owns a listing
    When the seller deletes it
    Then it no longer appears in the seller's listings

  Scenario: Delete requires confirmation and Cancel keeps the row
    Given a seller owns a listing
    When the seller opens their listings
    And the seller starts deleting the listing
    Then a confirm dialog names the listing and the listing is not deleted yet
    When the seller cancels the dialog
    Then the listing row is still listed

  Scenario: Confirming the delete removes the row with a toast
    Given a seller owns a listing
    When the seller opens their listings
    And the seller starts deleting the listing
    And the seller confirms the deletion
    Then the success toast appears and the listing row is gone
