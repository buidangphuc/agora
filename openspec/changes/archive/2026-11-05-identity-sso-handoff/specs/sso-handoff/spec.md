# sso-handoff Specification

## ADDED Requirements

### Requirement: Minting a hand-off code

team-identity SHALL, on `AuthService.CreateHandoffCode`, return a 32-byte random code (base64url, 43 chars) and
`expires_at = now + 60 s`, store only the SHA-256 of the code with `target_app`, the caller's user id and source `sid`,
and write the audit entry `auth.handoff_mint` inside the same transaction as the atomic cap check and insert, committing only after the audit write succeeds. A mint refused by the cap SHALL write no audit entry.

#### Scenario: Authenticated user mints a code for the seller app
- **WHEN** a buyer with a live session calls `CreateHandoffCode` with `target_app=HANDOFF_APP_SELLER`
- **THEN** the response carries a 43-character code and `expires_at` exactly 60 s after the server clock, and the table holds one row whose key is the SHA-256 of that code and no row contains the plaintext

#### Scenario: Mint writes its audit entry and the row commits together
- **WHEN** `CreateHandoffCode` succeeds
- **THEN** team-audit holds one `auth.handoff_mint` entry whose metadata has the target app and no code, no hash and no sid

#### Scenario: Unspecified target is refused
- **WHEN** `CreateHandoffCode` is called with `HANDOFF_APP_UNSPECIFIED`
- **THEN** the call fails `INVALID_ARGUMENT` and no row and no audit entry is written

#### Scenario: Service principal cannot mint
- **WHEN** a service principal calls `CreateHandoffCode`
- **THEN** the call fails `PERMISSION_DENIED` and no row is written
- **VERIFIED BY**: team-identity `internal/identity` `TestMintServicePrincipalDenied`

#### Scenario: Anonymous caller cannot mint
- **WHEN** an anonymous caller calls `CreateHandoffCode`
- **THEN** the call fails `UNAUTHENTICATED` and no row is written

#### Scenario: Non-ACTIVE account cannot mint
- **WHEN** a user whose account is suspended calls `CreateHandoffCode` with a still-valid access token
- **THEN** the call fails `PERMISSION_DENIED` and no row is written

#### Scenario: Audit down blocks the mint
- **IF** team-audit is unreachable
- **WHEN** `CreateHandoffCode` is called
- **THEN** the call fails `UNAVAILABLE` and no row exists afterwards

#### Scenario: Eleventh live code is refused
- **WHEN** a user who already holds 10 unexpired unredeemed codes calls `CreateHandoffCode`
- **THEN** the call fails `RESOURCE_EXHAUSTED`, the row count stays 10 and no `auth.handoff_mint` entry is written

#### Scenario: Expired and redeemed codes do not count toward the cap
- **WHEN** a user holds 9 live codes plus 5 expired and 5 redeemed codes and calls `CreateHandoffCode`
- **THEN** the call succeeds and the user has 10 live codes

#### Scenario: Concurrent mints cannot exceed the cap
- **WHEN** 20 concurrent `CreateHandoffCode` calls run for a user with 0 live codes
- **THEN** exactly 10 succeed, 10 fail `RESOURCE_EXHAUSTED`, the user has 10 live rows, and team-audit holds exactly 10 `auth.handoff_mint` entries

### Requirement: Peeking at a hand-off code

team-identity SHALL, on public `AuthService.PeekHandoffCode`, return the username of the code's user and `expires_at`
for a valid code of the given `target_app` whose user is ACTIVE, WITHOUT consuming it, writing audit entries or changing any row. The length check SHALL run before any lookup. Every
invalid code (including a non-ACTIVE account) SHALL return the same fixed `UNAUTHENTICATED` message as redeem.

#### Scenario: Peek returns the username and does not consume
- **WHEN** a fresh seller-target code is peeked 3 times and then redeemed
- **THEN** each peek returns the minting user's username and `expires_at`, and the redeem still succeeds

#### Scenario: Peek writes nothing
- **WHEN** a valid code is peeked
- **THEN** `redeemed_at` stays null and team-audit receives no entry

#### Scenario: Peek of an invalid code is indistinguishable
- **WHEN** an unknown, used, expired, wrong-target, malformed, revoked-source and non-ACTIVE-account code are each peeked
- **THEN** all responses have the same gRPC code `UNAUTHENTICATED` and the byte-identical message used by redeem

#### Scenario: Peek of a malformed code does no lookup
- **WHEN** a code of the wrong length is peeked
- **THEN** the fixed `UNAUTHENTICATED` is returned and the store receives no query
- **VERIFIED BY**: team-identity `internal/identity` `TestPeekMalformedNoLookup`

#### Scenario: Peek needs no token
- **WHEN** an anonymous caller peeks a valid code
- **THEN** the call succeeds

### Requirement: Redeeming a hand-off code

