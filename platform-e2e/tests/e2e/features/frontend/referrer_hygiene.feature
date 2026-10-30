@admin @auth
Feature: Admin and login URLs do not leak through referrers or analytics (frontend-referrer-hygiene)
  Admin URLs carry emails and ids in the query. They must not reach third parties through the
  Referer header, GA4 page_location / page_referrer, or a guest returnUrl.

  Scenario: Referrer hygiene: every response carries the policy header
    When the storefront is requested at "/", "/login" and "/admin/audit" without following redirects
    Then every response has the header Referrer-Policy "strict-origin-when-cross-origin"
    And every response still has a Content-Security-Policy header

  Scenario: Referrer hygiene: a weaker Referrer-Policy value is not configured
    When the storefront is requested at "/", "/login" and "/admin/audit" without following redirects
    Then each response has exactly one Referrer-Policy value and it is not a weaker policy
    And every response still has a Content-Security-Policy header

  Scenario: Referrer hygiene: admin audit page declares no-referrer
    Given an admin is logged in
    When the admin opens "/admin/audit?target_id=u-1"
    Then the page is the audit viewer
    And the document head contains the meta referrer "no-referrer"

  Scenario: Referrer hygiene: every admin page and login declare no-referrer
    Given an admin is logged in
    When the admin opens each of the admin pages cockpit, users, audit, listings and cases
    Then every admin page contains the meta referrer "no-referrer"
    When an anonymous visitor opens "/login" in a fresh browser context
    Then the document head contains the meta referrer "no-referrer"

  Scenario: Referrer hygiene: non-admin 403 view on an admin URL also sends no referrer
    Given a registered buyer for referrer hygiene
    And the referrer-hygiene buyer is signed in in the browser
    When the admin page "/admin/users?query=x" is opened by the signed-in buyer
    Then the 403 view "Không có quyền" is rendered without a redirect to login
    And the document head contains the meta referrer "no-referrer"

  Scenario: Referrer hygiene: no admin query reaches a cross-origin request header
    Given an admin is logged in
    When the admin opens "/admin/audit?target_id=u-1" and the page issues a cross-origin request
    Then the cross-origin request was observed
    And that request carries no Referer header
    When the admin opens "/" and the page issues a cross-origin request
    Then the cross-origin request was observed
    And that request carries a Referer header that is only the site origin

  Scenario: Referrer hygiene: dataLayer after an admin visit has no admin query
    Given the visitor has granted analytics consent
    And an admin is logged in
    When the admin opens the audit page with a target query and then navigates to the users search with an email query while recording the dataLayer
    Then page_location entries were pushed for the audit page and for the users page
    And no dataLayer page_location or page_referrer contains "target_id", "query=" or "a@b.vn"

  Scenario: Referrer hygiene: guest on admin users search gets path-only returnUrl
    When an anonymous visitor requests "/admin/users?query=a%40b.vn&state=banned"
    Then the visitor is on the login page with returnUrl "/login?returnUrl=%2Fadmin%2Fusers"
    And the location contains no "query" and no "a%40b.vn"

  Scenario: Referrer hygiene: non-admin guard still preserves the query
    When an anonymous visitor requests "/chat?thread=t1&tag=a&tag=b"
    Then the visitor is on the login page with returnUrl "/login?returnUrl=%2Fchat%3Fthread%3Dt1%26tag%3Da%26tag%3Db"

  Scenario: Referrer hygiene: signed-in non-admin still sees the 403 view
    Given a registered buyer for referrer hygiene
    And the referrer-hygiene buyer is signed in in the browser
    When the admin page "/admin/users?query=x" is opened by the signed-in buyer
    Then the 403 view "Không có quyền" is rendered without a redirect to login
