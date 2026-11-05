## REMOVED Requirements

### Requirement: team-identity builds on the tagged go-sdk with the ADR-0016 layout

**Reason**: The requirement pinned go-sdk `v1.0.2` and its scenario title named that version; a MODIFIED block cannot rename a scenario.

**Migration**: Replaced by the ADDED requirement "team-identity builds on a tagged go-sdk release with the ADR-0016 layout", which keeps every clause and scenario and states the tag rule instead of the version.

## ADDED Requirements

### Requirement: team-identity builds on a tagged go-sdk release with the ADR-0016 layout

`team-identity/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), SHALL contain no `replace` directive and SHALL pin golangci-lint with a `tool` directive. The repo SHALL contain no `third_party` directory. The code SHALL be laid out as `cmd/server/main.go`, `internal/app`, `internal/config`, `internal/rpc`, `internal/identity`, `internal/store`, `internal/events`, `internal/upstream`, `internal/token` and `internal/authz`; the packages `internal/handler`, `internal/service`, `internal/repository`, `internal/interceptor`, `internal/grpcserver`, `internal/bootstrap` and `internal/observability` SHALL NOT exist. `internal/identity` SHALL NOT import `google.golang.org/grpc`. The `Makefile` SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`.

#### Scenario: The module requires a tagged SDK release
- **WHEN** the layout test reads `go.mod`
- **THEN** it finds exactly one require line for the SDK module, at a version matching `^v\d+\.\d+\.\d+$`, the golangci-lint `tool` line, no `replace` line and no `third_party` directory
- **VERIFIED BY**: team-identity `internal/app` `TestModuleRequiresSDKTag`

#### Scenario: A legacy package or a transport import in the domain is refused
- **WHEN** the layout test lists `internal/` and the imports of `internal/identity`
- **THEN** none of the seven legacy package directories exists and `internal/identity` does not import `google.golang.org/grpc`, otherwise the test fails
- **VERIFIED BY**: team-identity `internal/app` `TestLayoutFollowsADR0016`

#### Scenario: The Makefile uses the shared files without copies
- **WHEN** the layout test reads `Makefile` and lists the repo root
- **THEN** the Makefile includes `../platform-core/make/go-service.mk`, names `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, and no `.golangci.yml` or `go-service.mk` file exists under `team-identity`
- **VERIFIED BY**: team-identity `internal/app` `TestMakefileUsesSharedFiles`

## MODIFIED Requirements

### Requirement: Logs identify the request and never carry secrets

Every service log line SHALL be JSON with `service` `team-identity`, and call logs SHALL carry the request id and, for an authenticated call, the keyed principal fingerprint `principal_fp` (never the raw principal id). The service SHALL NOT log a password, an access token, a refresh token, a refresh hash, a reset token, an address field, an ip, a user agent or a status note, in a log line or a span attribute.

#### Scenario: Call logs carry request id and principal id
- **WHEN** an authenticated `ListSessions` call with `x-request-id` `abc` completes
- **THEN** the captured call log has `request_id` `abc`, a `principal_fp` that differs from the raw principal id, no `principal_id` key and `service` `team-identity`
- **VERIFIED BY**: team-identity `internal/app` `TestCallLogsCarryRequestAndPrincipalID`

#### Scenario: A refused call logs the principal and no request body
- **WHEN** a seller principal calls `SetUserStatus` with a marker note and is refused with `PermissionDenied` (403)
- **THEN** the captured call log carries `principal_fp`, no `principal_id` key and the code `PermissionDenied`, and zero captured lines or span attributes contain the note marker
- **VERIFIED BY**: team-identity `internal/app` `TestDeniedCallLogsCarryNoRequestBody`

#### Scenario: Secrets never reach logs or spans
- **WHEN** Register, Login, RefreshSession (rotation, grace replay and reuse), RequestPasswordReset and SetUserStatus with a marker note run with log and span capture enabled
- **THEN** zero captured log lines and span attributes contain the password, either token, the refresh hash, the reset token or the note marker
- **VERIFIED BY**: team-identity `internal/app` `TestSecretsNeverReachLogsOrSpans`
