Code track only: every scenario gets a `not-testable` entry in `platform-core/FEATURES.yaml` (orchestrator, `## 2`).

## 1. ADR rules

Owns: `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md`.

- [x] 1.1 ADR-0016: add `### What goes into the SDK` (business-agnostic infrastructure only; no business logic) and extend `### Distribution` with the bump and approval rules; verify by reading.

## 2. Close (orchestrator)

- [x] 2.1 FEATURES: add the not-testable entry for "An SDK release does not re-sync services"; run `ci_local`. No tag.
