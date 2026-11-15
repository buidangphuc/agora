@buyer @legal_reacceptance @legal_consent
Feature: Users with stale acceptance are asked to accept again (change frontend-legal-pages)
  A signed-in user whose latest TERMS or PRIVACY_POLICY record is absent or differs from the current
  configured version sees a blocking dialog on every non-legal route. Legacy users are seeded through
  real Register calls, then their consent rows are removed. A user "at a newer or older version"
  is seeded by registering under the changed-versions identity stack.

  Scenario: A legacy user is prompted and accepts
    Given a legacy user without TERMS or PRIVACY_POLICY consent rows is signed in
    When the user loads the home page
    Then the re-acceptance dialog with "Đồng ý" and "Đăng xuất" is visible
    When the user clicks "Đồng ý" in the re-acceptance dialog
    Then the gateway received 1 RecordConsent call
    And the re-acceptance dialog closes within 1 second
    And the ledger holds TERMS and PRIVACY_POLICY records at the current versions with source "account"
    When the user reloads the page
    Then no re-acceptance dialog shows

  @destructive @legal_versions_override
  Scenario: A newer Terms version triggers the prompt
    Given team-identity runs with LEGAL_TERMS_VERSIONS "2026-11,2026-05"
    And a user registered at the identity current terms version is signed in
    When the user loads the home page
    Then the re-acceptance dialog with "Đồng ý" and "Đăng xuất" is visible
    When the user opens the legal page "/dieu-khoan-su-dung" in the browser
    Then the legal page renders and no re-acceptance dialog shows

  Scenario: Up-to-date users are not prompted
    Given a user registered at the current versions is signed in
    When the user loads 3 different pages in one tab
    Then no re-acceptance dialog shows
    And the gateway received at most 1 ListMyConsents call

  Scenario: Anonymous visitors are never prompted
    When an anonymous visitor opens the home page in the browser
    Then no re-acceptance dialog shows
    And the gateway received 0 PrivacyService calls

  @destructive
  Scenario: Consent lookup failure does not block browsing
    Given a user registered at the current versions is signed in
    And the consent table is temporarily unavailable to team-identity
    When the user loads the home page
    Then no re-acceptance dialog shows
    And the page renders normally with no error shown

  Scenario: Declining signs the user out
    Given a legacy user without TERMS or PRIVACY_POLICY consent rows is signed in
    When the user loads the home page
    And the user clicks "Đăng xuất" in the re-acceptance dialog
    Then the session cookies are cleared and the user is on "/" as anonymous
    And the legacy user has no TERMS or PRIVACY_POLICY consent row
