## MODIFIED Requirements

### Requirement: team-analytics builds on the tagged go-sdk with shared tooling

The `team-analytics` module SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), with no `replace` directive and no `third_party` directory, and SHALL pin `golangci-lint` as a `tool` directive.

#### Scenario: go.mod requires the SDK tag
- **WHEN** `go.mod` of team-analytics is parsed
- **THEN** it has exactly one require of the SDK module, at a version matching `^v\d+\.\d+\.\d+$`, no `replace` directive and a `tool` directive for golangci-lint
- **VERIFIED BY**: team-analytics `internal/app` `TestGoModRequiresSDKTagWithoutReplace`

#### Scenario: A vendored SDK copy is refused
- **WHEN** the team-analytics tree is inspected for `third_party/`, a service-local `.golangci.yml` and a copy of `go-service.mk`
- **THEN** none exists
- **VERIFIED BY**: team-analytics `internal/app` `TestNoVendoredSDKOrLocalLintCopy`
