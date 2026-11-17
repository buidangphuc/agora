# listing-bundle-integrity Specification

## Purpose
Defines the rules `ListingService.CreateBundle` enforces so a stored bundle only holds the caller's own eligible listings and its price is a real discount, before the bundle can be bought.

## Requirements

### Requirement: A bundle contains only the caller's own listings

`ListingService.CreateBundle` SHALL store a bundle only when every listed id is an existing listing owned by the
authenticated principal. A listing owned by another seller and a listing id that does not exist SHALL produce the same
`PERMISSION_DENIED` answer and nothing SHALL be stored.

#### Scenario: B1 A seller bundles their own listings

- **WHEN** a seller calls `CreateBundle` with two of their own published listings and a valid bundle price
- **THEN** the response carries a bundle with `seller_id` equal to the caller and `listing_ids` in the request order, and `GetBundle` returns it
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A seller bundles their own listings`

#### Scenario: B2 Another seller's listing is refused

- **WHEN** a seller calls `CreateBundle` with one of their own listings and one listing owned by another seller
- **THEN** the call fails with `PERMISSION_DENIED` and `ListBundlesBySeller` for the caller returns no new bundle
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `Another seller's listing is refused`

#### Scenario: B3 An unknown listing id gives the same answer as a foreign one

- **WHEN** a seller calls `CreateBundle` with one of their own listings and a listing id that does not exist
- **THEN** the call fails with `PERMISSION_DENIED` carrying the same message as in B2
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `An unknown listing id gives the same answer as a foreign one`

#### Scenario: B4 A caller without the listing write scope is refused

- **WHEN** a caller whose token lacks the `listing.write` scope calls `CreateBundle`
- **THEN** the call fails with `PERMISSION_DENIED` and no bundle is stored
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A caller without the listing write scope is refused`

### Requirement: A bundle holds two to ten distinct listings

`CreateBundle` SHALL refuse a bundle with fewer than 2 or more than 10 listing ids, or with a repeated listing id, with
`INVALID_ARGUMENT`.

#### Scenario: B5 A single-listing bundle is rejected

- **WHEN** a seller calls `CreateBundle` with exactly one listing id
- **THEN** the call fails with `INVALID_ARGUMENT` and no bundle is stored
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A single-listing bundle is rejected`

#### Scenario: B6 A bundle of exactly ten listings is accepted and eleven are rejected

- **WHEN** a seller calls `CreateBundle` with ten of their own listings and a valid price, then again with eleven
- **THEN** the first call returns a bundle with 10 listing ids and the second fails with `INVALID_ARGUMENT`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A bundle of exactly ten listings is accepted and eleven are rejected`

#### Scenario: B7 A repeated listing id is rejected

- **WHEN** a seller calls `CreateBundle` with the same own listing id twice
- **THEN** the call fails with `INVALID_ARGUMENT` and no bundle is stored
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A repeated listing id is rejected`

### Requirement: Variant listings cannot be bundled

`CreateBundle` SHALL refuse, with `INVALID_ARGUMENT`, any bundle that contains a listing that has one or more variants.

#### Scenario: B8 A bundle with a variant listing is rejected

- **WHEN** a seller calls `CreateBundle` with one plain listing and one of their own listings that has variants
- **THEN** the call fails with `INVALID_ARGUMENT` and no bundle is stored
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A bundle with a variant listing is rejected`

### Requirement: The bundle price is a real discount

`CreateBundle` SHALL require `bundle_price` to be greater than 0 and strictly less than the sum of the current prices
of the listed listings, and SHALL refuse any other value with `INVALID_ARGUMENT`.

#### Scenario: B9 A price equal to the sum of the parts is rejected

- **WHEN** a seller calls `CreateBundle` with two own listings priced 100000 and 50000 and `bundle_price` 150000
- **THEN** the call fails with `INVALID_ARGUMENT` and no bundle is stored
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A price equal to the sum of the parts is rejected`

#### Scenario: B10 One minor unit below the sum is accepted

- **WHEN** a seller calls `CreateBundle` with two own listings priced 100000 and 50000 and `bundle_price` 149999
- **THEN** the response carries a bundle whose `bundle_price` is 149999
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `One minor unit below the sum is accepted`

#### Scenario: B11 A zero or negative price is rejected

- **WHEN** a seller calls `CreateBundle` with valid listings and `bundle_price` 0, then again with -1
- **THEN** both calls fail with `INVALID_ARGUMENT` and no bundle is stored
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A zero or negative price is rejected`

### Requirement: Existing bundles are readable and never deleted by this change

`GetBundle` and `ListBundlesBySeller` SHALL keep returning every stored bundle, including one created before these
rules that breaks them, and no migration or write SHALL remove or alter such a bundle.

#### Scenario: B12 A bundle that predates the rules is still returned

- **WHEN** a bundle stored before the rules (price at or above the sum of its parts) is requested through `GetBundle` and `ListBundlesBySeller`
- **THEN** both calls return it unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `A bundle that predates the rules is still returned`

#### Scenario: B13 An unknown bundle id is not found

- **WHEN** a caller requests `GetBundle` with an id that does not exist
- **THEN** the call fails with `NOT_FOUND`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` `An unknown bundle id is not found`
