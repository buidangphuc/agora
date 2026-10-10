Feature: Only services may write audit events
  AuditService/WriteAuditEvent needs a SERVICE principal holding audit.write, so the
  audit trail cannot be forged by end users. Browsers cannot reach it at all.

  Scenario: A refused edge write leaves no audit event
    Given a logged-in seller
    And the seeded admin
    When the seller calls WriteAuditEvent through the gateway for a fresh target type
    Then the refused write call fails
    And the seeded admin's QueryAuditLog for that target type returns no events
