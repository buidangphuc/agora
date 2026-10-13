Code track only: there is no user-facing behaviour change. Every scenario gets a `not-testable` entry in `team-promotion/FEATURES.yaml`.

## 1. Migrate team-promotion (single packet)

Owns: `team-promotion/**`, the team-promotion blocks in `docker-compose.services.yaml`, `platform-gitops/argocd/apps/team-promotion.yaml`, `platform-gitops/envs/services/team-promotion.yaml`, and the box ticks.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.2.1 team-promotion`. Verify `go build ./...`.
- [x] 1.2 Write the spec tests first and see them red.
- [x] 1.3 Restructure per the proposal. Move the existing tests with their packages. Verify `make check` (Docker).
- [x] 1.4 Remove the flags from Settings, `.env.example`, the README, compose and gitops. Add the FEATURES entries. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, `red_first`, and `make docker` (then remove the image).

## 2. Use existing SDK packages instead of local copies

Behaviour, wire messages and env names stay the same; existing tests keep their assertions. Where a switch would change a client-visible message or a retry rule, leave that site and note it in the report.

Owns: `team-promotion/**` except `third_party/**`.

- [ ] 2.1 `internal/events/emitter.go` reads the request id with `reqctx.RequestID(ctx)` instead of incoming metadata, if the value is the same in every call path. Verify `make check`.
