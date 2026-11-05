# identity-handoff-contract Specification

## Purpose
TBD - created by archiving change p10-gaps-contract. Update Purpose after archive.

## Requirements

### Requirement: Identity hand-off and seller contract is additive

The identity proto SHALL declare enum `HandoffApp` (UNSPECIFIED=0, BUYER=1, SELLER=2), `AuthService.CreateHandoffCode`, `RedeemHandoffCode`, `PeekHandoffCode` and `BecomeSeller` with their messages as in the initiative contract, and event message `UserRoleGranted{user_id=1, role=2, granted_at=3}` for topic `identity.events`. Existing `AuthService` RPCs and messages SHALL remain unchanged. Enforcement and authz tests are owned by identity-sso-handoff and identity-seller-onboarding.

#### Scenario: Hand-off RPCs and messages exist
- **WHEN** the compiled identity descriptor is loaded
- **THEN** `AuthService` has `CreateHandoffCode(CreateHandoffCodeRequest) returns (CreateHandoffCodeResponse)` and `RedeemHandoffCode(RedeemHandoffCodeRequest) returns (RedeemHandoffCodeResponse)`, `CreateHandoffCodeRequest.target_app = 1` is `HandoffApp`, `CreateHandoffCodeResponse` has `code = 1` and `expires_at = 2`, `RedeemHandoffCodeRequest` has `code = 1` and `target_app = 2`, and `RedeemHandoffCodeResponse.result = 1` is `AuthResult`, `PeekHandoffCode(PeekHandoffCodeRequest) returns (PeekHandoffCodeResponse)` exists, `PeekHandoffCodeRequest` has `code = 1` and `target_app = 2`, and `PeekHandoffCodeResponse` has `username = 1` and `expires_at = 2` (Timestamp)
- **VERIFIED BY**: platform-core `packages/proto/platform/identity/v1` `TestGapsContract_HandoffRPCs`

#### Scenario: BecomeSeller and event exist
- **WHEN** the compiled identity descriptor is loaded
- **THEN** `AuthService.BecomeSeller(BecomeSellerRequest) returns (BecomeSellerResponse)` exists, `BecomeSellerRequest.terms_version = 1`, `BecomeSellerResponse.result = 1` is `AuthResult`, `HandoffApp` has exactly 3 values with the declared numbers, and `UserRoleGranted` has `user_id = 1`, `role = 2`, `granted_at = 3` (Timestamp)
- **VERIFIED BY**: platform-core `packages/proto/platform/identity/v1` `TestGapsContract_BecomeSellerAndEvent`

#### Scenario: Existing identity contract is untouched
- **WHEN** the identity descriptor is compared with the descriptor on main before this change
- **THEN** no existing field number, name, type or RPC signature differs, and `buf breaking --against main` with FILE rules exits 0 with no findings
- **VERIFIED BY**: platform-core `packages/proto/platform/identity/v1` `TestGapsContract_IdentityBreakingClean`

#### Scenario: Old AuthResult round trip still passes
- **WHEN** the existing `AuthResult` round trip test runs after the change
- **THEN** it passes unchanged
- **VERIFIED BY**: platform-core `packages/proto/platform/identity/v1` `TestAuthResultRoundTrip`

#### Scenario: Unspecified target is the zero value
- **WHEN** an empty `CreateHandoffCodeRequest` is serialized and unmarshalled
- **THEN** the wire size is 0 bytes and `target_app` decodes as `HANDOFF_APP_UNSPECIFIED`, so a server can reject it as INVALID_ARGUMENT
- **VERIFIED BY**: platform-core `packages/proto/platform/identity/v1` `TestGapsContract_UnspecifiedTargetIsZero`

#### Scenario: Removing an existing identity RPC is rejected
- **WHEN** a temp copy of the proto created by the test drops `AuthService.RefreshSession` and `buf breaking` runs against the original (the repo source is never edited)
- **THEN** `buf breaking` exits non-zero and names `RefreshSession`
- **VERIFIED BY**: platform-core `packages/proto/platform/identity/v1` `TestGapsContract_RemovedRPCDetected`

### Requirement: Hand-off RPCs document their principal rule and secrecy

