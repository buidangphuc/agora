@tracking @analytics @freshVisitor
Feature: Consent banner, Consent Mode v2 gating of GTM, and edge consent stamping
  A first-time visitor sees a consent banner and no Google host is contacted until analytics is
  granted. The choice lives in the first-party agora_consent cookie, GTM only loads after an
  analytics grant, first-party beacons flow regardless, and the edge stamps TrackingEvent.consent
  from the cookie alone, never from the beacon body. Google hosts are mocked in e2e.

  Scenario: First visit shows banner and contacts no Google host
    When a visitor with no consent cookie opens the home page
    Then the consent banner is visible with analytics, marketing and personalization groups
    And the DOM has 0 googletagmanager.com scripts
    And the browser issued 0 requests to googletagmanager.com and google-analytics.com

  Scenario: Accept analytics loads GTM and updates consent
    When a visitor with no consent cookie opens the home page
    And the visitor clicks "Accept all" on the consent banner
    Then window.dataLayer holds a consent default with every signal denied followed by a consent update with analytics_storage granted
    And exactly 1 GTM script is injected
    And the consent banner is removed from the DOM

  Scenario: Rejected analytics never loads GTM
    When a visitor with no consent cookie opens the home page
    And the visitor clicks "Reject all" on the consent banner
    And the visitor reloads the page and waits for the tracking queue idle window
    Then the DOM has 0 googletagmanager.com scripts
    And the browser issued 0 requests to googletagmanager.com and google-analytics.com

  @needsListing
  Scenario: Rejected visitor still sends a beacon
    Given a listing has been seeded via the API
    When a visitor with no consent cookie opens the home page
    And the visitor clicks "Reject all" on the consent banner
    And the visitor views the seeded listing and waits for the tracking queue idle window
    Then at least one POST to /api/track was sent
    And no beacon body has a consent key

  @needsListing
  Scenario: Consent change applies to later beacons only
    Given a listing has been seeded via the API
    When a visitor with no consent cookie opens the home page
    And the visitor clicks "Reject all" on the consent banner
    And the visitor views the seeded listing and waits for the tracking queue idle window
    And the visitor grants personalization through the Cookie settings link
    And the visitor views the seeded listing and waits for the tracking queue idle window
    Then the events published for beacons sent before the change carry personalization false
    And the events published for beacons sent after the change carry personalization true and version 1

  @needsListing
  Scenario: Rejected buyer is flagged personalization-denied
    Given a listing has been seeded via the API
    When a visitor with no consent cookie opens the home page
    And the visitor clicks "Reject all" on the consent banner
    And the visitor views the seeded listing and waits for the tracking queue idle window
    Then every event published for the seeded listing carries consent personalization false and version 1

  Scenario: Body consent is ignored
    When a beacon whose body claims personalization true arrives with cookie "v=1&a=0&m=0&p=0&ts=1760000000"
    Then the response is 202
    And the published event carries consent personalization false and version 1

  Scenario: No cookie stamps version 0
    When a beacon arrives with no agora_consent cookie
    Then the response is 202
    And the published event carries a consent message with all flags false and version 0
