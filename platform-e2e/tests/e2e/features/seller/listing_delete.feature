@seller @needsSeller @needsListing
Feature: Delete a listing
  As a seller, I can delete my listing from the catalog.

  Scenario: Seller deletes an existing listing
    Given a seller owns a listing
    When the seller deletes it
    Then it no longer appears in the seller's listings
