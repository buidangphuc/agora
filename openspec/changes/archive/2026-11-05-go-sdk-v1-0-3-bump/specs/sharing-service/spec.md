## MODIFIED Requirements

### Requirement: The service consumes the tagged SDK and the shared build files

`team-sharing/go.mod` SHALL contain exactly one `require` for `github.com/buidangphuc/agora/platform-core/packages/go-sdk`, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), SHALL pin golangci-lint as a `tool` directive, SHALL NOT contain a `replace` directive, and the repository SHALL NOT contain `team-sharing/third_party`, `team-sharing/make` or `team-sharing/.golangci.yml`. `team-sharing/Makefile` SHALL include `../platform-core/make/go-service.mk` and pass `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`. `cmd/server/main.go` SHALL only create the signal context, call `config.Load` and `app.Run`, and exit non-zero on error.

#### Scenario: go.mod requires the tag without replace
- **WHEN** `team-sharing/go.mod` is parsed
- **THEN** it requires the go-sdk module exactly once, at a version matching `^v\d+\.\d+\.\d+$`, has a golangci-lint `tool` line and has no `replace` directive
- **VERIFIED BY**: `team-sharing/internal/app` `TestGoModRequiresTaggedSDKWithoutReplace`

#### Scenario: No vendored or copied build files
- **WHEN** the paths `team-sharing/third_party`, `team-sharing/make` and `team-sharing/.golangci.yml` are looked up
- **THEN** none exists, and `team-sharing/Makefile` contains the include of `../platform-core/make/go-service.mk`
- **VERIFIED BY**: `team-sharing/internal/app` `TestNoVendoredSDKOrCopiedBuildFiles`

#### Scenario: The image builds from the module cache
- **WHEN** `docker build -t team-sharing:local team-sharing` runs with the repository `Dockerfile`
- **THEN** the build exits 0
- **VERIFIED BY**: `docker build -t team-sharing:local team-sharing` (tasks.md 1.8, run by `make -C team-sharing docker`)

### Requirement: Calls are logged and panics are contained

Every gRPC call to team-sharing SHALL pass through the `grpcx` chain: request id, panic recovery, call logging with request id and the keyed principal fingerprint `principal_fp` (never the raw principal id), and the `auth` interceptor. A panic in a handler SHALL reach the caller as `Internal` with message `internal error` and SHALL NOT stop the process. A call log record SHALL NOT contain the `short_code`, `target_id`, `utm` values or `og_*` text.

#### Scenario: A panicking handler returns Internal and the server keeps serving
- **WHEN** the store panics during one `ResolveShareLink` and a second `ResolveShareLink` for a good code follows
- **THEN** the first call fails with `Internal` `internal error` and the second succeeds
- **VERIFIED BY**: `team-sharing/internal/app` `TestPanicIsContained`

#### Scenario: Call logs carry no link data
- **WHEN** `CreateShareLink` is called for `listing` / `listing-8f3a9c` with an `og_title` of `Secret title`, and the log output of the call is read
- **THEN** one record names the method and carries `principal_fp` equal to `logx.PrincipalFP("user-1")` and no `principal_id` key, and no record contains `user-1`, `listing-8f3a9c`, `Secret title` or the returned short code
- **VERIFIED BY**: `team-sharing/internal/app` `TestCallLogsCarryNoLinkData`

#### Scenario: An anonymous call is logged without a principal id
- **WHEN** `ResolveShareLink` is called with `x-principal-id=anonymous` and the log output of the call is read
- **THEN** one record names the method and its code, no record carries `principal_id` or `principal_fp`, and the call is not rejected as unauthenticated
- **VERIFIED BY**: `team-sharing/internal/app` `TestAnonymousCallIsLoggedAndServed`
