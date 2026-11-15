@buyer @consent_banner_ledger @freshVisitor
Feature: Vietnamese consent banner that records signed-in choices (change frontend-legal-pages)
  The banner keeps the cookie behaviour of the consent capability; a signed-in commit also writes
  the changed purposes to the consent ledger, an anonymous one never calls PrivacyService.

  Scenario: Banner text is Vietnamese
    When a visitor with no consent cookie opens the home page in the browser
    Then the banner dialog title is "Tùy chọn cookie"
    And the banner groups are labelled "Phân tích", "Tiếp thị" and "Cá nhân hóa"
    And no English label of the previous banner remains

  Scenario: Signed-in save records only changed purposes
    Given a user registered at the current versions is signed in with the consent cookie "a=1&m=0&p=0"
    When the user opens the banner from "Cài đặt cookie", turns on "Cá nhân hóa" and clicks "Lưu lựa chọn"
    Then the consent cookie holds "a=1&m=0&p=1"
    And the gateway received 1 RecordConsent call
    And the newest ledger row is PERSONALIZATION granted with source "banner" and version "1"
    And the ledger gained exactly 1 row after the Register rows

  Scenario: First choice records all three purposes
    Given a user registered at the current versions is signed in with no consent cookie
    When the user clicks "Từ chối tất cả" on the banner
    Then the gateway received 1 RecordConsent call
    And the ledger gained 3 banner rows for ANALYTICS, MARKETING and PERSONALIZATION, all denied

  Scenario: Anonymous choice is cookie only
    When a visitor with no consent cookie opens the home page in the browser
    And the visitor clicks "Chấp nhận tất cả" on the banner
    Then the consent cookie matches "v=1&a=1&m=1&p=1&ts=<unix>"
    And the gateway received 0 PrivacyService calls

  @destructive
  Scenario: Ledger write failure keeps the cookie choice
    Given a user registered at the current versions is signed in with no consent cookie
    And the consent table is temporarily unavailable to team-identity
    When the user clicks "Từ chối tất cả" on the banner
    Then the consent cookie holds "a=0&m=0&p=0"
    And the banner closes and no error dialog is shown
