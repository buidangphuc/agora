@seller
Feature: Listing moderation
  Listing status is owned by the server. A seller's publish goes through the auto-screen, a flagged
  listing waits for an admin decision, and every decision is audited and reaches search.

  Scenario: Auto-screen: Publish end to end, clean and flagged
    Given a seller for moderation checks
    When the seller publishes a clean listing and a flagged listing through the gateway
    Then an anonymous reader gets the clean listing with status "LISTING_STATUS_PUBLISHED"
    And an anonymous reader gets not_found for the flagged listing
    And the seller sees the flagged listing with status "LISTING_STATUS_PENDING_REVIEW"

  Scenario: Moderation: Decision flows end to end with audit
    Given a seller for moderation checks
    And the seller has a flagged listing awaiting review
    When the admin rejects the listing with reason "counterfeit" and note "Fake logo"
    Then the moderation call succeeded
    And the audit log holds exactly one "listing.moderate" event by the admin for the listing with decision "reject"
    And the seller's own listings show the listing as "LISTING_STATUS_REJECTED" with reason "counterfeit"

  @destructive
  Scenario: Moderation: Moderation is refused while audit is down end to end
    Given a seller for moderation checks
    And the seller has a flagged listing awaiting review
    And team-audit is stopped for moderation
    When the admin rejects the listing with reason "counterfeit" and note "Fake logo"
    Then the moderation call fails with code "unavailable"
    When team-audit is started again for moderation
    Then the seller sees the listing with status "LISTING_STATUS_PENDING_REVIEW"
    And the audit log holds no "listing.moderate" event for the listing

  Scenario: Moderation: Rejected listing leaves search end to end
    Given a seller for moderation checks
    And the seller has a flagged listing awaiting review
    When the admin approves the listing
    Then the listing is found by a search within 90 seconds
    When the admin bans the listing with reason "prohibited_item"
    Then the listing is absent from search within 60 seconds
