Code track only: there is no user-facing behaviour change. Every scenario names its test and gets a `not-testable` entry in `team-audit/FEATURES.yaml` (create the file if it is missing).

## 1. Migrate team-audit (single packet)

Owns: `team-audit/**`, the `team-audit` service and migrate blocks in `docker-compose.services.yaml`, and the box ticks in this file.

- [ ] 1.1 Run `buf generate`, then `platform-core/tools/sync-go-sdk.sh go-sdk/v0.1.1 team-audit`. Verify with `go build ./...`.
- [ ] 1.2 Write the new spec tests first and see them fail.
- [ ] 1.3 Restructure per the proposal, copying team-sharing's wiring (`team-sharing/internal/app/app.go`). Move the existing tests with their packages. Verify `make check` (Docker).
- [ ] 1.4 Clean the team-audit compose block of removed flags. Write `.env.example`, the README and the FEATURES entries. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, and `make docker` (then remove the image).
