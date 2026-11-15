@auth @data_export
Feature: A signed-in user exports the data team-identity holds about them
  PrivacyService.ExportMyData returns one bounded JSON document for the caller only, without
  credentials, never cacheable (change identity-data-export).

  Scenario: Export lists every identity-held record of the caller
    Given an export buyer "A" with 2 addresses, 3 sessions, 5 login events and 4 consent records
    When "A" exports their data through the gateway
    Then the export is OK and holds 2 addresses, 3 sessions, 5 login events and 4 consents
    And the export account user id is the id of "A"

  Scenario: Export never contains credentials
    Given an export buyer "A" with a live refresh token, an unused handoff code and a status reason
    When "A" exports their data through the gateway
    Then the export bytes contain none of the password hash, the refresh token, the handoff code or the status reason
    And the export top-level keys are exactly the contracted nine

  Scenario: Consents are listed newest first with deletion state
    Given an export buyer "A" who changed analytics consent after accepting terms and requested account deletion
    When "A" exports their data through the gateway
    Then the first export consent is the analytics record and the second is older
    And the export deletion is pending with erase_after 48 hours after the request

  Scenario: Anonymous caller is refused
    When a caller without a token exports data through the gateway
    Then the export call fails unauthenticated

  Scenario: One user never sees another user's data
    Given an export buyer "A" with 2 addresses, 3 sessions, 5 login events and 4 consent records
    And an export buyer "B" with 1 address
    When "B" exports their data through the gateway
    Then the export holds only the address ids of "B"
    And the export account user id is the id of "B"

  Scenario: Login history over the cap is truncated and flagged
    Given an export buyer "A" with 1500 login events
    When "A" exports their data through the gateway
    Then the export holds the newest 1000 login events and truncated.login_history is true
    And the export data is at most 1048576 bytes

  Scenario: The edge response is not cacheable
    Given an export buyer "A" with 0 addresses, 1 sessions, 1 login events and 2 consent records
    When "A" exports their data through the gateway
    Then the export response header Cache-Control equals no-store
