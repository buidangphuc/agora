Code track only: there is no user-facing behaviour change. Every scenario gets a `not-testable` entry in `team-analytics/FEATURES.yaml`.

## 1. Migrate team-analytics (single packet)

Owns: `team-analytics/**`, the team-analytics blocks in `docker-compose.services.yaml`, `platform-gitops/argocd/apps/team-analytics.yaml`, `platform-gitops/platform/team-analytics/*.yaml`, and the box ticks.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.2.1 team-analytics`. Verify `go build ./...`.
- [x] 1.2 Write the spec tests first and see them red.
- [x] 1.3 Restructure per the proposal. Move the existing tests with their packages. Verify `make check` (Docker).
- [x] 1.4 Remove the flags from Settings, `.env.example`, the README, compose and gitops. Add the FEATURES entries. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, `red_first`, and `make docker` (then remove the image).
