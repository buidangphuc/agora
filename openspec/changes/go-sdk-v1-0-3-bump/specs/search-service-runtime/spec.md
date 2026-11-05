## MODIFIED Requirements

### Requirement: team-search is built from the tagged SDK and ships both binaries

`team-search/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), SHALL contain no `replace` directive and SHALL pin golangci-lint with a `tool` directive. The repository SHALL contain no `third_party` directory. The `Makefile` SHALL include `../platform-core/make/go-service.mk` and the lint config `../platform-core/packages/go-sdk/.golangci.yml` by reference. The image SHALL contain `/server` and `/indexer`, with entrypoint `/server`.

#### Scenario: go.mod requires the tag without a replace

- **WHEN** `team-search/go.mod` is parsed
- **THEN** it requires the SDK exactly once, at a version matching `^v\d+\.\d+\.\d+$`, has no `replace` directive and has a golangci-lint `tool` directive
- **VERIFIED BY**: `team-search/internal/config` `TestGoModRequiresTaggedSDK`

#### Scenario: A vendored SDK copy is rejected

- **WHEN** the file tree of `team-search` is scanned
- **THEN** no `third_party` directory and no local `.golangci.yml` exists
- **VERIFIED BY**: `team-search/internal/config` `TestNoVendoredSDKOrLintCopy`

#### Scenario: The image builds both binaries

- **WHEN** `docker build` runs in `team-search`
- **THEN** the image holds `/server` and `/indexer` and its entrypoint is `/server`
- **VERIFIED BY**: `team-search/internal/config` `TestDockerfileBuildsBothBinaries` (reads the Dockerfile) and the `docker build` command of the last code task

### Requirement: Request id and trace context are carried by the SDK interceptors

The query server SHALL echo or generate `x-request-id` and continue an incoming W3C `traceparent` through the `grpcx` interceptors, and SHALL emit one `grpc.request` log line per call with method, code, duration, request id and the keyed principal fingerprint `principal_fp` (never the raw principal id) (never the request body).

#### Scenario: A request id is logged and a trace is continued

- **WHEN** a client calls `SearchListings` with metadata `x-request-id: req-42` and a valid `traceparent`
- **THEN** the call log line carries `request_id` `req-42`, the active span in the handler has the incoming trace id, and the log line holds no query text
- **VERIFIED BY**: `team-search/internal/app` `TestRequestIDAndTraceparentPropagate`

#### Scenario: A malformed traceparent does not fail the call

- **WHEN** a client calls `SearchListings` with no `x-request-id` and a malformed `traceparent`
- **THEN** the call is not rejected, a generated request id appears in the log line, and a new trace is started
- **VERIFIED BY**: `team-search/internal/app` `TestRequestIDGeneratedAndBadTraceparentIgnored`
