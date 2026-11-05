## MODIFIED Requirements

### Requirement: team-order is built on the tagged go-sdk with the ADR-0016 layout

team-order SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once in `go.mod`, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), with no `replace` directive and no `third_party` directory. Its `Makefile` SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`, and the service SHALL hold no copy of either file. `golangci-lint` SHALL be pinned with a `tool` directive in `go.mod`. The binaries SHALL be `cmd/server/main.go`, which loads settings and calls `internal/app.Run(ctx, settings)`, and `cmd/resync-commits/main.go`. The packages `internal/bootstrap`, `internal/featureflags`, `internal/interceptor`, `internal/grpcserver`, `internal/handler`, `internal/service`, `internal/repository` and `internal/consumer` SHALL NOT exist.

#### Scenario: go.mod requires the tag and nothing local
- **WHEN** the module file of team-order is parsed
- **THEN** it has exactly one `require` of the SDK module, at a version matching `^v\d+\.\d+\.\d+$`, zero `replace` directives and a `tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint` line
- **VERIFIED BY**: team-order `internal/app` `TestGoModRequiresTaggedSDK`

#### Scenario: Shared make and lint files are referenced without a local copy
- **WHEN** the `Makefile` and the service directory are inspected
- **THEN** the `Makefile` includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and neither `.golangci.yml`, `make/go-service.mk` nor `third_party` exists in `team-order`
- **VERIFIED BY**: team-order `internal/app` `TestSharedMakeAndLintAreReferenced`

#### Scenario: Hand-rolled infrastructure packages are gone
- **WHEN** the directories under `team-order/internal` are listed
- **THEN** `bootstrap`, `featureflags`, `interceptor`, `grpcserver`, `handler`, `service`, `repository` and `consumer` are absent, and `app`, `config`, `rpc`, `order`, `store`, `events` and `upstream` are present
- **VERIFIED BY**: team-order `internal/app` `TestLayoutHasNoLegacyPackages`
