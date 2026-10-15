Feature: KYC reviews are audited
  ReviewKyc writes one team-audit event (kyc.review) before the decision is applied and fails
  closed when team-audit is unavailable. Scenario names echo the spec scenarios of
  openspec/changes/verification-kyc-audit. Events are read back through QueryAuditLog as admin.

  Scenario: An audited KYC approval leaves one audit event (Audited approve is visible end to end)
    Given a seller with a pending KYC submission
    And an admin is logged in
    When the admin approves the seller's KYC submission
    Then the verification gateway call returns 200
    And the seller's verification status is verified
    And exactly one kyc.review audit event exists for the submission with decision "approve"

  Scenario: An audited KYC rejection leaves one audit event (Audited reject is visible end to end)
    Given a seller with a pending KYC submission
    And an admin is logged in
    When the admin rejects the seller's KYC submission
    Then the verification gateway call returns 200
    And the seller's verification status is rejected and not verified
    And exactly one kyc.review audit event exists for the submission with decision "reject"

  @destructive
  Scenario: KYC approval is refused while the audit service is down (Audit down blocks approval end to end)
    Given a seller with a pending KYC submission
    And an admin is logged in
    When team-audit is stopped
    And the admin approves the seller's KYC submission
    Then the verification gateway call returns 503 with code unavailable
    And the seller's verification status is still not verified
    When team-audit is started again
    And the admin approves the seller's KYC submission
    Then the verification gateway call returns 200
    And exactly one kyc.review audit event exists for the submission with decision "approve"

  Scenario: A refused KYC review writes no audit event (Non-admin approval writes no event end to end)
    Given a signed-in buyer with a pending KYC submission
    When the buyer approves their own KYC submission
    Then the verification gateway call returns 403
    And no audit event exists for the submission
