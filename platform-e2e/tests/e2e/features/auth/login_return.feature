@auth @buyer
Feature: Login returns the shopper to where they were (frontend-login-return)
  As a shopper sent to the login page by a protected page or a purchase button
  I want to land back on the page I came from after signing in
  So that I do not lose my place, without ever being redirected off-site

  Scenario: Successful login returns to the requested page
    Given a registered buyer for return-to-page
    When the buyer opens the login page with returnUrl "/listing/abc?variant=v2" and signs in
    Then the browser lands on "/listing/abc?variant=v2"

  Scenario: Tampered returnUrl is rejected server-side
    Given a registered buyer for return-to-page
    When the buyer opens the login page with returnUrl "https://evil.example" and signs in
    Then the browser lands on the home page of the storefront and never on an external host

  Scenario: Guest opening a protected page is sent to login with its path
    When an anonymous visitor requests "/account/orders/o-9"
    Then the visitor is on the login page with returnUrl "/login?returnUrl=%2Faccount%2Forders%2Fo-9"

  Scenario: Query string of the guarded page is preserved with repeated keys in order
    When an anonymous visitor requests "/chat?thread=t1&tag=a&tag=b"
    Then the visitor is on the login page with returnUrl "/login?returnUrl=%2Fchat%3Fthread%3Dt1%26tag%3Da%26tag%3Db"

  Scenario: Param-less layout uses the x-pathname header
    When an anonymous visitor requests "/seller/orders"
    Then the visitor is on the login page with returnUrl "/login?returnUrl=%2Fseller%2Forders"

  Scenario: Over-length path falls back to plain login
    When an anonymous visitor requests "/chat?thread=" followed by 2100 characters
    Then the visitor is on the plain login page without a returnUrl
    When an anonymous visitor requests a chat path whose path plus query is exactly 2048 characters
    Then the visitor is on the login page whose returnUrl is that same path

  Scenario: Cart page guard covers update and remove
    When an anonymous visitor requests "/cart"
    Then the visitor is on the login page with returnUrl "/login?returnUrl=%2Fcart"

  Scenario: Signed-in user without the scope is not redirected to login
    Given a registered buyer for return-to-page
    And the buyer is signed in in the browser
    When the signed-in buyer requests "/seller/new"
    Then the page shows "Cần tài khoản Người Bán"
    And the browser is still on "/seller/new"

  Scenario: Returned user passes through the guard after login
    Given a registered buyer for return-to-page
    When an anonymous visitor requests "/favorites"
    And the buyer signs in on the login page that opened
    Then the browser lands on "/favorites"

  Scenario: Guest add-to-cart goes to login with the variant
    Given a published listing with variants and a guest on its page with the 256GB variant selected
    When the guest clicks Thêm vào giỏ
    Then the guest is on the login page for the listing with the selected variant

  Scenario: Add-to-cart is not replayed after login
    Given a published listing with variants and a guest on its page with the 256GB variant selected
    And a registered buyer for return-to-page
    When the guest clicks Thêm vào giỏ
    And the buyer signs in on the login page that opened
    Then the browser lands on the listing page with the selected variant
    And no add-to-cart call was issued after login
    And the buyer's cart is empty
    When the shopper clicks Thêm vào giỏ again
    Then the buyer's cart holds the listing

  Scenario: Guest buy-now goes to login, then lands on the PDP
    Given a published listing with variants and a guest on its page with the 256GB variant selected
    And a registered buyer for return-to-page
    When the guest clicks Mua ngay while recording page visits
    Then the guest is on the login page for the listing with the selected variant
    When the buyer signs in on the login page that opened
    Then the browser lands on the listing page with the selected variant
    And checkout was never visited
    And no add-to-cart call was issued after login
    And the buyer's cart is empty
