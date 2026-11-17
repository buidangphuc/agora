# identity-consent Specification

## Purpose
TBD - created by archiving change identity-legal-consent. Update Purpose after archive.

## Requirements

### Requirement: Registration requires the current legal versions

`AuthService.Register` SHALL require `terms_version` and `privacy_version`, each equal to the first entry of
`LEGAL_TERMS_VERSIONS` and `PRIVACY_POLICY_VERSIONS` respectively, and SHALL create the account and two consent rows
(purposes TERMS and PRIVACY_POLICY, `granted=true`, source `register`, server time) in one transaction. A missing,
empty or non-current version SHALL fail `INVALID_ARGUMENT` and create no user, session or consent row.

#### Scenario: Register with the current versions stores two consent records
- **WHEN** a visitor calls `Register` with `terms_version` and `privacy_version` equal to the current versions
- **THEN** the call succeeds and `ListMyConsents` for the new user returns exactly 2 records (TERMS, PRIVACY_POLICY) with those versions, `granted=true`, source `register` and `recorded_at` within 5 seconds of the call
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Register with the current versions stores two consent records

#### Scenario: Register without versions is rejected
- **WHEN** a visitor calls `Register` with an empty `terms_version` or an empty `privacy_version`
- **THEN** the call fails `INVALID_ARGUMENT` and logging in with that username fails as for an unknown user
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Register without versions is rejected

#### Scenario: Register with a stale version is rejected
- **WHEN** a visitor calls `Register` with a `terms_version` that is not the current one
- **THEN** the call fails `INVALID_ARGUMENT`, the error names the field `terms_version`, and 0 users, sessions and consent rows exist for that username
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Register with a stale version is rejected

#### Scenario: Consent insert failure rolls back the account
- **WHEN** the consent rows cannot be written during `Register`
- **THEN** the call fails and the username can be registered again afterwards because no user row was left behind
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Consent insert failure rolls back the account

#### Scenario: Registration needs no prior authentication
- **WHEN** an anonymous caller without a bearer token calls `Register` with the current versions
- **THEN** the call succeeds and the consent rows belong to the newly created user id, never to a caller-supplied id
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Registration needs no prior authentication

### Requirement: Legal versions are configured and validated at boot

team-identity SHALL read `LEGAL_TERMS_VERSIONS` and `PRIVACY_POLICY_VERSIONS` as comma lists (first = current), and
SHALL refuse to boot when either has no non-empty entry.

#### Scenario: Boot fails without a configured version
- **WHEN** the service starts with `PRIVACY_POLICY_VERSIONS` empty or unset
- **THEN** the service exits non-zero at start-up with an error naming `PRIVACY_POLICY_VERSIONS` and serves no request
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Boot fails without a configured version

#### Scenario: First entry is the current version
- **WHEN** `LEGAL_TERMS_VERSIONS=2026-11,2026-05` and a visitor registers with `terms_version=2026-05`
- **THEN** the call fails `INVALID_ARGUMENT` and registering with `2026-11` succeeds
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::First entry is the current version

### Requirement: Users record consent decisions

`PrivacyService.RecordConsent` SHALL append one ledger row per supplied record for the calling USER principal. It
SHALL accept 1 to 6 records with distinct non-UNSPECIFIED purposes, a non-empty `version` of at most 32 characters, and
a `source` in {`register`, `banner`, `account`, `seller_onboarding`}. It SHALL ignore client-supplied `recorded_at`,
SHALL take the user id only from the principal, and SHALL write all rows of one call atomically. Records with purpose
TERMS or PRIVACY_POLICY SHALL have `granted=true` and a version equal to the current configured version; records with
purpose SELLER_TERMS SHALL have `granted=true` and a version listed in `SELLER_TERMS_VERSIONS`.

#### Scenario: Banner choice stores one row per changed purpose
- **WHEN** a signed-in user calls `RecordConsent` with ANALYTICS granted and MARKETING denied, source `banner`
- **THEN** the call succeeds and `ListMyConsents` shows 2 new rows with those `granted` values, source `banner`, and no row for PERSONALIZATION
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Banner choice stores one row per changed purpose

