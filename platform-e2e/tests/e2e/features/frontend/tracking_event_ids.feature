@buyer @tracking
Feature: The storefront gives every tracking beacon its own event id
  The tracker stamps each beacon with a fresh UUID eventId, so a re-sent flush carries the same ids
  and the warehouse dedupes it instead of counting the event twice (tracking-ingest-integrity).

  @needsBuyer @needsListing
  Scenario: The storefront sends an event id on every beacon
    Given a buyer is logged in
    And a listing has been seeded via the API
    When a buyer opens a product page in the storefront
    Then every beacon the page sends to /api/track carries a distinct UUID eventId
