Code track only: every scenario gets a `not-testable` entry in `platform-core/FEATURES.yaml`; entries for removed requirements are deleted (orchestrator, `## 2`).

## 1. Trim the SDK (wave 1)

Owns: `platform-core/packages/go-sdk/pkg/auth/**`, `platform-core/packages/go-sdk/README.md`, `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md`.

- [x] 1.1 Remove `auth.RequireScopesAllowAnonymous` and its tests; verify `go build ./... && go test ./...` in `packages/go-sdk`.
- [x] 1.2 The SDK README no longer lists `RequireScopesAllowAnonymous`.
- [x] 1.3 ADR-0016: add `### What goes into the SDK` (business-agnostic infrastructure only; no business logic) and extend `### Distribution` with the bump and approval rules; verify by reading.

## 2. Close (orchestrator)

- [x] 2.1 FEATURES: delete entries for the removed requirement; add the not-testable entry for "An SDK release does not re-sync services"; run `ci_local`. No tag: the next tag is proposed to the owner first.
