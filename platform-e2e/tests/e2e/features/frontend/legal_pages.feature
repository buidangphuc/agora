@buyer @legal_pages
Feature: Legal pages and footer (change frontend-legal-pages)
  Seven public static Vietnamese legal pages and a consumer footer that links them and carries the
  operator placeholder block. Every page check is anonymous: no session cookie, no redirect.

  Scenario: A legal page renders title, version and placeholder
    When an anonymous visitor opens the legal page "dieu-khoan-su-dung"
    Then the response is 200 with a Vietnamese h1, a "Phiên bản" version of 1 to 32 characters and the legal placeholder body

  Scenario: All seven slugs resolve
    When an anonymous visitor requests each of the seven legal slugs
    Then each response is 200
    And the sitemap contains exactly those 7 legal URLs

  Scenario: An unknown legal slug is not found
    When an anonymous visitor opens the legal page "chinh-sach-khong-ton-tai"
    Then the response is 404 and an HTML not-found page renders without a legal title
    And the legal page "dieu-khoan-su-dung" still answers 200

  Scenario: Legal pages need no login
    When an anonymous visitor opens the legal page "chinh-sach-bao-mat"
    Then the response is 200 with no redirect to "/login"

  Scenario: Footer exposes the legal links
    When an anonymous visitor opens the home page
    Then the footer contains 7 links whose targets are the seven legal slugs
    And the footer RMA link target is "/chinh-sach-doi-tra"

  Scenario: Footer shows the operator placeholders
    When an anonymous visitor opens the home page
    Then the footer shows the four labelled operator fields each with the operator placeholder

  Scenario: Footer never links legal pages that are missing
    When an anonymous visitor opens the home page
    Then every footer legal link resolves to a 200 page
    And no footer link for the RMA entry points to "/account/orders"
