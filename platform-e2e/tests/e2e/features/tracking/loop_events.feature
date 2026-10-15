@tracking @analytics
Feature: The shop reports activation and loop events once, after the server accepted the action
  Registering, logging in, favoriting, sharing, following a shop, redeeming a referral code,
  checking in daily and subscribing to an alert each emit one TrackingEvent on analytics.events
  (and a GA4 dataLayer entry where the spec names one), only after the server succeeded, never
  for a failure, a removal or a guest (frontend-loop-events).

  # ── Activation ────────────────────────────────────────────────────────

  Scenario: Loop events: registering emits one sign_up event with the new principal
    Given the browser records tracking beacons
    When a visitor registers a new buyer account on the register page and lands on the home page
    Then exactly 1 "sign_up" event is on analytics.events for the principal within 10 seconds
    And the event property "method" equals "password"
    And window.dataLayer holds exactly 1 "sign_up" entry with method "password"

  @needsBuyer
  Scenario: Loop events: logging in emits one login event and a reload does not repeat it
    Given the browser records tracking beacons
    When the buyer logs in on the login page and lands on the home page
    Then exactly 1 "login" event is on analytics.events for the principal within 10 seconds
    And the event property "method" equals "password"
    And the "agora_auth_evt" cookie is absent
    When the page is reloaded twice
    Then exactly 1 "login" event is on analytics.events for the principal within 10 seconds
    And the "agora_auth_evt" cookie is absent

  @needsBuyer
  Scenario: Loop events: failed login and duplicate registration emit nothing
    Given the browser records tracking beacons
    When a visitor submits a wrong password on the login page
    Then the login error is displayed
    And the "agora_auth_evt" cookie is absent
    When another visitor registers the buyer's existing username on the register page
    Then the registration error is displayed
    And the "agora_auth_evt" cookie is absent
    When the visitor opens the home page
    Then zero "login" events are on analytics.events for the recorded sessions after 10 seconds
    And zero "sign_up" events are on analytics.events for the recorded sessions after 10 seconds
    # control: the observation channel does see a real login from this same browser session
    When the buyer logs in on the login page and lands on the home page
    Then exactly 1 "login" event is on analytics.events for the principal within 10 seconds

  # ── Engagement ────────────────────────────────────────────────────────

  @needsBuyer @needsListing
  Scenario: Loop events: favoriting emits one favorite event and the add_to_wishlist dataLayer entry
    Given the browser records tracking beacons
    And a buyer is logged in
    When the buyer clicks the heart on the seeded listing page
    Then exactly 1 "favorite" event for the seeded listing is on analytics.events for the principal within 10 seconds
    And window.dataLayer holds exactly 1 "add_to_wishlist" entry for the seeded listing

  @needsBuyer @needsListing
  Scenario: Loop events: un-favoriting emits nothing
    Given the browser records tracking beacons
    And a buyer is logged in
    When the buyer clicks the heart on the seeded listing page
    And the buyer clicks the heart again to remove the favorite
    Then exactly 1 "favorite" event for the seeded listing is on analytics.events for the principal within 10 seconds
    And the count of "favorite" events for the seeded listing stays 1 after 10 seconds

  @needsBuyer @needsListing
  Scenario: Loop events: sharing emits one share event per copy
    Given the browser records tracking beacons
    And a buyer is logged in
    When the buyer opens the share panel on the seeded listing page
    And the buyer clicks "Sao chép liên kết" once
    Then exactly 2 "share" events for the seeded listing are on analytics.events for the principal within 10 seconds
    And every event property "method" equals "copy_link"
    And no event contains the share short code

  @needsBuyer @needsSeller
  Scenario: Loop events: following a shop emits follow_seller and unfollowing emits nothing
    Given the browser records tracking beacons
    And a buyer is logged in
    When the buyer opens the seeded seller's shop page
    And the buyer follows the shop
    And the buyer unfollows the shop
    Then exactly 1 "follow_seller" event is on analytics.events for the principal within 10 seconds
    And the event property "seller_id" equals the shop id

  @needsBuyer @needsSeller @needsListing
  Scenario: Loop events: a guest cannot produce loop events
    Given the browser records tracking beacons
    When a guest opens the seeded listing page and clicks the heart
    And a guest opens the seeded seller's shop page and clicks follow
    Then the guest is answered Unauthenticated and nothing is favorited or followed
    And zero "favorite" events are on analytics.events for the recorded sessions after 10 seconds
    And zero "follow_seller" events are on analytics.events for the recorded sessions after 10 seconds
    # control: the same browser session signs in and its favorite is observed
    When the buyer is signed in within the same browser session
    And the buyer clicks the heart on the seeded listing page
    Then exactly 1 "favorite" event for the seeded listing is on analytics.events for the principal within 10 seconds

  # ── Account ───────────────────────────────────────────────────────────

  @needsBuyer
  Scenario: Loop events: redeeming a referral code emits referral_redeem without the code
    Given the browser records tracking beacons
    And another user has created a referral code through the API
    And a buyer is logged in
    When the buyer redeems that code on the referral page
    Then exactly 1 "referral_redeem" event is on analytics.events for the principal within 10 seconds
    And no event contains the referral code

  @needsBuyer
  Scenario: Loop events: an invalid referral code emits nothing
    Given the browser records tracking beacons
    And another user has created a referral code through the API
    And a buyer is logged in
    When the buyer redeems the nonexistent code "NOPE-0000-E2E" on the referral page
    Then the referral error is displayed
    And zero "referral_redeem" events are on analytics.events for the principal after 10 seconds
    # control: a valid code from this same session is observed
    When the buyer redeems that code on the referral page
    Then exactly 1 "referral_redeem" event is on analytics.events for the principal within 10 seconds

  @needsBuyer
  Scenario: Loop events: daily check-in emits daily_check_in with the streak
    Given the browser records tracking beacons
    And a buyer is logged in
    When the buyer checks in on the home page and reads the streak
    Then exactly 1 "daily_check_in" event is on analytics.events for the principal within 10 seconds
    And the event property "streak" equals the streak shown on the widget

  @needsBuyer @needsListing
  Scenario: Loop events: subscribing to an alert emits alert_subscribe and unsubscribing emits nothing
    Given the browser records tracking beacons
    And a buyer is logged in
    When the buyer turns on the "price_drop" alert on the seeded listing page
    And the buyer turns the "price_drop" alert off
    Then exactly 1 "alert_subscribe" event for the seeded listing is on analytics.events for the principal within 10 seconds
    And the event property "alert_type" equals "price_drop"
    And the count of "alert_subscribe" events for the seeded listing stays 1 after 10 seconds
