@fintech @integration
Feature: team-payment refuses to boot with mock payments in a shared environment
  Run as a black box: the real team-payment image is started on its own (docker run
  --rm, not touching the running stack) with ENV=production and MOCK_PAYMENTS=true.

  Scenario: team-payment refuses mock payments in production
    When the team-payment image is started with ENV "production" and MOCK_PAYMENTS "true"
    Then the process exits non-zero
    And its log names MOCK_PAYMENTS
