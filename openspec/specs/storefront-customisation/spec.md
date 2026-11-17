# storefront-customisation Specification

## Purpose
Defines how team-domain validates the storefront fields written by `ListingService.UpsertStorefront` (banner, tagline, featured listings, theme) so only safe, owner-scoped values reach the public shop page.

## Requirements

### Requirement: Storefront text and theme are validated by team-domain

`UpsertStorefront` SHALL reject with INVALID_ARGUMENT, and store nothing, a tagline longer than 120 runes after trimming or containing a control character, and a theme that is neither empty nor one of `default`, `dark`, `festive`. A valid tagline SHALL be stored trimmed.

#### Scenario: Valid tagline and theme are saved
- **WHEN** a seller with `listing.write` upserts a storefront with tagline "  Hoa tươi mỗi ngày  " and theme `festive`
- **THEN** the response and a following `GetStorefront` return tagline "Hoa tươi mỗi ngày" and theme `festive`
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertStoresTrimmedTaglineAndTheme`

#### Scenario: Tagline of exactly 120 runes is accepted (boundary)
- **WHEN** a seller upserts a storefront with a tagline of exactly 120 runes of multi-byte text
- **THEN** the call succeeds and the stored tagline has 120 runes
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertTagline120RunesAccepted`

#### Scenario: Tagline over 120 runes is rejected
- **WHEN** a seller upserts a storefront with a tagline of 121 runes
- **THEN** the call returns INVALID_ARGUMENT and the stored storefront is unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertTagline121RunesRejected`

#### Scenario: Tagline with a control character is rejected
- **WHEN** a seller upserts a storefront whose tagline contains a newline or a NUL character
- **THEN** the call returns INVALID_ARGUMENT and the stored storefront is unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertTaglineControlCharRejected`

#### Scenario: Unknown theme is rejected
- **WHEN** a seller upserts a storefront with theme `neon`
- **THEN** the call returns INVALID_ARGUMENT and the stored storefront is unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertUnknownThemeRejected`

#### Scenario: Empty theme is accepted as default
- **WHEN** a seller upserts a storefront with an empty theme
- **THEN** the call succeeds and the stored theme is empty
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertEmptyThemeAccepted`

### Requirement: Banner must be an uploaded object URL

`UpsertStorefront` SHALL accept `banner_url` only when it is empty, equals the banner already stored for the caller, or starts with the storage public base URL followed by `/`, has at most 2048 bytes and carries no query string or fragment; any other value SHALL be rejected with INVALID_ARGUMENT and nothing stored.

#### Scenario: Banner from the upload bucket is saved
- **WHEN** a seller obtains an upload URL with `GetImageUploadUrl`, uploads the file, and upserts a storefront with the returned `public_url` as banner_url
- **THEN** the call succeeds and `GetStorefront` returns the same banner_url
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/domain/storefront_customisation.feature` `Banner from the upload bucket is saved`

#### Scenario: External banner URL is rejected
- **WHEN** a seller upserts a storefront with banner_url `https://evil.example/pixel.png`
- **THEN** the call returns INVALID_ARGUMENT and the stored banner_url is unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertExternalBannerRejected`

#### Scenario: Non-http scheme, query string or fragment in banner is rejected
- **WHEN** a seller upserts a storefront with banner_url `javascript:alert(1)`, then `<public base>/a.png?x=1`, then `<public base>/a.png#f`
- **THEN** each call returns INVALID_ARGUMENT and the stored banner_url is unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertBannerSchemeQueryFragmentRejected`

#### Scenario: Banner can be cleared
- **WHEN** a seller who has a banner upserts the storefront with an empty banner_url
- **THEN** the call succeeds and `GetStorefront` returns an empty banner_url
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertEmptyBannerClears`

#### Scenario: A legacy banner already stored does not block saving other fields
- **WHEN** a seller whose stored banner_url is off-bucket upserts the same banner_url with a new tagline
- **THEN** the call succeeds and the new tagline is stored with the banner_url unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertUnchangedLegacyBannerAccepted`

### Requirement: Featured listings are capped, unique and owned by the caller

`UpsertStorefront` SHALL accept at most 6 featured listing ids, each non-empty, unique, and the id of an existing listing whose owner is the caller, using one batched lookup; it SHALL keep the given order. A foreign listing id and an unknown id SHALL produce the same INVALID_ARGUMENT response, and nothing SHALL be stored.

#### Scenario: Six owned listings are saved in the given order
- **WHEN** a seller owning listings A..F upserts featured_listing_ids [C, A, F, B, E, D]
- **THEN** the call succeeds and `GetStorefront` returns the ids in order [C, A, F, B, E, D]
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/domain/storefront_customisation.feature` `Six owned listings are saved in the given order`