The proto comments SHALL state: `CreateHandoffCode` and `BecomeSeller` need an authenticated user (not a service); `RedeemHandoffCode` is public like `RefreshSession`; `PeekHandoffCode` is public and read-only and does not consume the code; the code is a secret bearer credential that is never logged and only its hash is stored, single use, valid 60 s; `CreateHandoffCode` returns RESOURCE_EXHAUSTED when the user already holds 10 live codes; Redeem and Peek return one fixed UNAUTHENTICATED message for any invalid code. The audit catalogue comment SHALL list `auth.handoff_mint` and `auth.handoff_redeem`. `UserRoleGranted` SHALL be documented as published on `identity.events` keyed by user id.

#### Scenario: Comments carry the authz and secrecy rules
- **WHEN** the proto source of the four RPCs, `UserRoleGranted` and the audit catalogue are read
- **THEN** the `CreateHandoffCode` comment contains "bearer" and "never logged", the `RedeemHandoffCode` comment contains "Public", `BecomeSeller` contains "Authenticated user", the `CreateHandoffCode` comment also contains "60 s", "single use" and "RESOURCE_EXHAUSTED" with the cap 10, the `RedeemHandoffCode` and `PeekHandoffCode` comments contain "fixed UNAUTHENTICATED", the `PeekHandoffCode` comment contains "read-only" and "does not consume", `UserRoleGranted` contains "identity.events" and is keyed by user id, and the audit catalogue contains both `auth.handoff_mint` and `auth.handoff_redeem`
- **VERIFIED BY**: platform-core `packages/proto` `TestGapsContract_IdentityRPCsDocumented`

#### Scenario: Missing secrecy comment is rejected
- **WHEN** the test builds a temp copy of `identity.proto` with one of "never logged", "60 s", "single use", "RESOURCE_EXHAUSTED" or the "fixed UNAUTHENTICATED" wording removed from its RPC comment
- **THEN** `TestGapsContract_IdentityRPCsDocumented` run against the temp copy fails naming the RPC
- **VERIFIED BY**: platform-core `packages/proto` `TestGapsContract_IdentityRPCsDocumented`

### Requirement: Identity consumers are regenerated

Every repo that vendors `platform/identity/v1` (found by globbing `team-*/proto/platform/identity/v1`; 13 repos today) SHALL hold a byte-identical `proto/platform/identity/v1/identity.proto` and generated code that builds. Enforcement and authz tests are owned by identity-sso-handoff and identity-seller-onboarding.

#### Scenario: Identity copies match the source
- **WHEN** the vendored `identity.proto` of each consumer found by the glob is compared with the platform-core source
- **THEN** all 13 copies are byte-identical and each contains `CreateHandoffCode`, `PeekHandoffCode`, `BecomeSeller` and `UserRoleGranted`
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `identity-copies-match`

#### Scenario: A drifted or missing identity copy is rejected
- **WHEN** one consumer's vendored copy differs from the source or is absent
- **THEN** the check exits non-zero and prints the repo name
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `identity-copies-drift-rejected`

#### Scenario: Generated code compiles in Go and TypeScript consumers
- **WHEN** `go build ./...` runs in every Go consumer and `npm run typecheck` in every frontend consumer found by the glob, after regeneration
- **THEN** every command exits 0 and the generated `AuthService` client exposes `CreateHandoffCode`, `RedeemHandoffCode`, `PeekHandoffCode` and `BecomeSeller`
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `identity-builds`

### Requirement: Lint and compatibility gates pass

`buf lint` and `buf breaking --against main` (FILE rules) SHALL exit 0 for the whole proto module after this change, and regeneration SHALL be reproducible. Enforcement and authz tests are owned by the feature slices.

#### Scenario: Lint and breaking pass
- **WHEN** `make check` runs in platform-core
- **THEN** `buf lint` and `buf breaking` both exit 0 with 0 findings
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `buf-lint-breaking`

#### Scenario: Regeneration is reproducible
- **WHEN** `make proto` runs twice in platform-core
- **THEN** the second run leaves `git status --porcelain` empty for generated paths
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `regen-reproducible`
