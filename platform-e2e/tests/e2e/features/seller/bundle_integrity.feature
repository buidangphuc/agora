@seller
Feature: A bundle contains only valid listings of the caller
  ListingService.CreateBundle stores a bundle only for two to ten distinct, existing, variant-free
  listings owned by the caller, priced strictly between 0 and the sum of the parts. Foreign and
  unknown listing ids get one identical PERMISSION_DENIED answer. Bundles stored before these rules
  stay readable. (domain-bundle-integrity)

  Scenario: A seller bundles their own listings
    Given a seller owns published listings priced 100000 and 50000
    When the seller creates a bundle of "own1, own2" priced 120000
    Then the bundle call succeeds
    And the bundle belongs to the seller and lists "own1, own2" in that order
    And GetBundle returns the created bundle

  Scenario: Another seller's listing is refused
    Given a seller owns published listings priced 100000 and 50000
    And another seller owns a published listing priced 70000
    When the seller creates a bundle of "own1, foreign" priced 120000
    Then the bundle call fails with PERMISSION_DENIED
    And the seller has no stored bundle

  Scenario: An unknown listing id gives the same answer as a foreign one
    Given a seller owns published listings priced 100000 and 50000
    And another seller owns a published listing priced 70000
    When the seller creates a bundle of "own1, foreign" priced 120000
    And the seller creates a bundle of "own1, unknown" priced 120000
    Then the bundle call fails with PERMISSION_DENIED
    And both bundle calls carry the same message
    And the seller has no stored bundle

  Scenario: A caller without the listing write scope is refused
    Given a seller owns published listings priced 100000 and 50000
    And a buyer is signed in
    When the buyer creates a bundle of "own1, own2" priced 120000
    Then the bundle call fails with PERMISSION_DENIED
    And the seller has no stored bundle

  Scenario: A single-listing bundle is rejected
    Given a seller owns published listings priced 100000 and 50000
    When the seller creates a bundle of "own1" priced 50000
    Then the bundle call fails with INVALID_ARGUMENT
    And the seller has no stored bundle

  Scenario: A bundle of exactly ten listings is accepted and eleven are rejected
    Given a seller owns 11 published listings priced 100000 each
    When the seller creates a bundle of the first 10 listings priced 900000
    Then the bundle call succeeds
    And the created bundle has 10 listing ids
    When the seller creates a bundle of the first 11 listings priced 900000
    Then the bundle call fails with INVALID_ARGUMENT
    And the seller has exactly 1 stored bundle

  Scenario: A repeated listing id is rejected
    Given a seller owns published listings priced 100000 and 50000
    When the seller creates a bundle of "own1, own1" priced 120000
    Then the bundle call fails with INVALID_ARGUMENT
    And the seller has no stored bundle

  Scenario: A bundle with a variant listing is rejected
    Given a seller owns published listings priced 100000 and 50000
    And the seller owns a published listing with variants priced 80000
    When the seller creates a bundle of "own1, variant" priced 120000
    Then the bundle call fails with INVALID_ARGUMENT
    And the seller has no stored bundle

  Scenario: A price equal to the sum of the parts is rejected
    Given a seller owns published listings priced 100000 and 50000
    When the seller creates a bundle of "own1, own2" priced 150000
    Then the bundle call fails with INVALID_ARGUMENT
    And the seller has no stored bundle

  Scenario: One minor unit below the sum is accepted
    Given a seller owns published listings priced 100000 and 50000
    When the seller creates a bundle of "own1, own2" priced 149999
    Then the bundle call succeeds
    And the created bundle has price 149999

  Scenario: A zero or negative price is rejected
    Given a seller owns published listings priced 100000 and 50000
    When the seller creates a bundle of "own1, own2" priced 0
    Then the bundle call fails with INVALID_ARGUMENT
    When the seller creates a bundle of "own1, own2" priced -1
    Then the bundle call fails with INVALID_ARGUMENT
    And the seller has no stored bundle

  Scenario: A bundle that predates the rules is still returned
    Given a seller owns published listings priced 100000 and 50000
    And a bundle of "own1, own2" priced 200000 was stored before the rules
    When the legacy bundle is requested through GetBundle
    And the seller's bundles are listed
    Then GetBundle returns the legacy bundle unchanged
    And the seller's bundle list contains the legacy bundle unchanged

  Scenario: An unknown bundle id is not found
    Given a seller owns published listings priced 100000 and 50000
    When the seller requests GetBundle for an unknown bundle id
    Then the bundle call fails with NOT_FOUND
