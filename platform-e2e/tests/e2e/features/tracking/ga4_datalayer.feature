@tracking @buyer
Feature: GA4 / GTM data layer pattern
  As the platform, a rendered product list pushes GA4 ecommerce items to
  window.dataLayer and sends its tracking beacons to the edge as one batched
  request, fanned out per item under a shared event group id, loading
  GTM when a container id is configured and without personal data.

  @needsBuyer @needsListing
  Scenario: dataLayer receives ecommerce items
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the home page while recording tracking beacons
    Then window.dataLayer contains a "view_item_list" entry with ecommerce items
    And that dataLayer entry is immediately preceded by an ecommerce null reset

  @needsBuyer @needsListing
  Scenario: One request per rendered list
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the home page while recording tracking beacons
    And the tracking queue idle window elapses
    Then all impression beacons of one item list were sent in exactly 1 POST to "/api/track"
    And that request body is an array of at least 2 beacons

  @needsBuyer @needsListing
  Scenario: Fan-out rows share a group id
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the home page while recording tracking beacons
    And the tracking queue idle window elapses
    Then the impression beacons of one item list have distinct event ids and one non-empty event group id
    And each of those beacons carries its own position and the same item list id

  Scenario: GTM loads when ID is set
    Given the visitor has granted analytics consent
    When a guest opens the home page while recording tracking beacons
    And the tracking queue idle window elapses
    Then exactly 1 script element with id "GTM-E2E0001" in its src is in the DOM
    And window.dataLayer exists

  Scenario: No external request to Google in e2e
    Given the visitor has granted analytics consent
    When a guest opens the home page while recording tracking beacons
    And the tracking queue idle window elapses
    Then the GTM script was served by the intercepted mock
    And no request reached a real Google host

  @needsBuyer @needsListing
  Scenario: Payload keys limited to schema
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the home page while recording tracking beacons
    And the tracking queue idle window elapses
    Then neither window.dataLayer nor any beacon body contains the buyer's email, phone or name
