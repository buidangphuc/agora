Code track only: there is no user-facing behaviour change. Every scenario gets a `not-testable` entry in `team-engagement/FEATURES.yaml`.

## 1. Migrate team-engagement (single packet)

Owns: `team-engagement/**`, the team-engagement blocks in `docker-compose.services.yaml`, `platform-gitops/argocd/apps/team-engagement.yaml`, `platform-gitops/envs/services/team-engagement.yaml`, and the box ticks.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.2.1 team-engagement`. Verify `go build ./...`.
- [x] 1.2 Write the spec tests first and see them red.
- [x] 1.3 Restructure per the proposal. Move the existing tests with their packages. Verify `make check` (Docker).
- [x] 1.4 Remove the flags from Settings, `.env.example`, the README, compose and gitops. Add the FEATURES entries. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, `red_first`, and `make docker` (then remove the image).

## 2. Use existing SDK packages instead of local copies

Behaviour, wire messages and env names stay the same; existing tests keep their assertions. Where a switch would change a client-visible message or a retry rule, leave that site and note it in the report.

Owns: `team-engagement/**` except `third_party/**`.

- [ ] 2.1 Use `pgx.WithTx` for hand-rolled transactions. Verify `make check`.
- [ ] 2.2 Full-apply migration tests use `migrate.Up` (partial from/to tests keep raw SQL). Verify `make check`.
- [ ] 2.3 Use `grpcx.Status` + `ErrorTable` where the client message stays identical. Verify `make check`.
