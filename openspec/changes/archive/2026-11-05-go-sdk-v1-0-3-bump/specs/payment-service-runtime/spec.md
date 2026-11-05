## MODIFIED Requirements

### Requirement: team-payment is built on the tagged go-sdk with the ADR-0016 layout

team-payment SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once in `go.mod`, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), with no `replace` directive and no `third_party` directory. Its `Makefile` SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`, and the service SHALL hold no copy of either file. `golangci-lint` SHALL be pinned with a `tool` directive in `go.mod`. The binary SHALL be `cmd/server/main.go`, which loads settings and calls `internal/app.Run(ctx, settings)`. The packages `internal/bootstrap`, `internal/interceptor` and `internal/grpcserver` SHALL NOT exist.

#### Scenario: go.mod requires the tag and nothing local
- **WHEN** the module file of team-payment is parsed
- **THEN** it has exactly one `require` of the SDK module, at a version matching `^v\d+\.\d+\.\d+$`, zero `replace` directives and a `tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint` line
- **VERIFIED BY**: team-payment `internal/app` `TestGoModRequiresTaggedSDK`

#### Scenario: Shared make and lint files are referenced, not copied
- **WHEN** the `Makefile` and the service directory are inspected
- **THEN** the `Makefile` includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and neither `.golangci.yml`, `make/go-service.mk` nor `third_party` exists in `team-payment`
- **VERIFIED BY**: team-payment `internal/app` `TestSharedMakeAndLintAreReferenced`

#### Scenario: A replace directive or a copied shared file fails the layout checks
- **WHEN** `go.mod` gains a `replace` directive, or a `third_party` directory or a `.golangci.yml` appears in `team-payment`
- **THEN** the layout tests fail and name the offending file or directive
- **VERIFIED BY**: team-payment `internal/app` `TestLayoutChecksRejectCopiesAndReplace`

#### Scenario: Hand-rolled infrastructure packages are gone
- **WHEN** the directories under `team-payment/internal` are listed
- **THEN** `bootstrap`, `interceptor` and `grpcserver` are absent, and `internal/app`, `internal/config`, `internal/rpc`, `internal/payment`, `internal/settlement` and `internal/store` are present
- **VERIFIED BY**: team-payment `internal/app` `TestLayoutHasNoLegacyPackages`
