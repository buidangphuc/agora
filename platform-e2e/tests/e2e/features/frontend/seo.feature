@buyer @search
Feature: Search-engine metadata, crawl files and indexing policy (frontend-seo)
  Crawlers read raw HTML, so every scenario fetches the page over HTTP as a crawler would and
  asserts on the tags in the response. Seller and admin apps are fetched on their own origins.
  Every "absent" assertion is paired with a positive control on the same response.

  Scenario: SEO-1 A page title ends with one Agora suffix
    When an anonymous crawler fetches "/assistant" on the storefront
    Then the response status is 200
    And the title is "Trợ Lý Mua Sắm AI Thông Minh | Agora"

  Scenario: SEO-2 The home page uses the default title and description
    When an anonymous crawler fetches "/" on the storefront
    Then the response status is 200
    And the title is "Agora | Sàn thương mại điện tử"
    And the meta description is non-empty Vietnamese text
    And the canonical link is "[site]"

  Scenario: SEO-3 A forged Host header does not change the canonical origin
    When a crawler fetches "/" on the storefront claiming the Host "evil.example"
    Then the response status is 200
    And the canonical link and og:url start with "[site]"
    And neither the canonical link nor og:url contains "evil.example"

  Scenario: SEO-4 A listing page has title, description, canonical and Open Graph tags
    Given a published listing with an image
    When an anonymous crawler fetches "/listing/[listing]" on the storefront
    Then the response status is 200
    And the title is "[listing title] | Agora"
    And the meta tag og:type is "product"
    And the meta tag og:site_name is "Agora"
    And the canonical link is "[site]/listing/[listing]"
    And the meta tag og:url is "[site]/listing/[listing]"
    And the meta tag og:image is an absolute URL of the first image

  Scenario: SEO-5 A missing listing answers 404 without Open Graph tags
    When an anonymous crawler fetches "/listing/does-not-exist" on the storefront
    Then the response status is 404
    And the page has no og meta tag and no canonical link

  Scenario Outline: SEO-44 A held, banned or draft listing answers 404 without metadata
    Given a listing in status "<status>"
    When an anonymous crawler fetches "/listing/[listing]" on the storefront
    Then the response status is 404
    And the page has no og meta tag and no canonical link

    Examples:
      | status |
      | held   |
      | banned |
      | draft  |

  Scenario: SEO-6 A long description is cut at 160 characters
    Given a published listing whose description is 400 characters over several lines
    When an anonymous crawler fetches "/listing/[listing]" on the storefront
    Then the response status is 200
    And the meta description and og:description start with the listing description
    And the meta description and og:description hold at most 160 characters and no line break

  Scenario: SEO-7 A listing without an image omits og:image
    Given a published listing without an image
    When an anonymous crawler fetches "/listing/[listing]" on the storefront
    Then the response status is 200
    And the meta tag og:title is "[listing title] | Agora"
    And the meta tag og:type is "product"
    And the page has no og:image meta tag

  Scenario: SEO-9 A shop page uses the shop name and banner
    Given a seller with the shop name "Cửa hàng Mai" and a storefront banner
    When an anonymous crawler fetches "/shop/[shop]" on the storefront
    Then the response status is 200
    And the title is "Cửa hàng Mai | Agora"
    And the meta tag og:image is the storefront banner URL
    And the canonical link is "[site]/shop/[shop]"

  Scenario: SEO-12 A category page is indexable with a clean canonical
    Given an existing category
    When an anonymous crawler fetches "/search?category=[category]&sort=price_asc&page=2" on the storefront
    Then the response status is 200
    And the title is "[category name] | Agora"
    And the canonical link is "[site]/search?category=[category]"
    And the page has no noindex robots tag

  Scenario: SEO-13 A free-text search is noindex and follow
    When an anonymous crawler fetches "/search?q=%3Cscript%3Ex%3C%2Fscript%3E" on the storefront
    Then the response status is 200
    And the robots meta is "noindex, follow"
    And the page has no canonical link
    And the query text appears in no title or og meta tag

  Scenario: SEO-35 Plain search is indexable with its own title and description
    When an anonymous crawler fetches "/search" on the storefront
    Then the response status is 200
    And the title is "Tất cả sản phẩm | Agora"
    And the meta description is "Tìm và mua sản phẩm từ nhiều người bán trên Agora."

  Scenario: SEO-16 An anonymous crawler gets noindex on the private pages that render
    When an anonymous crawler fetches each of "/login" and "/register" on the storefront
    Then each response status is 200
    And each response robots meta is "noindex, nofollow"
    And each response has no og meta tag

  Scenario: SEO-17 A logged-in buyer gets noindex on the guest-redirected pages
    Given a signed-in buyer
    When the signed-in buyer fetches each of "/cart", "/favorites", "/notifications", "/chat", "/account/orders" and "/account/addresses" on the storefront
    Then each response status is 200
    And each response robots meta is "noindex, nofollow"
    And each response has no og meta tag

  Scenario: SEO-21 robots.txt disallows private prefixes and lists the sitemap
    When an anonymous crawler fetches "/robots.txt" on the storefront
    Then the response status is 200
    And the response content type is "text/plain"
    And robots.txt disallows each of the twelve private prefixes
    And the last line of robots.txt is "Sitemap: [site]/sitemap.xml"

  Scenario: SEO-30 An anonymous crawler is told to stay out of seller and admin prefixes
    When an anonymous crawler fetches "/robots.txt" on the storefront
    Then the response status is 200
    And robots.txt has the line "Disallow: /seller"
    And robots.txt has the line "Disallow: /admin"

  Scenario: SEO-23 The sitemap lists home, categories, listings and shops
    Given 4 published listings of 2 sellers created one after another
    When the crawler fetches "/sitemap.xml" once the four listings appear
    Then the response status is 200
    And the response content type is "application/xml"
    And the sitemap holds the home URL and every category URL
    And the sitemap lists the four listings newest first
    And the sitemap lists each of the two shops exactly once
    And every sitemap location is absolute on "[site]"

  Scenario: SEO-24 The sitemap omits unpublished listings
    Given a published listing and a banned listing and a held listing
    When the crawler fetches "/sitemap.xml" once the published listing appears
    Then the sitemap does not list the banned listing
    And the sitemap does not list the held listing
    And the sitemap lists no private prefix

  @slow
  Scenario: SEO-36 A cold sitemap with 5000 listings is measured
    Given 5000 published listings are seeded
    When the crawler fetches "/sitemap.xml" once 5000 listing URLs appear
    Then the response status is 200
    And the sitemap holds exactly 5000 listing URLs
    And the cold-render duration is recorded in the run output

  Scenario: SEO-27 No public page shows a demo brand string
    When a visitor opens "/" and "/search" in the browser
    Then no page text or title contains "Marketplace Showcase", "Polyrepo" or "BATDONGSAN SINH VIÊN"

  Scenario: SEO-29 The favicon is served
    When an anonymous crawler fetches "/" on the storefront
    Then the icon link href of the page answers 200 with an image content type

  Scenario: SEO-37 A seller page is noindex and titled with one Agora suffix
    Given a signed-in seller
    When the signed-in seller fetches "/seller/wallet" on the seller app
    Then the response status is 200
    And the title is "Ví người bán | Agora"
    And the robots meta is "noindex, nofollow"

  Scenario: SEO-38 The seller login page is noindex for an anonymous crawler
    When an anonymous crawler fetches "/login" on the seller app
    Then the response status is 200
    And the robots meta is "noindex, nofollow"
    And the title ends with exactly one " | Agora"

  Scenario: SEO-39 An admin page is noindex, titled once and keeps no-referrer
    Given a signed-in admin
    When the signed-in admin fetches "/admin/audit" on the admin app
    Then the response status is 200
    And the title is "Nhật ký kiểm toán | Agora"
    And the robots meta is "noindex, nofollow"
    And the referrer meta is "no-referrer"

  Scenario: SEO-40 The admin login and forbidden pages deny indexing for an anonymous crawler
    When an anonymous crawler fetches each of "/login" and "/forbidden" on the admin app
    Then each response status is 200
    And each response robots meta is "noindex, nofollow"
    And each response referrer meta is "no-referrer"
