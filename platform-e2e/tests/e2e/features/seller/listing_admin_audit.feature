@seller
Feature: Admin listing edits are audited
  An admin who edits or deletes a seller's listing leaves one audit event per action, and the
  edit is refused when the audit trail cannot be written. Owner edits are not audited.

  Scenario: Listing audit: Admin update and delete are audited end to end
    Given a seller owns a listing for the admin audit check
    When the seeded admin sends status rejected through UpdateListing
    Then the update succeeded and the listing keeps its stored status
    And the audit log holds exactly one "listing.admin_update" event for the listing by the admin with the seller id and the stored status
    When the seeded admin deletes the listing through the gateway
    Then the delete succeeded and the listing is gone
    And the audit log holds exactly one "listing.admin_delete" event for the listing by the admin with the seller id

  Scenario: Listing audit: Owner update is not audited end to end
    Given a seller owns a listing for the admin audit check
    When the owner renames the listing to "Owner renamed"
    Then the update succeeded and the listing has title "Owner renamed"
    And the audit log holds no event for the listing

  Scenario: Listing audit: Seller cannot edit another seller's listing end to end
    Given a seller owns a listing for the admin audit check
    And another seller without the admin scope
    When the other seller renames the listing to "Hijacked"
    Then the call fails with code "permission_denied"
    And the listing still has its original title
    And the audit log holds no event for the listing

  @destructive
  Scenario: Listing audit: Admin edit is refused while audit is down end to end
    Given a seller owns a listing for the admin audit check
    And team-audit is stopped
    When the seeded admin rejects the listing through the gateway
    Then the call fails with code "unavailable"
    When the seeded admin deletes the listing through the gateway
    Then the call fails with code "unavailable"
    And the listing still has its original title
    When team-audit is started again
    Then the audit log holds no event for the listing
