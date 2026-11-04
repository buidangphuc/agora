@auth @handoff
Feature: Cross-app hand-off on the buyer and seller apps
  As a user signed in on one app
  I want the other app to open signed in after one confirmation click
  So that I never see a second login form and a forged link cannot sign me in as someone else

  Scenario: GET shows the confirmation page and consumes nothing
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the seller app receives GET "/auth/handoff" for the code with next "/seller/orders"
    Then the response is 200 containing "Tiếp tục với tài khoản" and the buyer's username
    And the page holds a POST form and sets no cookie
    And the code still redeems through the gateway

  Scenario: HEAD and prefetch never consume
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the seller app receives a HEAD and a prefetch GET for the code
    Then the code is still unredeemed in the table
    And the code still redeems through the gateway

  Scenario: POST redeems and redirects without the code
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form of the seller app is submitted from its own origin with next "/seller/orders"
    Then the response is a 303 to "/seller/orders"
    And the response sets seller_session and seller_refresh cookies
    And the response has Referrer-Policy "same-origin"
    And the Location contains no code

  Scenario: Forged cross-site POST is refused
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form is posted with headers "Origin: https://evil.example"
    Then the response is a 303 to "/login" with no cookie
    When the confirmation form is posted with headers "Sec-Fetch-Site: cross-site"
    Then the response is a 303 to "/login" with no cookie
    And the code still redeems through the gateway

  Scenario: Origin matching Host but not the configured origin is refused
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form is posted claiming host "seller.attacker.example" with headers "Origin: https://seller.attacker.example"
    Then the response is a 303 to "/login" with no cookie
    And the code still redeems through the gateway

  Scenario: Evil Origin with same-origin Sec-Fetch-Site is refused
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form is posted with headers "Origin: https://evil.example, Sec-Fetch-Site: same-origin"
    Then the response is a 303 to "/login" with no cookie
    And the code still redeems through the gateway

  Scenario: Origin null is refused
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form is posted with headers "Origin: null, Sec-Fetch-Site: same-origin"
    Then the response is a 303 to "/login" with no cookie
    And the code still redeems through the gateway

  Scenario: Refused POST sends no-referrer and no next
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form is posted with next "/seller/orders" with headers "Origin: https://evil.example"
    Then the 303 Location is exactly "/login"
    And the response has Referrer-Policy "same-origin"

  Scenario: Already signed-in user gets the confirmation and the POST replaces the session
    Given a signed-in seller named bob who holds seller cookies
    And a signed-in buyer
    And a fresh seller-target code of the buyer
    When bob opens the hand-off link of the buyer with his seller cookies
    Then the page reads "Tiếp tục với tài khoản" and the buyer's username
    When bob confirms from the seller app's own origin
    Then the seller cookies now belong to the buyer and no longer to bob

  Scenario: POST with neither Origin nor Sec-Fetch-Site is refused
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form is posted with headers ""
    Then the response is a 303 to "/login" with no cookie
    And the code still redeems through the gateway

  Scenario: Same-origin POST via Sec-Fetch-Site is accepted
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form is posted with next "/seller/orders" with headers "Sec-Fetch-Site: same-origin"
    Then the response is a 303 to "/seller/orders"
    And the response sets seller_session and seller_refresh cookies
    And the code no longer redeems through the gateway

  Scenario: Confirmation page cannot be framed or cached
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the seller app receives GET "/auth/handoff" for the code with next "/seller"
    Then the response is 200 containing "Tiếp tục với tài khoản" and the buyer's username
    And the response headers frame-deny, no-store and no-referrer are present

  Scenario: Code is in the POST body only
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the seller app receives GET "/auth/handoff" for the code with next "/seller"
    Then the form action is "/auth/handoff" with no query
    And the code is a hidden field of the form
    And the page references no external script, style, image or font

  Scenario: Username is escaped and truncated
    Given a signed-in user named with markup "<script>alert(1)</script>"
    And a fresh seller-target code of the buyer
    When the seller app receives GET "/auth/handoff" for the code with next "/seller"
    Then the HTML holds "&lt;script&gt;" and no executable script element for the name
    Given a signed-in user named with 500 characters
    And a fresh seller-target code of the buyer
    When the seller app receives GET "/auth/handoff" for the code with next "/seller"
    Then the displayed name is at most 64 characters

  @slow
  Scenario: Dwelling past the TTL falls back to login
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the seller app receives GET "/auth/handoff" for the code with next "/seller/orders"
    And 61 seconds have passed since the first mint
    And the confirmation form of the seller app is submitted from its own origin with next "/seller/orders"
    Then the response is a 303 to "/login?next=/seller/orders" with no cookie

  Scenario: Attacker link needs the victim's click
    Given a signed-in buyer named mallory with a fresh seller-target code
    When a victim's browser opens the link on the seller app
    Then the page names mallory and the browser holds no seller session cookie
    And the code is still unredeemed in the table
    When the victim clicks the confirmation button
    Then the code is redeemed

  @slow
  Scenario: Invalid code on GET falls back to login
    Given a signed-in buyer
    And the invalid codes: used, expired, wrong-target and revoked-source
    And a fresh seller-target code of the buyer
    When each invalid code is opened with GET on the seller app with next "/seller/orders"
    Then every response is a 303 to "/login?next=/seller/orders" with no cookie and no-referrer
    And the valid code opens the confirmation page as a control

  Scenario: Failed redemption on POST falls back to login
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When a POST from the seller app's own origin carries an unknown code with next "/seller/orders"
    Then the response is a 303 to "/login?next=/seller/orders" with no cookie
    And the response has Referrer-Policy "same-origin"
    And the code still redeems through the gateway

  Scenario: Missing or oversized code
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the seller app receives GET without a code
    Then the response is a 303 to a login path with no cookie
    When the seller app receives GET with a 65-character code
    Then the response is a 303 to a login path with no cookie
    And the valid code opens the confirmation page as a control

  Scenario: Unsafe next is neutralised
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the confirmation form of the seller app is submitted from its own origin with next "https://evil.example/x"
    Then the redirect target is a local path and never an external URL
    Given a fresh seller-target code of the buyer
    When the confirmation form of the seller app is submitted from its own origin with next "//evil.example"
    Then the redirect target is a local path and never an external URL
    And the response sets seller_session and seller_refresh cookies

  Scenario: Code minted on click, never at render
    Given a signed-in buyer who is a seller on the buyer app
    When the buyer app renders the home page
    Then team-audit holds no "auth.handoff_mint" entry for the buyer
    And the rendered HTML contains no hand-off code
    When the buyer clicks the seller channel link
    Then team-audit holds one "auth.handoff_mint" entry for the buyer

  Scenario: Admin never hands off
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    When the admin app receives GET "/auth/handoff" for the code with next "/"
    Then the response is 404
    And the code is still unredeemed in the table

  Scenario: Code redacted in access logs
    Given a signed-in buyer
    And a fresh buyer-target code of the buyer
    When the buyer app receives GET "/auth/handoff" for the code with a marker next and the buyer's session
    Then no buyer app log line contains the code
    And the buyer app log line for the marker shows "code=REDACTED"

  Scenario: Rejected hand-off request is still redacted
    When the buyer app receives GET "/auth/handoff" for the invalid code "BAD" with a marker next
    Then the buyer app answers with a 303 to login
    And no buyer app log line contains the invalid code
    And the buyer app log line for the marker shows "code=REDACTED"

  Scenario: Anonymous request is redacted without a session
    Given a signed-in buyer
    And a fresh buyer-target code of the buyer
    When the buyer app receives GET "/auth/handoff" for the code with a marker next and no session
    Then no buyer app log line contains the code
    And the buyer app log line for the marker shows "code=REDACTED"

  Scenario: Other query parameters keep their value
    Given a signed-in buyer
    And a fresh buyer-target code of the buyer
    When the buyer app receives GET "/auth/handoff" for the code with a marker next and no session
    Then the buyer app log line for the marker still shows the marker next value

  Scenario: Buyer to seller signed in
    Given a signed-in buyer who is a seller on the buyer app
    When the buyer clicks the seller channel link
    And the browser reaches the seller app confirmation page
    And the buyer clicks the confirmation button
    Then the browser ends on a seller app page without a login form
    And the seller_session cookie is set and the URL holds no code

  Scenario: Seller to buyer signed in
    Given a signed-in seller on the seller app
    When the seller clicks the link to the buyer storefront
    And the browser reaches the buyer app confirmation page
    And the buyer clicks the confirmation button
    Then the browser ends on the buyer app signed in
    And the session cookie is set and the URL holds no code

  Scenario: Reused link shows the login form
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And the hand-off link is opened and confirmed in a browser
    When the same hand-off URL is opened in a fresh browser context
    Then the seller app shows its login form and sets no seller_session cookie

  @slow
  Scenario: Expired link shows the login form
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And 61 seconds have passed since the first mint
    When the hand-off URL is opened in a browser
    Then the seller app shows its login form and sets no seller_session cookie

  Scenario: Code for the other app shows the login form
    Given a signed-in buyer
    And a fresh buyer-target code of the buyer
    When the hand-off URL is opened in a browser on the seller app
    Then the seller app shows its login form and sets no seller_session cookie

  Scenario: Source logged out before click
    Given a signed-in buyer
    And a fresh seller-target code of the buyer
    And the source session is logged out
    When the hand-off URL is opened in a browser
    Then the seller app shows its login form and sets no seller_session cookie

  Scenario: Cap reached still navigates
    Given a signed-in buyer who is a seller on the buyer app
    And the buyer has minted 10 live codes
    And an eleventh mint by the buyer is refused with "resource_exhausted"
    When the buyer clicks the seller channel link
    Then the browser opens the seller app login with next "/seller"

  Scenario: Logout stays per app
    Given a signed-in buyer who is a seller on the buyer app
    When the buyer clicks the seller channel link
    And the browser reaches the seller app confirmation page
    And the buyer clicks the confirmation button
    And the user logs out of the seller app
    Then the buyer app session cookie is unchanged and still signed in
