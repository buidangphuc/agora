## Why

The migrations of 16 services onto go-sdk surfaced recurring gaps:

- **Anonymous scope checks.** team-domain, team-search and team-analytics each wrote a local `requireScopes`, because SDK `auth.RequireScopes` rejects the gateway's anonymous principal. Anonymous callers carry public scopes (for example `listing.read`) and must be judged by scope, getting PermissionDenied when one is missing.
- **Lint overrides lost on sync.** `sync-go-sdk.sh` overwrites a service's `.golangci.yml`, which drops the service's narrow exclusions (team-search).
- **Dockerfile port default.** The template Dockerfile still defaults `GRPC_PORT` to 50051, so a service that forgets to override it listens on another service's port.
- **Stale template.** `platform-core/templates/_service-go` predates ADR-0016: it has `internal/handler|service|repository`, its own interceptors and no SDK.
- **No drift check.** Nothing checks that a service's `third_party/go-sdk` equals the tag recorded in its `VERSION` (ADR-0016 promised it, phase 7).

initiative: go-platform-sdk

## What Changes

- `auth.RequireScopesAllowAnonymous(ctx, scopes...)`:
  - an anonymous principal with the scopes passes;
  - a missing principal is Unauthenticated;
  - a missing scope is PermissionDenied.
- `sync-go-sdk.sh` keeps the block between `# BEGIN service-local` and `# END service-local` in the service's `.golangci.yml` when it re-syncs. The SDK `.golangci.yml` ships an empty block at the right place.
- The template Dockerfile has no `GRPC_PORT` default, and `EXPOSE` uses the build arg. `make docker` already passes it.
- `templates/_service-go` is rebuilt from `examples/hello` on the SDK: vendored SDK placeholder, layout, Makefile include, `.env.example`, and README steps.
- New `platform-core/tools/check-go-sdk-vendor.sh` verifies every `team-*/third_party/go-sdk` against the tag in its `VERSION`. It is wired into the `repo` job of `.github/workflows/asdlc-gates.yml`, which requires a fetch of tags, and into `scripts/asdlc/ci_local.sh`.
- Released as `go-sdk/v0.2.2`, local tag.

## Non-goals

Re-syncing the services to v0.2.2. Each service picks it up later; the local `requireScopes` helpers can then be removed.

## Impact

`platform-core` (go-sdk `pkg/auth`, `.golangci.yml`, tools, templates), CI workflow `repo` job, and `scripts/asdlc/ci_local.sh`.

## Non-functional requirements

The vendor check runs in under 30s on CI.

## Data classification

internal; unchanged.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Vendored SDK | Tampering: local edits to `third_party/go-sdk` | Drift check fails CI |
| Public RPCs | Elevation of privilege: anonymous principal passes without scope | `RequireScopesAllowAnonymous` still requires every listed scope |
