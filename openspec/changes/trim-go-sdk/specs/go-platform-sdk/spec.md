## MODIFIED Requirements

### Requirement: Services vendor a pinned SDK copy

`platform-core/tools/sync-go-sdk.sh <tag> <service-dir>...` SHALL, for each service directory:
- export `packages/go-sdk` at git tag `<tag>` into `<service>/third_party/go-sdk` (replacing any previous copy);
- write `third_party/go-sdk/VERSION` containing the tag and commit;
- copy `make/go-service.mk` and `.golangci.yml` into the service;
- ensure `go.mod` has `replace github.com/buidangphuc/platform-core/packages/go-sdk => ./third_party/go-sdk`;
- run `go mod tidy`.

It SHALL fail without changes if the tag does not exist. A service SHALL move to a newer tag only in a change that uses something the newer tag adds; a change that cuts an SDK tag SHALL NOT re-sync services.

#### Scenario: Sync vendors the tagged tree
- **WHEN** the script runs with an existing tag against a scratch copy of a service
- **THEN** `third_party/go-sdk/VERSION` names the tag, `go.mod` has the replace, and `go build ./...` succeeds in the service
- **VERIFIED BY**: `platform-core/tools/sync-go-sdk_test.sh`

#### Scenario: Unknown tag changes nothing
- **WHEN** the script runs with a tag that does not exist
- **THEN** it exits non-zero and the service tree is unchanged
- **VERIFIED BY**: `platform-core/tools/sync-go-sdk_test.sh`

#### Scenario: An SDK release does not re-sync services
- **WHEN** a change cuts a new `go-sdk/vX.Y.Z` tag
- **THEN** the same change does not modify any `team-*/third_party/go-sdk/VERSION`
- **VERIFIED BY**: review of the change diff (not-testable)

## REMOVED Requirements

### Requirement: Retries and circuit breaking use shared policies
**Reason**: no service imports `pkg/resilience`; one-off retry needs use failsafe-go directly.
**Migration**: none; no consumer.

### Requirement: Feature flags fail open to defaults
**Reason**: only team-order and team-promotion evaluate flags; the client is service code again.
**Migration**: team-order and team-promotion own `internal/featureflags` with the same fail-open behaviour and env names.

### Requirement: Flags can be built over any provider
**Reason**: removed with `pkg/flags`.
**Migration**: the service-local `internal/featureflags` keeps a provider seam for tests.

### Requirement: Scope checks can admit scoped anonymous callers
**Reason**: a few lines that three services already keep locally; no service vendors it.
**Migration**: none; services keep their local `requireScopes`.