#### Scenario: Re-accepting a new Terms version appends a row
- **WHEN** the current Terms version changes from T1 to T2 and a signed-in user calls `RecordConsent` with TERMS version T2, source `account`
- **THEN** the call succeeds, the newest TERMS row has version T2, and the earlier TERMS row with T1 is still returned
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Re-accepting a new Terms version appends a row

#### Scenario: Invalid records are rejected without partial writes
- **WHEN** a signed-in user calls `RecordConsent` with 7 records, or two records of the same purpose, or purpose UNSPECIFIED, or a 33-character version, or an unknown source, or an empty list
- **THEN** each call fails `INVALID_ARGUMENT` and the user's record count is unchanged
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Invalid records are rejected without partial writes

#### Scenario: Stale or withdrawn legal acceptance is rejected
- **WHEN** a signed-in user calls `RecordConsent` with TERMS at a non-current version, or PRIVACY_POLICY with `granted=false`
- **THEN** the call fails `INVALID_ARGUMENT` and no row is written, including for other records in the same call
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Stale or withdrawn legal acceptance is rejected

#### Scenario: Client time and user id are ignored
- **WHEN** a signed-in user calls `RecordConsent` with `recorded_at` set to year 2000
- **THEN** the stored `recorded_at` is within 5 seconds of the server time and the row belongs to the principal's user id
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Client time and user id are ignored

#### Scenario: Anonymous and service principals cannot record consent
- **WHEN** an anonymous caller, or a service principal, calls `RecordConsent`
- **THEN** the anonymous call fails `UNAUTHENTICATED`, the service call fails `PERMISSION_DENIED`, and no row is written
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Anonymous and service principals cannot record consent

### Requirement: Users list their own consent history

`PrivacyService.ListMyConsents` SHALL return only the calling user's records, newest `recorded_at` first (ties broken by
insertion order, newest first), paged by `PageRequest` with default size 20 and maximum 100, with an opaque cursor.

#### Scenario: History is newest first and paged
- **WHEN** a user with 25 records calls `ListMyConsents` with `page_size=10` and follows `next_cursor` until it is empty
- **THEN** the calls return 10, 10 and 5 records, in non-increasing `recorded_at` order, with no record repeated or missing
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::History is newest first and paged

#### Scenario: A user never sees another user's records
- **WHEN** user B calls `ListMyConsents` after user A recorded consent
- **THEN** the response contains none of A's records and the call fails nothing; B's own count is unchanged
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::A user never sees another user's records

#### Scenario: Page size is clamped and bad cursors are rejected
- **WHEN** a user calls `ListMyConsents` with `page_size=500`, then with a malformed cursor
- **THEN** the first call returns at most 100 records and the second fails `INVALID_ARGUMENT`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Page size is clamped and bad cursors are rejected

#### Scenario: Anonymous caller cannot list
- **WHEN** an anonymous caller calls `ListMyConsents`
- **THEN** the call fails `UNAUTHENTICATED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Anonymous caller cannot list

#### Scenario: A user with no records gets an empty list
- **WHEN** a legacy user with no consent rows calls `ListMyConsents`
- **THEN** the response has 0 records and an empty `next_cursor`, which the frontend reads as "re-acceptance required"
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::A user with no records gets an empty list

### Requirement: The consent ledger is append-only and reachable at the edge

No RPC SHALL update or delete a consent row. team-gateway SHALL forward `PrivacyService.RecordConsent` and
`ListMyConsents` to team-identity for signed-in users and SHALL reject anonymous calls to either RPC with `UNAUTHENTICATED`.

#### Scenario: Consent is reachable through the gateway
- **WHEN** a signed-in user calls `PrivacyService/RecordConsent` then `ListMyConsents` through the gateway
- **THEN** both succeed and the list contains the recorded row
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Consent is reachable through the gateway

#### Scenario: Anonymous edge call is refused
- **WHEN** a caller without a bearer token calls `PrivacyService/RecordConsent` through the gateway
- **THEN** the response is `UNAUTHENTICATED` and team-identity receives 0 calls
- **VERIFIED BY**: platform-e2e/tests/e2e/features/identity/legal_consent.feature::Anonymous edge call is refused