team-identity SHALL, on `AuthService.RedeemHandoffCode`, accept the code and `target_app` from an unauthenticated
caller, atomically mark the code redeemed (single use), verify the source session is live and the account ACTIVE,
write audit entry `auth.handoff_redeem`, and return an `AuthResult` for a NEW session (own sid, own refresh token)
for `target_app` whose roles are those of the user at redeem time. The new session SHALL be independent of the source
session after redeem. Every failure SHALL return `UNAUTHENTICATED` with one fixed message.

#### Scenario: Valid code opens a new session for the target app
- **WHEN** a seller-target code minted 10 s ago is redeemed with `target_app=HANDOFF_APP_SELLER`
- **THEN** the response has an access token and refresh token, the new session's sid differs from the source sid, and the source session still refreshes normally

#### Scenario: Redeem at the TTL boundary
- **WHEN** a code is redeemed exactly at `expires_at`
- **THEN** the call succeeds, and a redeem at `expires_at + 1 ms` fails `UNAUTHENTICATED`
- **VERIFIED BY**: team-identity `internal/identity` `TestRedeemAtExpiryInstant`

#### Scenario: Derived session carries the roles at redeem time
- **WHEN** a user is granted role seller after minting a seller-target code and the code is redeemed
- **THEN** the new access token's scopes include seller
- **VERIFIED BY**: team-identity `internal/identity` `TestRedeemRolesAtRedeemTime`

#### Scenario: Derived session survives source logout
- **WHEN** a code is redeemed successfully and the source session is then logged out
- **THEN** the new session still refreshes successfully

#### Scenario: Reused code is refused
- **WHEN** a code that was already redeemed is redeemed again
- **THEN** the call fails `UNAUTHENTICATED` and no second session exists

#### Scenario: Concurrent redeems create one session
- **WHEN** 20 requests redeem the same fresh code at once
- **THEN** exactly one succeeds, 19 fail `UNAUTHENTICATED`, one new session exists, team-audit holds exactly one `auth.handoff_redeem` entry, and the 19 losers each return within the redeem p95 of 150 ms and write no audit entry

#### Scenario: Expired code is refused
- **WHEN** a code 61 s old is redeemed
- **THEN** the call fails `UNAUTHENTICATED`

#### Scenario: Wrong target is refused
- **WHEN** a buyer-target code is redeemed with `target_app=HANDOFF_APP_SELLER`
- **THEN** the call fails `UNAUTHENTICATED` and the code stays unredeemed for its true target

#### Scenario: Revoked source session is refused
- **WHEN** the source session was logged out or revoked after minting and the code is redeemed
- **THEN** the call fails `UNAUTHENTICATED` and no session is opened

#### Scenario: Account suspended after minting
- **WHEN** the account is set to a non-ACTIVE status after minting and the code is redeemed
- **THEN** the call fails `UNAUTHENTICATED` and no session is opened

#### Scenario: Every failure is indistinguishable
- **WHEN** an unknown code, used code, expired code, wrong-target code, malformed (wrong length) code and a revoked-source code are each redeemed
- **THEN** all six responses have the same gRPC code and byte-identical message

#### Scenario: Audit down blocks the redeem and keeps the code usable (accepted oracle)
- **IF** team-audit is unreachable
- **WHEN** a valid code is redeemed
- **THEN** the call fails `UNAVAILABLE`, no session is opened, and the code redeems successfully once audit is back within its TTL

#### Scenario: Redeem audit entry
- **WHEN** a redeem succeeds
- **THEN** team-audit holds one `auth.handoff_redeem` entry whose actor is the user id stored with the code, with the target app and result `ok`, and no code, hash or sid

### Requirement: Hand-off code secrecy

team-identity and team-gateway SHALL NOT log, trace, audit or error-message the plaintext code or its hash.

#### Scenario: Code absent from logs even on failure
- **WHEN** a code is minted, redeemed successfully, and redeemed again unsuccessfully with log capture on for identity and gateway
- **THEN** the failed redeem is rejected and the captured output contains neither the plaintext code, nor its SHA-256 hex or base64, nor the user id

### Requirement: TTL override only in the test environment

team-identity SHALL honour a hand-off TTL override setting `HANDOFF_TTL_OVERRIDE` only when `ENV=test` and SHALL fail startup when it is set
under any other `ENV`. When the override is set (non-zero, non-negative, `ENV=test`), the service built in `internal/app/app.go` SHALL use it as the TTL of every minted code (wired through `WithHandoffTTL`); when it is unset the TTL SHALL stay the 60 s default. A negative override SHALL still fail startup.

#### Scenario: Override accepted in test
- **WHEN** identity starts with `ENV=test` and a 5 s override
- **THEN** a minted code has `expires_at` 5 s after the server clock
- **VERIFIED BY**: team-identity `internal/config` `TestTTLOverrideOnlyInTestAccepted`

