## Purpose

Defines the browser tracking layer in team-frontend: how events are queued, batched and delivered without blocking
the user, when an impression counts, how duplicate impressions are suppressed, how the anonymous visitor is
identified and stitched at login, and which surfaces are instrumented.

## ADDED Requirements

### Requirement: Events are batched and delivered without blocking the user

The SDK SHALL queue events in memory and deliver them to `POST /api/track` in batches of at most 20 events, at
least every 5 seconds while events are pending, and SHALL flush the queue with `navigator.sendBeacon` (falling back
to a `keepalive` fetch) when the page becomes hidden or is unloaded. Delivery SHALL never throw into, delay or
fail a user action, and SHALL be a no-op during server rendering.

#### Scenario: Several events go out as one batch

- **WHEN** a visitor triggers 6 tracked events within 2 seconds
- **THEN** the gateway receives one request carrying those 6 events in order, each with its own
  `client_event_id`

#### Scenario: Pending events survive navigation away

- **WHEN** a visitor triggers events and then closes the tab before the flush interval elapses
- **THEN** the pending events are sent with `sendBeacon` on page hide and land in the warehouse

#### Scenario: A failing collector never breaks browsing

- **WHEN** the gateway collector is unreachable while the visitor browses, adds to cart and checks out
- **THEN** every user action completes normally and no error is shown

### Requirement: An impression counts only when the card was actually seen

The SDK SHALL emit an `IMPRESSION` for a card in a placement only after at least 50% of the card has been inside
the viewport continuously for at least 1 second, and SHALL emit at most one impression per
(`request_id`, `listing_id`) per page view, however often the card scrolls in and out.

#### Scenario: A card below the fold is not counted until seen

- **WHEN** a search returns 20 results and the visitor sees only the first 8 without scrolling
- **THEN** impressions exist for positions 1 to 8 only

#### Scenario: A brief flash is not an impression

- **WHEN** a card is scrolled past and stays at least 50% visible for less than 1 second
- **THEN** no impression is emitted for it

#### Scenario: Scrolling back does not duplicate an impression

- **WHEN** a card has been impressed, scrolled out of view and scrolled back in
- **THEN** exactly one impression exists for that (`request_id`, `listing_id`)

### Requirement: The anonymous visitor id is a first-party random cookie

The SDK SHALL identify a visitor with a random UUIDv4 `anonymous_id` stored in a first-party cookie (`bds_aid`,
`SameSite=Lax`, `Secure` outside local, lifetime 13 months, refreshed on use), readable by server rendering so
anonymous serving requests can carry it, and a `session_id` that rotates after 30 minutes of inactivity. An id
already present in `localStorage` from the previous SDK SHALL be adopted once. No identifier SHALL be derived from
device, browser or network characteristics.

#### Scenario: The same visitor keeps one anonymous id across visits

- **WHEN** an anonymous visitor browses, leaves, and returns the next day in the same browser
- **THEN** events from both visits carry the same `anonymous_id` and different `session_id` values

#### Scenario: Server rendering sees the anonymous id

- **WHEN** an anonymous visitor with an `anonymous_id` cookie loads the home page
- **THEN** the home `Recommend` request made during server rendering carries that `anonymous_id`

### Requirement: Login stitches the anonymous visitor to the user

After a successful login or registration the SDK SHALL emit one `IDENTIFY` event carrying the current
`anonymous_id`; the authenticated user is taken from the verified session at the edge, never from the event body.

#### Scenario: Pre-login browsing is linked to the user

- **WHEN** an anonymous visitor views two listings, then logs in as buyer B
- **THEN** the warehouse holds one identity link between that `anonymous_id` and B, and B's resolved activity
  includes the two pre-login views

### Requirement: Key surfaces are instrumented without visual change

The frontend SHALL emit: `PAGE_VIEW` on every route change; `IMPRESSION` and `CLICK` with attribution on home rows
(`home.for_you`, `home.trending`, `home.recently_viewed`, `home.flash_sale`), search results (`search.results`),
PDP similar items (`pdp.similar`) and cart cross-sell (`cart.cross_sell`) when shown; `SEARCH` once per submitted
search with scrubbed query, filters and `result_count`; `VIEW` on a PDP with `dwell_ms` sent when the visitor leaves
it; `ADD_TO_CART` and `REMOVE_FROM_CART` with quantity; `CHECKOUT_START` when checkout opens with its listing ids;
`SHARE` with channel. No layout, style or copy change other than the opt-out control.

#### Scenario: A PDP view reports how long it was looked at

- **WHEN** a visitor opens a listing page, stays about 8 seconds and navigates away
- **THEN** the warehouse holds a `VIEW` for that listing with `dwell_ms` between 7000 and 10000

#### Scenario: Checkout start lists what is being bought

- **WHEN** a buyer with two listings in the cart opens checkout
- **THEN** one `CHECKOUT_START` lands carrying both listing ids and the cart item count

#### Scenario: A search is recorded once with its result count

- **WHEN** a buyer submits the search "áo khoác" with a category filter and gets 12 results
- **THEN** exactly one `SEARCH` event lands with that query, the category filter and `result_count = 12`
