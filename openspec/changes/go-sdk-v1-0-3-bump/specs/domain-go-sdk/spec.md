## MODIFIED Requirements

### Requirement: team-domain depends on the tagged go-sdk and follows the ADR-0016 layout

`team-domain/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), SHALL contain no `replace` directive, and the repository SHALL contain no `team-domain/third_party` directory. golangci-lint SHALL be pinned as a `tool` directive in `go.mod`. `team-domain/Makefile` SHALL include `../platform-core/make/go-service.mk`, set `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, and `team-domain` SHALL hold no copy of either file. `cmd/server/main.go` SHALL only load settings and call `internal/app.Run(ctx, settings)` (or `app.Migrate` for `migrate up`). The packages `internal/bootstrap`, `internal/grpcserver`, `internal/interceptor`, `internal/observability`, `internal/handler`, `internal/service`, `internal/repository`, `internal/consumer`, `internal/upstream` and `internal/storage` SHALL NOT exist; their code SHALL live in `internal/app`, `internal/rpc`, `internal/listing`, `internal/store`, `internal/events`, `internal/screen`, `internal/audit` and `internal/objects`.

#### Scenario: Domain SDK: go.mod pins the tag without replace
- **WHEN** `team-domain/go.mod` is parsed
- **THEN** it requires go-sdk exactly once, at a version matching `^v\d+\.\d+\.\d+$`, has 0 `replace` directives, declares golangci-lint as a `tool`, and `team-domain/third_party` does not exist
- **VERIFIED BY**: team-domain `internal/app` `TestGoModRequiresTaggedSDK`

#### Scenario: Domain SDK: Makefile references the shared files
- **WHEN** `team-domain/Makefile` is read and the directory is listed
- **THEN** the Makefile includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and `team-domain/.golangci.yml` and `team-domain/make/` do not exist
- **VERIFIED BY**: team-domain `internal/app` `TestMakefileUsesSharedBuildFiles`

#### Scenario: Domain SDK: Superseded packages are gone
- **WHEN** the directories under `team-domain/internal` are listed
- **THEN** none of `bootstrap`, `grpcserver`, `interceptor`, `observability`, `handler`, `service`, `repository`, `consumer`, `upstream`, `storage` exists and each of `app`, `config`, `rpc`, `listing`, `store`, `events`, `objects`, `screen`, `audit` does
- **VERIFIED BY**: team-domain `internal/app` `TestLegacyPackagesRemoved`
