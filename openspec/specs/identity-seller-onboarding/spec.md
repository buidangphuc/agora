# identity-seller-onboarding Specification

## Purpose
TBD - created by archiving change identity-seller-onboarding. Update Purpose after archive.

## Requirements

### Requirement: An ACTIVE user becomes a seller with BecomeSeller

`AuthService.BecomeSeller(terms_version)` SHALL, for an authenticated ACTIVE user principal whose `terms_version` exactly equals the current terms version (the first entry of `SELLER_TERMS_VERSIONS`), add role `seller` to that user, store `seller_terms_version` and `seller_terms_accepted_at`, and return an `AuthResult` whose access token is signed on the caller's existing `sid` with the seller scopes and whose refresh token is empty (the stored refresh token is unchanged).

#### Scenario: Seller onboarding: a buyer becomes a seller
- **WHEN** an ACTIVE buyer calls `BecomeSeller` with the current terms version
- **THEN** the response token verifies against the JWKS, carries `sid` equal to the caller's session id and scope `listing.write`, and `GetUser` for that user lists roles `buyer` and `seller`
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerGrantsRoleAndToken`

#### Scenario: Seller onboarding: acceptance is recorded
- **WHEN** an ACTIVE buyer calls `BecomeSeller` with version `V` at time `T`
- **THEN** the user row holds `seller_terms_version = V` and `seller_terms_accepted_at = T` (within 1 s)
- **VERIFIED BY**: `team-identity/internal/store` `TestGrantSellerStoresTermsAcceptance`

#### Scenario: Seller onboarding: the refresh token is unchanged
- **WHEN** an ACTIVE buyer calls `BecomeSeller` and then `RefreshSession` with the refresh token issued at login
- **THEN** the response of `BecomeSeller` has an empty refresh token and `RefreshSession` succeeds on the same `sid` with seller scopes
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerKeepsRefreshToken`

#### Scenario: Seller onboarding: an unknown terms version is refused
- **WHEN** an ACTIVE buyer calls `BecomeSeller` with an unknown or empty `terms_version`, or with an older version still listed in `SELLER_TERMS_VERSIONS`
- **THEN** the call fails `INVALID_ARGUMENT`, the user keeps roles `buyer` only and no outbox row is written
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerUnknownTermsVersion`

#### Scenario: Seller onboarding: the terms version must match exactly
- **WHEN** an ACTIVE buyer calls `BecomeSeller` with the current version altered only by surrounding whitespace or letter case (for example ` V1` or `v1` when the current version is `V1`)
- **THEN** the call fails `INVALID_ARGUMENT`, the user keeps roles `buyer` only and no outbox row is written
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerTermsVersionExactMatch`

#### Scenario: Seller onboarding: a non-ACTIVE account is refused
- **WHEN** a user whose status is RESTRICTED calls `BecomeSeller` with the current terms version
- **THEN** the call fails `PERMISSION_DENIED`, the user's roles are unchanged and no outbox row is written
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerNonActiveDenied`

#### Scenario: Seller onboarding: an existing seller who is no longer ACTIVE gets no token
- **WHEN** a user who already holds `seller` and whose status is RESTRICTED, SUSPENDED or BANNED calls `BecomeSeller` with the current terms version
- **THEN** the call fails `PERMISSION_DENIED`, the response carries no token, and no outbox row is written
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerExistingSellerNonActiveDenied`

#### Scenario: Seller onboarding: an admin restriction is not bypassed by a concurrent call
- **WHEN** `SetUserStatus(RESTRICTED)` commits before a `BecomeSeller` call of the same user reaches the role write
- **THEN** the call fails `PERMISSION_DENIED` and the user has no `seller` role
- **VERIFIED BY**: `team-identity/internal/store` `TestGrantSellerRechecksStatusUnderLock`

#### Scenario: Seller onboarding: a service or anonymous caller is refused
- **WHEN** a service principal, or a request without a token, calls `BecomeSeller`
- **THEN** the gateway answers `UNAUTHENTICATED` for anonymous and `PERMISSION_DENIED` for the service principal, and no user is changed
- **VERIFIED BY**: `team-gateway/internal/edge` `TestBecomeSellerRequiresUserPrincipal`

#### Scenario: Seller onboarding: a revoked session cannot mint a token
- **WHEN** a user whose session was revoked calls `BecomeSeller` with the old session's still-unexpired access token
- **THEN** the call fails `UNAUTHENTICATED`, no role is added and no outbox row is written
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerRevokedSession`

### Requirement: BecomeSeller publishes UserRoleGranted once

A first grant SHALL enqueue exactly one `UserRoleGranted{user_id, role="seller", granted_at}` on `identity.events` through the outbox in the same transaction as the role write. A call by a user who already holds `seller` SHALL return a fresh token, SHALL NOT change the stored terms version or acceptance time and SHALL NOT enqueue an event and SHALL NOT write an audit entry. A first grant SHALL write exactly one audit entry with action `auth.become_seller` (actor: the calling user; metadata: the accepted `terms_version` and `result=granted`; never a token, a refresh token or a session id) before the role grant commits, fail-closed like admin mutations: when the audit write fails, `BecomeSeller` SHALL answer `UNAVAILABLE` with `google.rpc.ErrorInfo{domain:"identity.platform", reason:"AUDIT_UNAVAILABLE"}` and SHALL write no role, no terms fields and no `UserRoleGranted` event.

#### Scenario: Seller onboarding: the first grant emits one event
- **WHEN** an ACTIVE buyer calls `BecomeSeller` once
- **THEN** exactly one outbox row of type `UserRoleGranted` with that user id and role `seller` exists and is delivered to `identity.events` within 10 s
- **VERIFIED BY**: `team-identity/internal/app` `TestBecomeSellerEnqueuesUserRoleGranted`

#### Scenario: Seller onboarding: a failed transaction leaves no event
- **WHEN** the role write fails inside `BecomeSeller` (injected store error)
- **THEN** the call fails with an internal error and zero outbox rows exist for the user
- **VERIFIED BY**: `team-identity/internal/store` `TestGrantSellerRollsBackOutbox`

#### Scenario: Seller onboarding: a second call is idempotent
- **WHEN** an ACTIVE user who is already a seller calls `BecomeSeller` again with the current terms version
- **THEN** the call succeeds with a new token on the same `sid`, the stored `seller_terms_version` and `seller_terms_accepted_at` equal the first call's values, and the outbox still holds exactly one `UserRoleGranted` row for the user
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerIdempotentForSeller`

