## ADDED Requirements

### Requirement: AuthResult carries a refresh token

`AuthResult` in `platform/identity/v1/identity.proto` SHALL declare `string refresh_token = 4` and `google.protobuf.Timestamp refresh_expires_at = 5`, leaving fields 1-3 unchanged. `refresh_token` is opaque to clients and is empty when the server does not issue a refresh token (sessions disabled, or a grace replay). Grace semantics: replaying the immediately previous refresh token within 10 s (inclusive) of its rotation SHALL return a fresh access token for the same sid with an EMPTY `refresh_token`; after 10 s the replay is reuse, SHALL return `Unauthenticated` and SHALL revoke the session.

#### Scenario: Fields exist with the agreed numbers
- **WHEN** the compiled `AuthResult` descriptor is inspected
- **THEN** `refresh_token` is field 4 of type string and `refresh_expires_at` is field 5 of type `google.protobuf.Timestamp`, and fields 1, 2, 3 keep their names, types and numbers
- **VERIFIED BY**: `platform-core/packages/proto` descriptor check run by `make check`/`buf build` plus a generated-code field-number assertion in the generated-code task (tasks 2.1)

#### Scenario: Change is backward compatible
- **WHEN** `buf breaking --against` the committed HEAD runs on the proto
- **THEN** it exits 0 with no findings
- **VERIFIED BY**: `make -C platform-core breaking`

#### Scenario: Boundary, sessions disabled
- **WHEN** an `AuthResult` is serialised with `refresh_token` unset
- **THEN** it round-trips with an empty string and an unset `refresh_expires_at`
- **VERIFIED BY**: Go round-trip test `platform-core/packages/proto/platform/identity/v1/authresult_roundtrip_test.go` (tasks 2.1)

### Requirement: AuthService exposes RefreshSession

`AuthService` SHALL declare `rpc RefreshSession(RefreshSessionRequest) returns (RefreshSessionResponse)`. `RefreshSessionRequest` SHALL have `string refresh_token = 1`. `RefreshSessionResponse` SHALL have `AuthResult result = 1` holding a new access token and the rotated refresh token. The RPC comment SHALL state that it is public (no Principal or scope), is not retried by callers, and returns `Unauthenticated` for an unknown, expired, revoked or reused token and for an empty or malformed one. The comment SHALL also state the grace semantics above.

#### Scenario: Service and messages exist in every generated language
- **WHEN** `buf generate` runs
- **THEN** the Go, Python, JVM and TS outputs each expose `RefreshSession`, `RefreshSessionRequest.refresh_token` and `RefreshSessionResponse.result`
- **VERIFIED BY**: `make -C platform-core proto` followed by a grep of each generated tree (tasks 2.2)

#### Scenario: Lint passes
- **WHEN** `buf lint` runs on the proto
- **THEN** it exits 0 (request/response naming follows the existing `<Rpc>Request`/`<Rpc>Response` convention)
- **VERIFIED BY**: `make -C platform-core lint-proto`

#### Scenario: Error contract is documented
- **WHEN** a reader opens the `RefreshSession` RPC comment in the proto
- **THEN** it names `Unauthenticated` for unknown, expired, revoked, reused, empty and malformed tokens, states the RPC is public and not retried, says reuse revokes the session, and states the grace semantics (previous token within 10 s inclusive: fresh access token, same sid, empty `refresh_token`; after 10 s: reuse)
- **VERIFIED BY**: proto comment check in `make -C platform-core check` review gate (tasks 1.3); behaviour is verified in identity-session-refresh

#### Scenario: Existing RPCs are untouched
- **WHEN** the proto diff is reviewed
- **THEN** `buf breaking` reports no findings, and the diff adds only the listed symbols (checked in review)
- **VERIFIED BY**: `make -C platform-core breaking` plus diff review

#### Scenario: Empty or malformed token is documented as Unauthenticated
- **WHEN** a reader checks the contract for `RefreshSessionRequest.refresh_token` that is empty or malformed
- **THEN** the proto comment and this spec state the result is `Unauthenticated`, the same code as for an unknown token (no distinguishing oracle)
- **VERIFIED BY**: proto comment review (tasks 1.3); behaviour verified in identity-session-refresh

#### Scenario: Grace replay semantics are documented
- **WHEN** a reader checks the contract for a replay of the immediately previous refresh token
- **THEN** the proto comment states: at 10 s inclusive or less, fresh access token, same sid, empty `refresh_token`; after 10 s, `Unauthenticated` and session revoked
- **VERIFIED BY**: proto comment review (tasks 1.3); behaviour verified in identity-session-refresh
