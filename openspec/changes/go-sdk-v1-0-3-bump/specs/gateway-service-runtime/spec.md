## RENAMED Requirements

- FROM: `### Requirement: team-gateway builds on go-sdk v1.0.2 with the ADR-0016 layout`
- TO: `### Requirement: team-gateway builds on a tagged go-sdk release with the ADR-0016 layout`

## MODIFIED Requirements

### Requirement: team-gateway builds on a tagged go-sdk release with the ADR-0016 layout

team-gateway SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), with no `replace` directive and no `third_party` copy, and SHALL pin golangci-lint as a `tool` directive. `cmd/gateway/main.go` SHALL only load `internal/config` settings and call `internal/app.Run(ctx, settings)`. The Makefile SHALL include `../platform-core/make/go-service.mk` and the SDK `.golangci.yml`. The binary SHALL NOT import a test fake, a no-op publisher or an in-memory stand-in for a dependency. The image SHALL build from `cmd/gateway` without a `third_party` directory.

#### Scenario: go.mod uses the tag without replace
- **WHEN** `go.mod` is parsed
- **THEN** it requires go-sdk exactly once, at a version matching `^v\d+\.\d+\.\d+$`, has a golangci-lint `tool` line and no `replace` directive, and no `third_party` directory exists
- **VERIFIED BY**: team-gateway `internal/app` `TestGoModRequiresSDKTagWithoutReplace`

#### Scenario: The runtime binary does not import a fake
- **WHEN** the dependency graph of `./cmd/gateway` is listed
- **THEN** it contains no package whose path ends in `test`, `fake` or `noop`
- **VERIFIED BY**: team-gateway `internal/app` `TestRuntimeDoesNotImportFakes`

#### Scenario: The Makefile uses the shared targets
- **WHEN** `make help` is run in `team-gateway`
- **THEN** the output lists `check`, `lint`, `test`, `check-env` and `docker` from `go-service.mk`
- **VERIFIED BY**: team-gateway `internal/app` `TestMakefileIncludesSharedTargets`
