# ADR-0018 — Go SDK tagged module distribution

**Status:** Accepted · **Date:** 2026-10-22 · **Relates to:** ADR-0016

## Context

ADR-0016 distributed the Go SDK by vendoring: `sync-go-sdk.sh` copied it into each service under `third_party/go-sdk`, a `replace` directive pointed at the copy, and a drift gate (`check-go-sdk-vendor.sh`) compared the copy to its tag. The copies drift, the service-local lint overrides pollute the shared lint file, and the module path (`github.com/buidangphuc/platform-core/...`) is not fetchable.

## Decision

Supersede the distribution part of ADR-0016. Its package and layout rules stay.

- Module path: `github.com/buidangphuc/agora/platform-core/packages/go-sdk`.
- Releases are git tags `platform-core/packages/go-sdk/vX.Y.Z`. A service `require`s a tag with `go get`; no `replace`, no `third_party`, no vendored copy.
- `sync-go-sdk.sh`, `check-go-sdk-vendor.sh` and their tests are removed, with their gate steps.
- Services include the shared `platform-core/make/go-service.mk` and pass `platform-core/packages/go-sdk/.golangci.yml` by relative path. Neither is copied. A service lint exclusion is a path-scoped rule in the shared `.golangci.yml`; goimports `local-prefixes` is `github.com/buidangphuc`.
- The SDK keeps no enable flag for any dependency.
- Local and test data comes from mock data, not a mock service: the real service serves mock data through a guarded `MOCK_DATA` mode that strict environments refuse.

## Consequences

Upgrading is `go get` of a new tag plus a reviewed diff. The SDK must be tagged before a service can use a change. Each service migrates in its own change.
