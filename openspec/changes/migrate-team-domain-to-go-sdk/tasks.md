Code track only: there is no user-facing behaviour change. Each scenario names its test in a `VERIFIED BY` line and gets a `not-testable` entry in `team-domain/FEATURES.yaml`. The orchestrator also runs the existing automated listing e2e features against the rebuilt service (1.7).

## 1. Migrate team-domain (single packet)

Owns: `team-domain/**`, plus the box ticks in this file.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.1.0 team-domain`, then `buf generate`. Verify with `go build ./...`.
- [x] 1.2 Write the new tests first and see them fail: `TestBootRequiresDatabase`, `TestBootFailsWhenDatabaseUnreachable` and `TestShutdownStopsRelayerBeforePoolClose`. Use testkit Postgres and a fake publisher.
- [x] 1.3 Restructure as the proposal's table says. Delete the addon registry and the old platform packages. Turn the relayer and sweeper into `lifecycle.Component`s. Use a `lifecycle.Stack` for the publisher and the pool. Move the existing tests with their packages and keep their assertions. Verify: the full suite is green, with Docker for the Postgres tests.
- [x] 1.4 Keep the env var names and defaults (the drift test passes). `Makefile` includes `make/go-service.mk` and keeps `dev`/`stop`/`migrate`. The `Dockerfile` is from the template (`GRPC_PORT=50051`). Verify `make check` and `make docker` (then remove the image).
- [x] 1.5 Add `not-testable` entries to `team-domain/FEATURES.yaml` for the three new scenarios (titles equal to the scenario titles). Update `README.md` for the layout. Verify with `make -C platform-e2e features-check`.
- [x] 1.6 Review fixes. Remove the `os.Setenv("GRPC_PORT", …)` default from `config.Load`: compose (`GRPC_PORT=50051`), the Dockerfile `ENV` and `.env.example` already set it, and ADR-0016 says `GRPC_PORT` has no default. `TestLoadDefaults` sets `GRPC_PORT` explicitly; that is the only assertion-setup change allowed. Add or identify `TestAnonymousStockCallIsPermissionDenied`. Add `not-testable` FEATURES entries for "The publisher is not used after it is closed", "Existing suites stay green after the move" and "Anonymous callers are still denied stock RPCs with PermissionDenied". Verify `make check`, `features-check` and `spec_sync --strict`.
- [ ] 1.7 (orchestrator) Rebuild team-domain from this branch in the local stack and run its automated platform-e2e features.
