@seller @admin
Feature: team-domain deletes an erased seller's listings and shop data on UserErased
  A UserErased event on identity.events makes team-domain delete every listing of that seller in any
  status with its variants, the storefront, the bundles and the hold marker, and emit one
  ListingChanged(DELETED) per listing on listing.events. The event is produced to the real topic;
  everything is read back through gateway RPCs, the broker and, where no RPC reaches it, listing_db.
  Every "gone" assertion is paired with a positive control (present before, other seller untouched).
  (change domain-seller-erasure, initiative trust-legal)

  Scenario: Erased seller's listings in every status are deleted
    Given an erasure seller "A" with listings "P1" published, "V" published with 2 variants, "R" pending_review, "D" draft, "H" held and "B" banned
    And an erasure seller "B" with a published listing "BL"
    When a UserErased event for the erasure seller "A" is consumed
    Then 0 listings and 0 variants remain for the erasure seller "A"
    And exactly 1 DELETED event exists for each of the 6 listings of "A"

  Scenario: Deleted listings leave the store
    Given an erasure seller "A" with a published listing "P1" that is findable
    When a UserErased event for the erasure seller "A" is consumed
    Then within 60 seconds a search for the title of "P1" returns 0 results
    And GetListing for "P1" returns not_found

  Scenario: Delete events match the existing delete path
    Given an erasure seller "A" with a published listing "P1"
    When a UserErased event for the erasure seller "A" is consumed
    Then the DELETED event of "P1" carries its id and title and principal "service-team-domain"

  Scenario: Other sellers' listings are not touched
    Given an erasure seller "A" with a published listing "P1"
    And an erasure seller "B" with a published listing "BL"
    When a UserErased event for the erasure seller "A" is consumed
    Then 0 listings and 0 variants remain for the erasure seller "A"
    And the listing "BL" of seller "B" is still published and has no DELETED event

  Scenario: Erasure has no RPC entry point and owner checks stay
    Given an erasure seller "A" with a published listing "P1"
    And an erasure seller "B" with a published listing "BL"
    When seller "B" calls DeleteListing on "P1" and an anonymous caller calls DeleteListing on "P1"
    Then the calls are denied with permission_denied and unauthenticated
    And the listing "P1" still exists

  Scenario: Roles in the event do not widen the delete
    Given an erasure seller "A" with a published listing "P1"
    And an erasure seller "N" with no listings
    When a UserErased event with no roles for the erasure seller "A" is consumed
    And a UserErased event with no roles for the erasure seller "N" is consumed
    Then 0 listings and 0 variants remain for the erasure seller "A"
    And the erasure consumer completes the events of "N" without a DLQ record

  Scenario: Batch boundary of 500 listings
    Given an erasure seller "A" with a published listing "P1" cloned to 501 listings
    When a UserErased event for the erasure seller "A" is consumed
    Then 0 listings and 0 variants remain for the erasure seller "A"
    And exactly 1 DELETED event exists for each of the 501 listings of "A"

  Scenario: Storefront and bundles are deleted
    Given an erasure seller "A" with a published listing "P1"
    And the erasure seller "A" has a storefront and 2 bundles
    When a UserErased event for the erasure seller "A" is consumed
    Then GetStorefront for the former slug of "A" returns not_found
    And 0 bundles exist for the erasure seller "A"

  Scenario: Another seller's storefront and bundles survive
    Given an erasure seller "A" with a published listing "P1"
    And an erasure seller "B" with a published listing "BL"
    And the erasure seller "A" has a storefront and 2 bundles
    And the erasure seller "B" has a storefront and 3 bundles
    When a UserErased event for the erasure seller "A" is consumed
    Then GetStorefront for the former slug of "A" returns not_found
    And the storefront of "B" and its 3 bundles are unchanged

  Scenario: Redelivered event is a no-op
    Given an erasure seller "A" with a published listing "P1"
    When a UserErased event for the erasure seller "A" is consumed
    And the same UserErased event for the erasure seller "A" is delivered again
    Then the erasure consumer completes both deliveries without a DLQ record
    And exactly 1 DELETED event exists for each of the 1 listings of "A"

  Scenario: Second event for the same user is a no-op
    Given an erasure seller "A" with a published listing "P1"
    When a UserErased event for the erasure seller "A" is consumed
    And a UserErased event with a new event id for the erasure seller "A" is consumed
    Then the erasure consumer completes both deliveries without a DLQ record
    And exactly 1 DELETED event exists for each of the 1 listings of "A"

  Scenario: Erasure of a user with no listings
    Given an erasure seller "N" with no listings
    When a UserErased event for the erasure seller "N" is consumed
    Then the erasure consumer completes the events of "N" without a DLQ record
    And no DELETED event names a listing of "N"

  Scenario: Malformed event goes to the DLQ
    Given an erasure seller "A" with a published listing "P1"
    When an undecodable record, a UserErased with an empty user_id and a UserErased without erased_at are produced
    Then all three records appear on identity.events.team-domain.dlq
    And the listing "P1" of seller "A" still exists

  Scenario: Other event types are ignored
    Given an erasure seller "A" with a published listing "P1" that is findable
    When the admin suspends the erasure seller "A"
    Then within 60 seconds the listing "P1" is absent from search
    And the listing "P1" of seller "A" still exists
    And the erasure group has no lag and the DLQ holds nothing for the seller "A"

  Scenario: A poison erasure event does not block seller hold
    Given an erasure seller "A" with a published listing "P1" that is findable
    When an undecodable record, a UserErased with an empty user_id and a UserErased without erased_at are produced
    And the admin suspends the erasure seller "A"
    Then within 60 seconds the listing "P1" is absent from search
    And all three records appear on identity.events.team-domain.dlq

  Scenario: Logs carry no raw identifiers
    Given an erasure seller "A" with listings "P1" published, "V" published with 2 variants, "R" pending_review, "D" draft, "H" held and "B" banned
    When a UserErased event for the erasure seller "A" is consumed
    Then the team-domain logs of the erasure mention the erasure and contain neither the user id nor any listing id of "A"
