@seller @admin
Feature: Seller enforcement hold
  When an admin suspends or bans a seller, the seller's listings leave the store, and they come
  back when the seller is reinstated. team-domain reacts to UserStatusChanged on identity.events.
  Scenario names echo the spec scenarios of openspec/changes/domain-seller-enforcement-hold.
  Every "gone" assertion is paired with a positive control (visible before, other seller
  untouched) so it cannot pass vacuously.

  Scenario: Seller hold: Held listings leave the store
    Given an enforcement-hold seller with a published listing that is findable
    And a second enforcement-hold seller with a published listing that is findable
    When the admin suspends the first enforcement-hold seller
    Then within 60 seconds the first seller's listing is absent from search
    And a non-owner gets not_found for the first seller's listing
    And the second seller's listing is still returned by search and by GetListing

  Scenario: Seller hold: Restored listing is visible again
    Given an enforcement-hold seller with a published listing that is findable
    When the admin suspends the first enforcement-hold seller
    Then within 60 seconds the first seller's listing is absent from search
    When the admin sets the first enforcement-hold seller back to active
    Then within 60 seconds the first seller's listing is returned by search
    And a non-owner gets the first seller's listing with status "LISTING_STATUS_PUBLISHED"

  Scenario: Seller hold: Suspension lifted at login restores listings
    Given an enforcement-hold seller with a published listing that is findable
    When the admin suspends the first enforcement-hold seller until 8 seconds from now
    Then within 60 seconds the first seller's listing is absent from search
    When the suspension end time has passed and the first seller logs in
    Then within 60 seconds the first seller's listing is returned by search
    And a non-owner gets the first seller's listing with status "LISTING_STATUS_PUBLISHED"

  Scenario: Seller hold: Owner sees the held listing and the reason
    Given an enforcement-hold seller with a published listing that is findable
    When the admin suspends the first enforcement-hold seller
    Then within 60 seconds the first seller's listing is absent from search
    And the owner's own listings show the listing as "LISTING_STATUS_HELD" with reason "seller_enforcement"

  Scenario: Seller hold: Restricted seller keeps listings published
    Given an enforcement-hold seller with a published listing that is findable
    And a second enforcement-hold seller with a published listing that is findable
    When the admin restricts the first enforcement-hold seller
    And the admin suspends the second enforcement-hold seller
    Then within 60 seconds the second seller's listing is absent from search
    And the first seller's listing is still returned by search and by GetListing
