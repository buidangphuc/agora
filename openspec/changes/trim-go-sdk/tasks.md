Code track only: every scenario gets a `not-testable` entry in `platform-core/FEATURES.yaml`; entries for removed requirements are deleted (orchestrator, `## 2`).

## 1. Trim the SDK (wave 1)

Owns: `platform-core/packages/go-sdk/**`, `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md`, `platform-core/README.md` (go-sdk line only), `platform-core/templates/**` and `platform-core/tools/**` (only if they reference a removed part).

- [ ] 1.1 Delete `pkg/flags/**` and `pkg/resilience/**`; remove `config.Flags` and its tests, `auth.RequireScopesAllowAnonymous` and its tests; `go mod tidy` in the SDK; verify `go build ./... && go test ./...` in `packages/go-sdk` and `examples/hello`.
- [ ] 1.2 README of the SDK and `platform-core/README.md` no longer list `flags`, `resilience` or `RequireScopesAllowAnonymous`; verify `grep -rn 'pkg/flags\|pkg/resilience\|RequireScopesAllowAnonymous\|config.Flags' platform-core` prints only CHANGELOG-style history, if any.
- [ ] 1.3 ADR-0016: add `### What goes into the SDK` (admission rule: substantial, shared by at least three services, hard to get right per service; small helpers stay in services) and extend `### Distribution` with the bump rule; drop failsafe-go and feature flags from the library table; replace the phase-4 connection list in `## Open question`/`## Consequences` with the admission rule; verify by reading.

## 2. Close (orchestrator)

- [ ] 2.1 FEATURES: delete entries for removed requirements; add the not-testable entry for "An SDK release does not re-sync services"; run `ci_local`; tag `go-sdk/v0.3.0` locally.
