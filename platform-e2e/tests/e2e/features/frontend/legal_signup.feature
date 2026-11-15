@auth @buyer @legal_signup @legal_consent
Feature: Sign-up requires accepting the Terms and the Privacy Policy (change frontend-legal-pages)
  The register form shows two unchecked boxes; the account and its two consent rows exist only when
  both are accepted at the versions the identity service currently requires.

  Scenario: Registering with both boxes checked records two consents
    Given the "register" page is open
    Then the Terms and Privacy checkboxes are unchecked and link to their pages in a new tab
    When the visitor submits the register form with both boxes checked
    Then the user lands on the home page
    And ListMyConsents for the new user returns the TERMS and PRIVACY_POLICY records at the current versions with source "register"

  Scenario: Unchecked box blocks submission
    Given the "register" page is open
    When the visitor submits the register form with the Terms box unchecked
    Then a field error is shown on the register form and the browser stays on "/register"
    And the gateway received 0 Register calls
    And no account exists for the submitted username

  Scenario: A forged post without acceptance fields is refused
    Given the "register" page is open
    When the register form is submitted with both boxes checked but the acceptance fields stripped from the post
    Then a field error is shown on the register form and the browser stays on "/register"
    And the gateway received 0 Register calls
    And no account exists for the submitted username

  @destructive @legal_versions_override
  Scenario: A stale version is reported to the user
    Given team-identity runs with LEGAL_TERMS_VERSIONS "2026-11,2026-05"
    And the "register" page is open
    When the visitor submits the register form with both boxes checked
    Then the form shows "Điều khoản đã được cập nhật. Vui lòng tải lại trang và đồng ý lại."
    And no account exists for the submitted username