#### Scenario: Seller onboarding: concurrent calls emit one event
- **WHEN** one ACTIVE buyer sends two `BecomeSeller` calls concurrently
- **THEN** both succeed and exactly one `UserRoleGranted` row exists
- **VERIFIED BY**: `team-identity/internal/store` `TestGrantSellerConcurrentOneEvent`

#### Scenario: Seller onboarding: a first grant writes one audit entry
- **WHEN** an ACTIVE buyer calls `BecomeSeller` once with the current terms version
- **THEN** team-audit holds exactly one entry with action `auth.become_seller`, actor equal to the user id, metadata containing the terms version and `result=granted` and containing no token, refresh token or session id
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerWritesAuditEntry`

#### Scenario: Seller onboarding: audit unavailable refuses the grant
- **WHEN** an ACTIVE buyer calls `BecomeSeller` while team-audit is unavailable
- **THEN** the call fails `UNAVAILABLE` with `ErrorInfo` reason `AUDIT_UNAVAILABLE`, the user keeps its roles, `seller_terms_version` and `seller_terms_accepted_at` stay empty and zero `UserRoleGranted` outbox rows exist for the user
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerAuditDownRefusesGrant`

#### Scenario: Seller onboarding: an audit failure after the role write rolls it back
- **WHEN** the audit write succeeds but the transaction commit fails, or the audit write fails after the role row update inside the transaction (injected errors)
- **THEN** the user is still a buyer with empty terms fields and no outbox row exists
- **VERIFIED BY**: `team-identity/internal/store` `TestGrantSellerAuditFailureRollsBack`

#### Scenario: Seller onboarding: a repeat call by a seller does not audit again
- **WHEN** an ACTIVE user who is already a seller calls `BecomeSeller` again, with team-audit available or unavailable
- **THEN** the call succeeds with a new token and team-audit holds still exactly one `auth.become_seller` entry for the user
- **VERIFIED BY**: `team-identity/internal/identity` `TestBecomeSellerRepeatDoesNotAudit`

#### Scenario: Seller onboarding: audit unavailable is refused through the gateway
- **WHEN** team-audit is stopped and a signed-in buyer calls `POST :8080/platform.identity.v1.AuthService/BecomeSeller` with the current terms version
- **THEN** the response is Connect `unavailable` with `ErrorInfo` reason `AUDIT_UNAVAILABLE` and a follow-up token refresh still shows no `listing.write`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/seller_onboarding.feature` `Seller onboarding: audit unavailable is refused through the gateway`

### Requirement: BecomeSeller meets its latency budget

`BecomeSeller` SHALL answer with p95 below 200 ms against the local stack.

#### Scenario: Seller onboarding: latency over 50 calls
- **WHEN** 50 distinct ACTIVE buyers each call `BecomeSeller` once through the gateway on the local stack
- **THEN** the 95th percentile of the response times is below 200 ms
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/seller_onboarding.feature` `Seller onboarding: latency over 50 calls`

#### Scenario: Seller onboarding: a rejected call is also fast
- **WHEN** 50 ACTIVE buyers each call `BecomeSeller` with an unknown terms version through the gateway
- **THEN** every call fails `INVALID_ARGUMENT` and the 95th percentile of the response times is below 200 ms
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/seller_onboarding.feature` `Seller onboarding: a rejected call is also fast`

### Requirement: The gateway exposes BecomeSeller to signed-in users

team-gateway SHALL route `platform.identity.v1.AuthService/BecomeSeller` for authenticated user principals only, forwarding the caller's session id, and SHALL NOT list it as public.

#### Scenario: Seller onboarding: a signed-in buyer becomes a seller through the gateway
- **WHEN** a registered buyer signs in on the seller app, calls `POST :8080/platform.identity.v1.AuthService/BecomeSeller` with its bearer token and the current terms version
- **THEN** the response is 200 with a token whose `scopes` contain `listing.write`, and a follow-up `CreateListing` with that token no longer fails `PERMISSION_DENIED`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/seller_onboarding.feature` `Seller onboarding: a signed-in buyer becomes a seller through the gateway`

#### Scenario: Seller onboarding: an admin restriction after onboarding still applies
- **WHEN** a buyer has become a seller and an admin then calls `SetUserStatus` RESTRICTED for that user and the user refreshes the session
- **THEN** the refreshed token has no `listing.write` and carries `status: "restricted"`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/seller_onboarding.feature` `Seller onboarding: an admin restriction after onboarding still applies`

#### Scenario: Seller onboarding: an anonymous caller is refused at the edge
- **WHEN** a request without a token calls `BecomeSeller` on the gateway
- **THEN** the response is 401
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/seller_onboarding.feature` `Seller onboarding: an anonymous caller is refused at the edge`
