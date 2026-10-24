@buyer @needsBuyer @needsListing @needsOrder
Feature: Audited dispute resolution
  ResolveDispute writes one team-audit event before the dispute changes, and fails closed
  (Unavailable, dispute unchanged) when team-audit is down. Asserted through the gateway
  and QueryAuditLog as admin.

  Scenario: Dispute audit: Resolved dispute leaves one audit event end to end
    Given the buyer has an open dispute on the order
    And an admin has moved the dispute to investigating
    When an admin resolves the dispute with resolution "refund buyer"
    Then the dispute audit call succeeds
    And exactly one dispute.resolve audit event exists for the dispute with the admin as actor and resolution "refund buyer" and status "resolved"
    And the dispute has 2 dispute.resolve audit events in total

  @destructive
  Scenario: Dispute audit: Resolution is refused while team-audit is down end to end
    Given the buyer has an open dispute on the order
    And an admin has moved the dispute to investigating
    And team-audit is stopped
    When an admin resolves the dispute with resolution "refund buyer"
    Then the dispute audit call is refused with code "unavailable"
    And the dispute audit call message is "audit trail unavailable"
    And the dispute is still investigating
    And after team-audit is started again no resolved dispute.resolve audit event exists for the dispute

  Scenario: Dispute audit: A buyer cannot resolve a dispute end to end
    Given the buyer has an open dispute on the order
    When the buyer tries to resolve the dispute
    Then the dispute audit call is refused with code "permission_denied"
    And the dispute is still open with an empty resolution
    And no dispute.resolve audit event exists for the dispute

  Scenario: Dispute audit: Resolving a closed dispute writes no second event end to end
    Given the buyer has an open dispute on the order
    And an admin has moved the dispute to investigating
    When an admin resolves the dispute with resolution "refund buyer"
    And an admin resolves the dispute again with resolution "second try"
    Then the dispute audit call is refused with code "failed_precondition"
    And exactly one dispute.resolve audit event exists for the dispute with the admin as actor and resolution "refund buyer" and status "resolved"
    And the dispute has 2 dispute.resolve audit events in total