#### Scenario: Override is the TTL of minted codes in the running app
- **WHEN** the app is built with `ENV=test` and `HANDOFF_TTL_OVERRIDE=5s` and a user calls `CreateHandoffCode`
- **THEN** the response `expires_at` is 5 s after the server clock and the code is refused once 5 s have passed
- **VERIFIED BY**: team-identity `internal/app` `TestAppWiresHandoffTTLOverride`

#### Scenario: Unset override keeps 60 s in the running app
- **WHEN** the app is built with `ENV=test` and no `HANDOFF_TTL_OVERRIDE`
- **THEN** the response `expires_at` of `CreateHandoffCode` is exactly 60 s after the server clock
- **VERIFIED BY**: team-identity `internal/app` `TestAppDefaultHandoffTTLWithoutOverride`

#### Scenario: Negative override fails startup
- **WHEN** identity starts with `ENV=test` and `HANDOFF_TTL_OVERRIDE=-1s`
- **THEN** startup fails with a config error
- **VERIFIED BY**: team-identity `internal/config` `TestTTLOverrideNegativeRefused`

#### Scenario: Override refused outside test
- **WHEN** identity starts with `ENV=staging` (or local, prod) and the override set
- **THEN** startup fails with a config error and no listener opens
- **VERIFIED BY**: team-identity `internal/config` `TestTTLOverrideOnlyInTestRefusedElsewhere`

#### Scenario: Default TTL without override
- **WHEN** identity starts with `ENV=prod` and no override
- **THEN** a minted code has `expires_at` exactly 60 s after the server clock
- **VERIFIED BY**: team-identity `internal/identity` `TestMintReturnsCodeAndTTL`

### Requirement: Hand-off code purge

team-identity SHALL delete `handoff_codes` rows whose `expires_at` is more than 24 h in the past, redeemed or not.

#### Scenario: Old rows are purged, recent ones kept
- **WHEN** the sweeper runs with rows expired 25 h ago and 23 h ago
- **THEN** only the 25 h row is deleted
- **VERIFIED BY**: team-identity `internal/store` `TestHandoffPurge`

### Requirement: Gateway exposure

team-gateway SHALL expose `CreateHandoffCode` to authenticated user callers and `PeekHandoffCode` and
`RedeemHandoffCode` as public RPCs the same way as `RefreshSession`, and SHALL NOT retry `RedeemHandoffCode` upstream.

#### Scenario: Redeem needs no token
- **WHEN** a request without an Authorization header calls `RedeemHandoffCode` through the gateway with a valid code
- **THEN** the gateway forwards it and returns the new session result

#### Scenario: Peek needs no token at the gateway
- **WHEN** a request without an Authorization header calls `PeekHandoffCode` through the gateway with a valid code
- **THEN** the gateway forwards it and returns the username

#### Scenario: Mint without a token is refused
- **WHEN** a request without an Authorization header calls `CreateHandoffCode` through the gateway
- **THEN** the gateway returns `UNAUTHENTICATED` and identity receives no call

#### Scenario: Redeem is never retried
- **WHEN** identity answers `UNAVAILABLE` to a redeem forwarded by the gateway
- **THEN** identity received exactly one `RedeemHandoffCode` call and the caller gets `UNAVAILABLE`
- **VERIFIED BY**: team-gateway `internal/edge` `TestAuthForwarderRedeemNotRetriedHandoff`

### Requirement: The gateway forwards the session id only where it is needed

team-gateway SHALL set the `x-principal-sid` metadata on an upstream call only for the `platform.identity.v1.AuthService` procedures `BecomeSeller`, `CreateHandoffCode`, `PeekHandoffCode` and `RedeemHandoffCode`, and only when the verified token carries a `sid`. It SHALL NOT forward `x-principal-sid` on any other procedure or upstream service (`Register`, `Login`, `RefreshSession` and every non-identity service included).

#### Scenario: Session id is forwarded to BecomeSeller and CreateHandoffCode
- **WHEN** a user principal whose token has `sid=S` calls `BecomeSeller` and `CreateHandoffCode` through the edge
- **THEN** the outgoing metadata of each call contains `x-principal-sid=S`
- **VERIFIED BY**: team-gateway `internal/edge` `TestOutgoingForwardsSidOnlyToAllowlist`

#### Scenario: Session id is never forwarded elsewhere
- **WHEN** the same principal calls `Login`, `RefreshSession`, `PeekHandoffCode` anonymously, or any procedure of another service (for example listing or order)
- **THEN** the outgoing metadata of each call has no `x-principal-sid` key and `x-principal-id`, `x-principal-type` and scopes are unchanged
- **VERIFIED BY**: team-gateway `internal/edge` `TestOutgoingOmitsSidOutsideAllowlist`

#### Scenario: A client-supplied sid header is dropped
- **WHEN** an anonymous caller sends its own `x-principal-sid` header on `RedeemHandoffCode` or on a non-allow-listed procedure
- **THEN** the upstream receives no `x-principal-sid`, because the value comes only from the verified token
- **VERIFIED BY**: team-gateway `internal/edge` `TestOutgoingOmitsSidOutsideAllowlist`
