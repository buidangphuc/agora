## MODIFIED Requirements

### Requirement: team-chat requires a tagged SDK and shares make and lint files by reference

`team-chat/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), SHALL NOT contain a `replace` directive and SHALL pin `golangci-lint` as a `tool`. The repository SHALL NOT contain `team-chat/third_party`, `team-chat/.golangci.yml` or `team-chat/make/`. `team-chat/Makefile` SHALL set `SERVICE := team-chat`, `GRPC_PORT := 50057` and `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml` and include `../platform-core/make/go-service.mk`. `cmd/server/main.go` SHALL only load settings and call `internal/app.Run`, or `internal/app.Migrate` for the arguments `migrate up`.

#### Scenario: go.mod requires the tag without replace
- **WHEN** `team-chat/go.mod` is parsed
- **THEN** it requires the SDK exactly once, at a version matching `^v\d+\.\d+\.\d+$`, has no `replace` line, and lists `github.com/golangci/golangci-lint` under `tool`
- **VERIFIED BY**: `team-chat/internal/config` `TestGoModRequiresTagWithoutReplace`

#### Scenario: Make and lint files are referenced, not copied
- **WHEN** the `team-chat` directory is listed and its `Makefile` read
- **THEN** no `third_party`, `.golangci.yml` or `make` entry exists, and the Makefile includes the shared make file and sets the shared `LINT_CONFIG`
- **VERIFIED BY**: `team-chat/internal/config` `TestSharedMakeAndLintAreReferenced`

#### Scenario: main stays a thin entry point
- **WHEN** `cmd/server/main.go` is read
- **THEN** it imports `internal/app` and `internal/config`, calls `app.Migrate` for the arguments `migrate up` and `app.Run` otherwise, and defines no other function than `main` and `run`
- **VERIFIED BY**: `team-chat/internal/config` `TestMainOnlyCallsAppRun`
