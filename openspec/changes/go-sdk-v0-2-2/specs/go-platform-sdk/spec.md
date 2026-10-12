## ADDED Requirements

### Requirement: Scope checks can admit scoped anonymous callers

`auth.RequireScopesAllowAnonymous(ctx, scopes...)` SHALL return `codes.Unauthenticated` when no principal is present, and `codes.PermissionDenied` when any listed scope is missing. It SHALL return the principal otherwise, including an anonymous principal that holds every listed scope.

#### Scenario: Anonymous caller with the public scope is admitted
- **WHEN** the anonymous principal holds `listing.read` and `RequireScopesAllowAnonymous(ctx, "listing.read")` is called
- **THEN** it returns the principal and no error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auth` `TestRequireScopesAllowAnonymousAdmitsScopedAnonymous`

#### Scenario: Anonymous caller without the scope is denied, not unauthenticated
- **WHEN** the anonymous principal lacks `listing.write`
- **THEN** it returns `PermissionDenied`
- **VERIFIED BY**: `TestRequireScopesAllowAnonymousDeniesMissingScope`

### Requirement: Sync keeps service-local lint overrides

`sync-go-sdk.sh` SHALL keep the lines between `# BEGIN service-local` and `# END service-local` in the service's `.golangci.yml` when it replaces that file. The SDK `.golangci.yml` SHALL contain an empty block of that shape inside `linters.exclusions.rules`.

#### Scenario: Re-sync does not drop a service exclusion
- **WHEN** a service has an exclusion inside its service-local block and the script re-syncs
- **THEN** the new `.golangci.yml` contains that exclusion inside the block and `golangci-lint config verify` passes
- **VERIFIED BY**: `platform-core/tools/sync-go-sdk_test.sh`

### Requirement: Vendored SDK copies match their tag

`check-go-sdk-vendor.sh` SHALL, for each `team-*/third_party/go-sdk/VERSION`, compare the vendored tree with `git archive <tag>:platform-core/packages/go-sdk`. It SHALL exit non-zero and name each file that differs or is missing.

#### Scenario: A hand-edited vendored file fails the check
- **WHEN** one file under a service's `third_party/go-sdk` is edited
- **THEN** the check exits non-zero and names that service and file
- **VERIFIED BY**: `platform-core/tools/check-go-sdk-vendor_test.sh`

### Requirement: The Go service template follows ADR-0016

`platform-core/templates/_service-go` SHALL mirror `examples/hello`: the layout, `internal/app` wiring, `Makefile` including `make/go-service.mk`, `.env.example`, and a README with the sync and rename steps. It SHALL NOT contain its own interceptors or platform code.

#### Scenario: A service scaffolded from the template builds without platform copies
- **WHEN** the template is copied to a scratch dir, renamed, and synced to the current tag
- **THEN** `go build ./...` and `go vet ./...` pass and no `internal/interceptor` or `internal/grpcserver` exists
- **VERIFIED BY**: `platform-core/tools/template_smoke_test.sh`
