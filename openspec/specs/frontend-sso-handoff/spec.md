# frontend-sso-handoff Specification

## Purpose
TBD - created by archiving change identity-sso-handoff. Update Purpose after archive.

## Requirements

### Requirement: Session package hand-off support

`fe-platform/packages/session` SHALL provide a `startHandoff` server action that, on a user click, mints a code
through the gateway for the signed-in user and returns the target app's `/auth/handoff?code=<code>&next=<path>` URL,
and a route handler factory for `/auth/handoff`. GET SHALL peek the code and render a confirmation page "Tiếp tục với
tài khoản <username>?" with a submit button; only POST SHALL redeem server-side, set that app's own cookies and
redirect (303) to `next` without the code. GET and HEAD SHALL NOT consume the code. The code SHALL travel in the POST body, never in the form action URL. POST SHALL be accepted only from the app's own origin, being its configured public origin and never the request Host or Forwarded headers: when `Origin` is present it MUST equal the own origin (`null` is refused), otherwise `Sec-Fetch-Site` MUST be `same-origin`; any other POST SHALL respond 303 to `/login` carrying no `next` and `Referrer-Policy: no-referrer`, with no redeem call and no cookie. A user already signed in to the target app SHALL still get the confirmation page, and the POST SHALL replace that app's session with the new account. The 200 page SHALL send `Cache-Control: no-store`, `Content-Security-Policy: frame-ancestors 'none'` and `X-Frame-Options: DENY`, load no external resource, and HTML-escape and truncate the username. The confirmation page SHALL send `Referrer-Policy: same-origin` both as a header and as a `<meta name="referrer">` element, so that a same-origin form POST carries the real `Origin` and a cross-origin navigation gets no referrer; every other response SHALL carry
`Referrer-Policy: no-referrer`. Only `buyer` and `seller` presets enable it.

#### Scenario: GET shows the confirmation page and consumes nothing
- **WHEN** the seller app receives `GET /auth/handoff?code=C&next=/seller/orders` for a valid code of user "alice"
- **THEN** the response is 200 containing "Tiếp tục với tài khoản alice?" and a POST form, sets no cookie, and a later redeem of C still succeeds

#### Scenario: HEAD and prefetch never consume
- **WHEN** `HEAD /auth/handoff?code=C` and a request with `Purpose: prefetch` are sent
- **THEN** neither calls the gateway redeem, and C is still redeemable

#### Scenario: POST redeems and redirects without the code
- **WHEN** the confirmation form is submitted and redemption succeeds
- **THEN** the response is 303 to `/seller/orders`, sets `seller_session` and `seller_refresh` cookies, has `Referrer-Policy: no-referrer`, and the Location contains no `code`

#### Scenario: Forged cross-site POST is refused
- **WHEN** a POST to `/auth/handoff` arrives with `Origin: https://evil.example` (or `Sec-Fetch-Site: cross-site`)
- **THEN** the response is 303 to `/login`, the gateway receives no redeem call, and no cookie is set

#### Scenario: Origin matching Host but not the configured origin is refused
- **WHEN** a POST carries `Origin: https://seller.attacker.example` and `Host: seller.attacker.example`, while the configured public origin is `https://seller.example`
- **THEN** the response is 303 to `/login`, the gateway receives no redeem call, and no cookie is set

#### Scenario: Evil Origin with same-origin Sec-Fetch-Site is refused
- **WHEN** a POST carries `Origin: https://evil.example` and `Sec-Fetch-Site: same-origin`
- **THEN** the response is 303 to `/login` and the gateway receives no redeem call

#### Scenario: Origin null is refused
- **WHEN** a POST carries `Origin: null` and `Sec-Fetch-Site: same-origin`
- **THEN** the response is 303 to `/login` and the gateway receives no redeem call

#### Scenario: Refused POST sends no-referrer and no next
- **WHEN** a POST is refused by the origin check
- **THEN** the 303 Location is `/login` without a `next` parameter and the response has `Referrer-Policy: no-referrer`

#### Scenario: Already signed-in user gets the confirmation and the POST replaces the session
- **WHEN** a user signed in as "bob" on the seller app opens a hand-off link of "alice", sees "Tiếp tục với tài khoản alice?" and confirms
- **THEN** the seller cookies afterwards belong to "alice" and no longer to "bob"

#### Scenario: POST with neither Origin nor Sec-Fetch-Site is refused
- **WHEN** a POST to `/auth/handoff` carries neither header
- **THEN** the response is 303 to `/login`, the gateway receives no redeem call, and no cookie is set

#### Scenario: Same-origin POST via Sec-Fetch-Site is accepted
- **WHEN** a POST carries `Sec-Fetch-Site: same-origin` and no `Origin`
- **THEN** the redeem is called once and the response is the 303 to `next`

#### Scenario: Confirmation page cannot be framed or cached
- **WHEN** the 200 confirmation page is served
- **THEN** it has `Content-Security-Policy: frame-ancestors 'none'`, `X-Frame-Options: DENY`, `Cache-Control: no-store` and `Referrer-Policy: same-origin` (header and meta), and a cross-origin navigation from it carries no referrer

#### Scenario: Code is in the POST body only
- **WHEN** the confirmation page is rendered for code C
- **THEN** the form `action` is `/auth/handoff` with no query containing C, C is in a hidden body field, and the page references no external script, style, image or font

#### Scenario: Username is escaped and truncated
- **WHEN** the peeked username is `<script>alert(1)</script>` or 500 characters long
- **THEN** the HTML holds `&lt;script&gt;` and no executable script element, and the displayed name is at most 64 characters

