Code track only: there is no user-facing behaviour change. Each new scenario names its test in a `VERIFIED BY` line and gets a `not-testable` entry in `team-domain/FEATURES.yaml`. The e2e scenario reuses the existing automated features.

## 1. Migrate team-domain (single packet)

Owns: `team-domain/**`, plus the box ticks in this file.

- [ ] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.1.0 team-domain`, then `buf generate`. Verify with `go build ./...`.
- [ ] 1.2 Write the new tests first and see them fail: `TestBootRequiresDatabase`, `TestBootFailsWhenDatabaseUnreachable` and `TestShutdownStopsRelayerBeforePoolClose`. Use testkit Postgres and a fake publisher.
- [ ] 1.3 Restructure as the proposal's table says. Delete the addon registry and the old platform packages. Turn the relayer and sweeper into `lifecycle.Component`s. Use a `lifecycle.Stack` for the publisher and the pool. Move the existing tests with their packages and keep their assertions. Verify: the full suite is green, with Docker for the Postgres tests.
- [ ] 1.4 Keep the env var names and defaults (the drift test passes). `Makefile` includes `make/go-service.mk` and keeps `dev`/`stop`/`migrate`. The `Dockerfile` is from the template (`GRPC_PORT=50051`). Verify `make check` and `make docker` (then remove the image).
- [ ] 1.5 Add `not-testable` entries to `team-domain/FEATURES.yaml` for the three new scenarios (titles equal to the scenario titles). Update `README.md` for the layout. Verify with `make -C platform-e2e features-check`.
- [ ] 1.6 (orchestrator) Rebuild team-domain from this branch in the local stack and run its automated platform-e2e features.
