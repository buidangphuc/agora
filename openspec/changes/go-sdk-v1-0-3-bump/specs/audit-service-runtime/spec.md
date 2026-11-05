## REMOVED Requirements

### Requirement: team-audit builds on the tagged go-sdk with the ADR-0016 layout

**Reason**: The requirement pinned go-sdk `v1.0.2` and its scenario title named that version; a MODIFIED block cannot rename a scenario.

**Migration**: Replaced by the ADDED requirement "team-audit builds on a tagged go-sdk release with the ADR-0016 layout", which keeps every clause and scenario and states the tag rule instead of the version.

## ADDED Requirements

### Requirement: team-audit builds on a tagged go-sdk release with the ADR-0016 layout

`team-audit/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), SHALL contain no `replace` directive and SHALL pin golangci-lint as a `tool` directive. The repo SHALL contain no `third_party` directory. The code SHALL be laid out as `cmd/server/main.go`, `internal/app`, `internal/config`, `internal/rpc`, `internal/audit` and `internal/store`; the packages `internal/handler`, `internal/service`, `internal/repository`, `internal/interceptor` and `internal/grpcserver` SHALL NOT exist. `internal/audit` SHALL NOT import `google.golang.org/grpc` or generated code. The `Makefile` SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`.

#### Scenario: The module requires a tagged SDK release
- **WHEN** the layout test reads `go.mod`
- **THEN** it finds exactly one require line for the SDK module, at a version matching `^v\d+\.\d+\.\d+$`, the golangci-lint `tool` line, no `replace` line and no `third_party` directory
- **VERIFIED BY**: team-audit `internal/app` `TestModuleRequiresSDKTag`

#### Scenario: A legacy package or a transport import in the domain is refused
- **WHEN** the layout test lists `internal/` and the imports of `internal/audit`
- **THEN** none of the five legacy package directories exists and `internal/audit` imports neither `google.golang.org/grpc` nor `generated/`, otherwise the test fails
- **VERIFIED BY**: team-audit `internal/app` `TestLayoutFollowsADR0016`

#### Scenario: The Makefile uses the shared files without copies
- **WHEN** the layout test reads `Makefile` and lists the repo root
- **THEN** the Makefile includes `../platform-core/make/go-service.mk`, names `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, and no `.golangci.yml` or `go-service.mk` file exists under `team-audit`
- **VERIFIED BY**: team-audit `internal/app` `TestMakefileUsesSharedFiles`
