Code track only: there is no user-facing behaviour change. Each scenario names its test in a `VERIFIED BY` line and gets a `not-testable` entry in `team-domain/FEATURES.yaml`. The orchestrator also runs the existing automated listing e2e features against the rebuilt service (1.7).

## 1. Migrate team-domain (single packet)

Owns: `team-domain/**`; for 1.8 also the team-domain entries in `platform-gitops/argocd/apps/team-domain.yaml`, `platform-gitops/envs/services/team-domain.yaml` and the team-domain service block in `docker-compose.services.yaml`; plus the box ticks in this file.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.1.0 team-domain`, then `buf generate`. Verify with `go build ./...`.
- [x] 1.2 Write the new tests first and see them fail: `TestBootRequiresDatabase`, `TestBootFailsWhenDatabaseUnreachable` and `TestShutdownStopsRelayerBeforePoolClose`. Use testkit Postgres and a fake publisher.
- [x] 1.3 Restructure as the proposal's table says. Delete the addon registry and the old platform packages. Turn the relayer and sweeper into `lifecycle.Component`s. Use a `lifecycle.Stack` for the publisher and the pool. Move the existing tests with their packages and keep their assertions. Verify: the full suite is green, with Docker for the Postgres tests.
- [x] 1.4 Keep the env var names and defaults (the drift test passes). `Makefile` includes `make/go-service.mk` and keeps `dev`/`stop`/`migrate`. The `Dockerfile` is from the template (`GRPC_PORT=50051`). Verify `make check` and `make docker` (then remove the image).
- [x] 1.5 Add `not-testable` entries to `team-domain/FEATURES.yaml` for the three new scenarios (titles equal to the scenario titles). Update `README.md` for the layout. Verify with `make -C platform-e2e features-check`.
- [x] 1.6 Review fixes. Remove the `os.Setenv("GRPC_PORT", …)` default from `config.Load`: compose (`GRPC_PORT=50051`), the Dockerfile `ENV` and `.env.example` already set it, and ADR-0016 says `GRPC_PORT` has no default. `TestLoadDefaults` sets `GRPC_PORT` explicitly; that is the only assertion-setup change allowed. Add or identify `TestAnonymousStockCallIsPermissionDenied`. Add `not-testable` FEATURES entries for "The publisher is not used after it is closed", "Existing suites stay green after the move" and "Anonymous callers are still denied stock RPCs with PermissionDenied". Verify `make check`, `features-check` and `spec_sync --strict`.
- [x] 1.7 Drop `KAFKA_ENABLED` and `OUTBOX_ENABLED`: always wire the Kafka publisher and the relayer, and remove the strict-config `KAFKA_ENABLED=false` check, the no-op publisher path in app wiring (keep `NoopPublisher` only if tests use it) and both keys from `.env.example`, README and tests. Add `TestNoEnableFlagsDeclared` (Settings declares none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `GRPC_REFLECTION_ENABLED`). Verify `make check`.
- [x] 1.8 Re-sync to `go-sdk/v0.1.1` (after the orchestrator tags it): drop `DATABASE_ENABLED` and `GRPC_REFLECTION_ENABLED` uses; `TestBootRequiresDatabase` now covers a missing `DATABASE_URL`; retitle the FEATURES entry to "Missing database URL is rejected". Remove `KAFKA_ENABLED` from `platform-gitops/argocd/apps/team-domain.yaml` and `platform-gitops/envs/services/team-domain.yaml` and from the team-domain block in `docker-compose.services.yaml`, and `OUTBOX_ENABLED` there too. Verify `make check`, `features-check`, `spec_sync --strict`.
- [x] 1.9 (orchestrator) Rebuild team-domain from this branch in the local stack and run its automated platform-e2e features.

## 2. Use existing SDK packages instead of local copies

Behaviour, wire messages and env names stay the same; existing tests keep their assertions. Where a switch would change a client-visible message or a retry rule, leave that site and note it in the report.

Owns: `team-domain/**` except `third_party/**` (domain: the re-sync writes `third_party/**`).

- [x] 2.1 Re-sync to the existing tag `go-sdk/v0.2.1` (`platform-core/tools/sync-go-sdk.sh go-sdk/v0.2.1 team-domain`); do not touch other services. Verify `make check`.
- [x] 2.2 Replace the own outbox store, relayer and Kafka producer (`internal/store/outbox_pg.go`, `internal/events/relayer.go`, `internal/events/publisher.go` client setup) with SDK `outbox.NewStore`/`NewRelayer` and `kafkax.NewProducer`; use `config.Kafka`/`config.Outbox` keeping the current env names via tags; replace `afterServer` with `lifecycle.StopAfter`. Compare the existing outbox table with `outbox.SchemaSQL`; if they differ add a forward-only migration that keeps the previous release working. Verify `make check`.
- [x] 2.3 Use `pgx.WithTx` for hand-rolled `Begin/Commit/Rollback` blocks. Verify `make check`.
- [x] 2.4 Tests that apply every migration use `migrate.Up` instead of globbing `*.up.sql`. Verify `make check`.
- [x] 2.5 Use `grpcx.Status` + `ErrorTable` where the client message stays identical. Verify `make check`.

## 3. Adopt go-sdk v1.0.0

Follow `platform-core/packages/go-sdk/README.md` "Upgrading to v1" and "Outbox table migration" at tag `go-sdk/v1.0.0`. Behaviour, wire messages, gRPC codes and Kafka payloads stay the same unless a step below says otherwise; existing test assertions are not loosened. Any earlier unchecked "Use existing SDK packages" items are done here against v1.

Owns: `team-domain/**`, the team-domain blocks of `docker-compose*.yaml` and `platform-gitops/**` for team-domain.

- [x] 3.1 Sync `go-sdk/v1.0.0` into team-domain only and fix every breaking change (package renames, ErrorTable, constructors returning errors, ENV required, renamed duration env keys and removed `OUTBOX_MAX_ATTEMPTS`, outbox `PublishRecord`, consume Record and DLQ headers, .env.example, compose and gitops).
- [x] 3.2 Outbox table: forward-only migrations implementing `outbox.UpgradeSQL` (Prepare, Backfill, Switch in one transaction, Finish; `CONCURRENTLY` statements in their own migration files), a test that applies them to a table in the previous schema with rows and checks order, and a README deploy note (stop v0 relayers, purge published rows, requeue or delete v0 `failed` rows).
- [x] 3.3 Replace local copies with SDK packages where the SDK now has them (as in the earlier task list, against v1). Sites left local get a one-line reason in the report.
- [x] 3.4 Verify `make check`, `docker build`, red_first, openspec validate.
