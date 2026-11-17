@auth @buyer @legal_consent
Feature: Legal versions at registration and the consent ledger
  Register requires the current Terms and Privacy versions and stores them as consent rows;
  PrivacyService records and lists each user's own consent decisions (change identity-legal-consent).

  Scenario: Register with the current versions stores two consent records
    When a visitor registers with the current terms and privacy versions
    Then Register succeeds and ListMyConsents for the new user returns exactly the TERMS and PRIVACY_POLICY records at the current versions

  Scenario: Register without versions is rejected
    When a visitor registers with terms version "" and privacy version "CURRENT_PRIVACY"
    And a visitor registers with terms version "CURRENT" and privacy version ""
    Then every Register call failed INVALID_ARGUMENT
    And logging in with each of those usernames fails as for an unknown user

  Scenario: Register with a stale version is rejected
    When a visitor registers with terms version "1999-01" and privacy version "CURRENT_PRIVACY"
    Then every Register call failed INVALID_ARGUMENT
    And the Register error names the field "terms_version"
    And logging in with each of those usernames fails as for an unknown user

  @destructive
  Scenario: Consent insert failure rolls back the account
    Given the consent table is temporarily unavailable to team-identity
    When a visitor registers with the current terms and privacy versions
    Then the Register call failed
    When the consent table is available again
    Then registering the same username again succeeds

  Scenario: Registration needs no prior authentication
    When an anonymous caller registers with the current versions and a client-supplied user id
    Then Register succeeds
    And the new user's two consent records belong to the user id in the issued token

  @destructive
  Scenario: Boot fails without a configured version
    When the team-identity image is started with PRIVACY_POLICY_VERSIONS empty
    Then the identity process exits non-zero and its log names PRIVACY_POLICY_VERSIONS

  @destructive @legal_versions_override
  Scenario: First entry is the current version
    Given team-identity runs with LEGAL_TERMS_VERSIONS "2026-11,2026-05"
    Then registering with the old terms version fails INVALID_ARGUMENT
    And registering with the new terms version succeeds

  Scenario: Banner choice stores one row per changed purpose
    Given a signed-in buyer who registered with the current versions
    When the buyer records ANALYTICS granted and MARKETING denied from the banner
    Then RecordConsent succeeds and exactly 2 banner records were added for ANALYTICS and MARKETING

  @destructive @legal_versions_override
  Scenario: Re-accepting a new Terms version appends a row
    Given a buyer registered at the previous terms version
    And team-identity runs with LEGAL_TERMS_VERSIONS "2026-12,2026-11"
    When the buyer records TERMS at the new current version from the account page
    Then the newest TERMS record has the new version and the earlier TERMS record is still listed

  Scenario: Invalid records are rejected without partial writes
    Given a signed-in buyer who registered with the current versions
    When the buyer submits the invalid consent request "seven records"
    And the buyer submits the invalid consent request "duplicate purpose"
    And the buyer submits the invalid consent request "unspecified purpose"
    And the buyer submits the invalid consent request "33-character version"
    And the buyer submits the invalid consent request "unknown source"
    And the buyer submits the invalid consent request "empty list"
    Then every invalid request failed INVALID_ARGUMENT
    And the buyer's record count is unchanged

  Scenario: Stale or withdrawn legal acceptance is rejected
    Given a signed-in buyer who registered with the current versions
    When the buyer records TERMS at a non-current version together with a valid ANALYTICS record
    Then that call failed INVALID_ARGUMENT
    When the buyer records PRIVACY_POLICY with granted false together with a valid MARKETING record
    Then every invalid request failed INVALID_ARGUMENT
    And the buyer's record count is unchanged

  Scenario: Client time and user id are ignored
    Given a signed-in buyer who registered with the current versions
    When the buyer records ANALYTICS with recorded_at in the year 2000
    Then the stored recorded_at is within 5 seconds of the server time
    And the row is listed for the calling user

  Scenario: Anonymous and service principals cannot record consent
    Given a signed-in buyer who registered with the current versions
    When an anonymous caller calls RecordConsent
    And a service principal calls RecordConsent on team-identity
    Then the anonymous call failed UNAUTHENTICATED
    And the service call failed PERMISSION_DENIED
    And no consent row was written

  Scenario: History is newest first and paged
    Given a buyer with 25 consent records
    When the buyer lists consents with page size 10 following next_cursor until it is empty
    Then the pages hold 10, 10 and 5 records
    And the records are in non-increasing recorded_at order with none repeated or missing

  Scenario: A user never sees another user's records
    Given a buyer with 25 consent records
    And another buyer who recorded a banner choice
    Then a user's list holds none of the other buyer's records and their own count is unchanged

  Scenario: Page size is clamped and bad cursors are rejected
    Given a signed-in buyer who registered with the current versions
    When the buyer lists consents with page size 500
    And the buyer lists consents with a malformed cursor
    Then the first call returns at most 100 records and the second fails INVALID_ARGUMENT

  Scenario: Anonymous caller cannot list
    When an anonymous caller calls ListMyConsents
    Then the list call failed UNAUTHENTICATED

  Scenario: A user with no records gets an empty list
    Given a legacy buyer with no consent rows
    When the legacy buyer calls ListMyConsents
    Then the response has 0 records and an empty next_cursor

  Scenario: Consent is reachable through the gateway
    Given a signed-in buyer who registered with the current versions
    When the buyer calls PrivacyService RecordConsent then ListMyConsents through the gateway
    Then both calls succeed and the list contains the recorded banner row

  Scenario: Anonymous edge call is refused
    When an anonymous caller calls RecordConsent
    Then the response is UNAUTHENTICATED
