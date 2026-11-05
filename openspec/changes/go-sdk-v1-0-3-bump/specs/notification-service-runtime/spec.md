## MODIFIED Requirements

### Requirement: team-notification depends on the tagged SDK and follows the Go service layout

`team-notification` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once in `go.mod`, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), SHALL pin golangci-lint as a `tool` directive, SHALL NOT contain a `replace` directive or a `third_party/` directory, and SHALL include `../platform-core/make/go-service.mk` from its `Makefile` with `LINT_CONFIG` pointing at the SDK `.golangci.yml`, without copies of either file. `cmd/server/main.go` SHALL only create the signal context, call `config.Load` and `app.Run`, and exit non-zero on error. The packages `internal/bootstrap`, `internal/grpcserver`, `internal/interceptor`, `internal/handler`, `internal/repository`, `internal/service`, `internal/consumer` and `internal/upstream` SHALL NOT exist.

#### Scenario: The module requires the SDK tag without a replace
- **WHEN** `go.mod` and the service tree are inspected
- **THEN** `go.mod` has exactly one `require` for the SDK module, at a version matching `^v\d+\.\d+\.\d+$`, a golangci-lint `tool` line, no `replace` line, no `third_party` directory, and none of the removed `internal/` packages exists
- **VERIFIED BY**: team-notification `internal/app` `TestModuleRequiresSDKTagWithoutReplace`

#### Scenario: A go.mod with a replace directive is rejected
- **WHEN** the module check runs on a `go.mod` that contains `replace github.com/buidangphuc/agora/platform-core/packages/go-sdk => ../x` or requires the SDK at a pseudo-version, at a `-` suffixed version or twice, or has no golangci-lint `tool` line
- **THEN** the check returns an error naming the offending line
- **VERIFIED BY**: team-notification `internal/app` `TestModuleCheckRejectsReplaceAndWrongVersion`

#### Scenario: The Makefile reuses the shared targets
- **WHEN** the `Makefile` is read
- **THEN** it sets `SERVICE := team-notification` and `GRPC_PORT := 50058`, includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and no `.golangci.yml` or `go-service.mk` copy exists in the service
- **VERIFIED BY**: team-notification `internal/app` `TestMakefileIncludesSharedGoServiceMk`