#### Scenario: A seventh featured listing is rejected
- **WHEN** a seller upserts 7 featured_listing_ids, all owned
- **THEN** the call returns INVALID_ARGUMENT and the stored storefront is unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertSevenFeaturedRejected`

#### Scenario: A listing owned by another seller is rejected
- **WHEN** seller S1 upserts featured_listing_ids containing a listing owned by seller S2
- **THEN** the call returns INVALID_ARGUMENT, the message does not name the owner, and S1's stored featured ids are unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/domain/storefront_customisation.feature` `A listing owned by another seller is rejected`

#### Scenario: Unknown and foreign listing ids give the same answer
- **WHEN** a seller upserts featured_listing_ids with an id that does not exist, then with an id owned by another seller
- **THEN** both calls return INVALID_ARGUMENT with identical code and message
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertFeaturedForeignAndUnknownSameError`

#### Scenario: Duplicate or empty featured ids are rejected
- **WHEN** a seller upserts featured_listing_ids [A, A], then [A, ""]
- **THEN** each call returns INVALID_ARGUMENT and the stored storefront is unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertFeaturedDuplicateOrEmptyRejected`

#### Scenario: Featured lookup uses one batched read
- **WHEN** a seller upserts 6 featured listing ids
- **THEN** the repository records exactly 1 listing read call for the whole request
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertFeaturedSingleBatchedLookup`

#### Scenario: Lookup failure stores nothing
- **WHEN** the listing read fails with a database error during an upsert with featured ids
- **THEN** the call returns a non-OK status other than INVALID_ARGUMENT and the stored storefront is unchanged
- **VERIFIED BY**: `team-domain/internal/listing/storefront_test.go` `TestUpsertFeaturedLookupErrorStoresNothing`

#### Scenario: A featured listing unpublished later is not re-validated on read
- **WHEN** a featured listing is moved to draft after the storefront was saved and a buyer calls `GetStorefront`
- **THEN** the response still contains the stored featured ids and the call succeeds
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/domain/storefront_customisation.feature` `A featured listing unpublished later is not re-validated on read`

### Requirement: Storefront writes stay owner-scoped and scope-gated

`UpsertStorefront` SHALL require `listing.write`, SHALL key the row by the authenticated principal regardless of the request `seller_id`, and SHALL keep every field the request does not change from being cleared by a full-storefront request that repeats current values.

#### Scenario: Principal without listing.write is denied
- **WHEN** a buyer token without `listing.write` calls `UpsertStorefront` with a valid storefront
- **THEN** the call returns PERMISSION_DENIED and no storefront row is written
- **VERIFIED BY**: `team-domain/internal/rpc/storefront_test.go` `TestUpsertStorefrontRequiresWriteScope`

#### Scenario: Body seller_id of another seller is ignored
- **WHEN** seller S1 upserts a storefront whose body `seller_id` is S2's id
- **THEN** the row stored is S1's and S2's storefront is unchanged
- **VERIFIED BY**: `team-domain/internal/rpc/storefront_test.go` `TestUpsertStorefrontIgnoresBodySellerID`

#### Scenario: Resending current values keeps all fields
- **WHEN** a seller with a stored banner, tagline, featured ids and theme upserts the same storefront with only the tagline changed
- **THEN** `GetStorefront` returns the new tagline and the unchanged banner_url, featured ids and theme
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/domain/storefront_customisation.feature` `Resending current values keeps all fields`

#### Scenario: Existing display name and slug rules still apply
- **WHEN** a seller upserts a storefront with an empty slug, then with an 81-rune display_name
- **THEN** each call returns INVALID_ARGUMENT and the stored storefront is unchanged
- **VERIFIED BY**: `team-domain/internal/rpc/storefront_test.go` `TestUpsertStorefrontRulesUnchangedOnSharedHandler`
