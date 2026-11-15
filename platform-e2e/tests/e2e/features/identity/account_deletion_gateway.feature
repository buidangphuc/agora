@auth @account_deletion
Feature: The gateway routes the lifecycle RPCs for signed-in users only
  RequestAccountDeletion, CancelAccountDeletion and GetAccountDeletionStatus reach team-identity for a
  signed-in user; the internal obligation RPCs answer 501 (change identity-account-deletion).

  Scenario: Signed-in user reaches the RPC
    Given a signed-in buyer "A" with no obligations
    When "A" reads the deletion status
    Then the gateway answers 200 with the status body

  Scenario: Anonymous request is rejected at the edge
    When a caller without a token requests account deletion through the gateway
    Then the gateway answers 401

  Scenario: Internal obligation RPCs are not exposed
    Given a signed-in buyer "A" with no obligations
    When "A" and an anonymous caller post GetUserObligations and GetPayoutObligations through the gateway
    Then every one of those gateway calls answers 501