#### Scenario: Dwelling past the TTL falls back to login
- **WHEN** the user opens the confirmation page and clicks the button 61 s after the code was minted
- **THEN** the redeem fails and the response is 303 to `/login?next=<safe next>` with no cookie

#### Scenario: Attacker link needs the victim's click
- **WHEN** a victim opens a link minted by user "mallory"
- **THEN** the page names "mallory", the victim's browser holds no new session cookie, and redeem is called only after the victim submits the form

#### Scenario: Invalid code on GET falls back to login
- **WHEN** GET `/auth/handoff` carries a reused, expired, wrong-target or revoked-source code
- **THEN** the response is 303 to `/login?next=<safe next>` with no session cookie and `Referrer-Policy: no-referrer`

#### Scenario: Failed redemption on POST falls back to login
- **WHEN** the POST redeem fails with `UNAUTHENTICATED`
- **THEN** the response is 303 to `/login?next=<safe next>`, sets no session cookie, and carries `Referrer-Policy: no-referrer`

#### Scenario: Missing or oversized code
- **WHEN** `/auth/handoff` is requested with no `code` or a code longer than 64 characters
- **THEN** the handler redirects to `/login` without calling the gateway

#### Scenario: Unsafe next is neutralised
- **WHEN** `next=https://evil.example/x` or `next=//evil.example`
- **THEN** the redirect target is the app's default landing path, never an external URL

#### Scenario: Gateway outage on peek or redeem
- **IF** the gateway answers `UNAVAILABLE`
- **WHEN** the handler runs
- **THEN** it redirects to `/login?next=<safe next>` and no cookie is set
- **VERIFIED BY**: fe-platform `packages/session` `gateway outage falls back to login`

#### Scenario: Code minted on click, never at render
- **WHEN** a page with a cross-app link is server-rendered and then the link is clicked
- **THEN** the gateway receives no `CreateHandoffCode` during render, one call on click, and the rendered HTML contains no code

#### Scenario: Handler never mints for an anonymous visitor
- **WHEN** `startHandoff` runs with no session cookie
- **THEN** it returns the plain target `/login?next=<path>` URL and makes no mint call to the gateway
- **VERIFIED BY**: fe-platform `packages/session` `anonymous startHandoff returns login url`

#### Scenario: Admin never hands off
- **WHEN** the admin preset requests `startHandoff` or the handler
- **THEN** the call throws at configuration time and no admin route `/auth/handoff` exists

#### Scenario: Code never logged or echoed
- **WHEN** the handler runs GET and POST with console capture
- **THEN** the output does not contain the code
- **VERIFIED BY**: fe-platform `packages/session` `never logs the code`

### Requirement: Access logs redact the hand-off code

Ingress and the Next server logs of the buyer and seller apps SHALL redact the `code` query parameter on
`/auth/handoff`.

#### Scenario: Code redacted in access logs
- **WHEN** a request `GET /auth/handoff?code=C&next=/x` passes through the ingress and the app
- **THEN** no ingress or app log line contains C, and the line shows `code=REDACTED`

#### Scenario: Rejected hand-off request is still redacted
- **WHEN** an invalid code request `GET /auth/handoff?code=BAD` fails with a 303 to login
- **THEN** no log line contains BAD and the line shows `code=REDACTED`

#### Scenario: Anonymous request is redacted without a session
- **WHEN** an unauthenticated visitor without a session opens `/auth/handoff?code=C`
- **THEN** no log line contains C

#### Scenario: Other query parameters keep their value
- **WHEN** a request to `/auth/handoff` carries `next=/x`
- **THEN** the log line still shows `next=/x`

### Requirement: Cross-app links open the other app signed in

team-frontend (buyer) and team-frontend-seller SHALL start every cross-app navigation to the other app through
`startHandoff` on click, and SHALL fall back to the plain target URL (its login) when minting fails.

#### Scenario: Buyer to seller signed in
- **WHEN** a signed-in buyer clicks "Kênh người bán" and then clicks the confirmation button on the seller app
- **THEN** the browser ends on the seller app page without a login form, with `seller_session` set and the URL holding no `code`

#### Scenario: Seller to buyer signed in
- **WHEN** a signed-in seller clicks the link to the buyer storefront and confirms
- **THEN** the browser ends on the buyer app signed in, with `session` set and the URL holding no `code`

#### Scenario: Reused link shows the login form
- **WHEN** the same hand-off URL is opened a second time in a fresh browser context
- **THEN** the seller app shows its login form and no `seller_session` cookie is set

#### Scenario: Expired link shows the login form
- **WHEN** a hand-off URL is opened 61 s after it was built
- **THEN** the seller app shows its login form

#### Scenario: Code for the other app shows the login form
- **WHEN** a buyer-target code is opened on the seller app's `/auth/handoff`
- **THEN** the seller app shows its login form

#### Scenario: Source logged out before click
- **WHEN** the buyer logs out in another tab and then opens the already-built hand-off URL
- **THEN** the seller app shows its login form

#### Scenario: Mint failure still navigates
- **IF** minting fails
- **WHEN** the buyer clicks "Kênh người bán"
- **THEN** the browser opens the seller app's `/login?next=<path>` as before
- **VERIFIED BY**: fe-platform `packages/session` `mint failure falls back to the plain login url`

#### Scenario: Cap reached still navigates
- **IF** the user already holds 10 live codes
- **WHEN** the buyer clicks "Kênh người bán"
- **THEN** the browser opens the seller app's `/login?next=<path>`

#### Scenario: Logout stays per app
- **WHEN** the user logs out of the seller app after a hand-off
- **THEN** the buyer app session cookie is unchanged and still signed in
