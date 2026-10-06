Feature: KYC review is an admin-only action
  ReviewKyc is enforced in team-verification from the gateway-forwarded principal:
  it needs the admin scope and a reviewer can never review their own submission.
  The gateway stays a plain forwarder.

  Scenario: A buyer cannot approve their own KYC
    Given a signed-in buyer with a pending KYC submission
    When the buyer approves their own KYC submission
    Then the verification gateway call returns 403
    And the buyer's verification status is still not verified

  Scenario: An admin approves a seller's KYC
    Given a seller with a pending KYC submission
    And an admin is logged in
    When the admin approves the seller's KYC submission
    Then the verification gateway call returns 200
    And the seller's verification status is verified
