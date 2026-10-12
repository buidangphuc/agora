Code track only: there is no user-facing behaviour change. Every scenario names its test in a `VERIFIED BY` line and gets a `not-testable` entry in `team-sharing/FEATURES.yaml`.

## 1. Migrate team-sharing (single packet)

Owns: `team-sharing/**`, `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md` (migration section only), `platform-core/packages/go-sdk/examples/hello/**` (migration location only), `platform-core/packages/go-sdk/README.md` (migration lines only), plus the box ticks in this file.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.1.0 team-sharing`, then `buf generate` so `generated/` exists for building (it stays gitignored). Verify with `go build ./...`.
- [x] 1.2 Write the new tests first and see them fail: `TestBootFailsWhenDatabaseUnreachableOutsideLocal`, `TestLocalFallsBackToMemoryStore`, `TestHealthReportsDatabaseDown`, `TestCreateAnonymousStoresEmptyCreator` and `TestEnvExampleInSync`.
- [x] 1.3 Restructure per ADR-0016 and `platform-core/packages/go-sdk/examples/hello`:
  - `internal/handler` → `internal/rpc`
  - `internal/service` → `internal/sharelink`
  - `internal/repository` → `internal/store`; `migrations/` stays at the root, with a new `migrations/embed.go` (package `migrations`, `//go:embed *.sql`) for tests
  - add `internal/app`, and rewrite `internal/config` on SDK `config`
  - delete `internal/grpcserver` and `internal/interceptor`
  
  Move the existing tests with their packages and keep their assertions. Verify that the existing behaviour tests named in the spec are green.
- [x] 1.4 `Makefile` includes `make/go-service.mk` (`SERVICE=team-sharing`, `GRPC_PORT=50065`). The `Dockerfile` is from the template. `.env.example` is in sync. Verify `make check` and `make docker` (then remove the image).
- [x] 1.5 Amend ADR-0016, the README and `examples/hello`: migrations live at `<repo>/migrations` with `embed.go`, applied by a migrate job, not at boot. In the example, move `internal/store/migrations` to `migrations/` and drop migrate-at-boot from `app.Run` (keep `TestMigrationsApply` via the root package; replace `TestMigrationFailureFailsBootLocally` with `TestBootFailsWhenDatabaseUnreachableOutsideLocal` coverage that already exists; delete the migration-failure test). Verify `make -C platform-core/packages/go-sdk check`.
- [x] 1.6 Add `team-sharing/FEATURES.yaml`, with one `not-testable` entry per spec scenario whose title equals the scenario title. Update `README.md` to describe the new layout and how to run. Verify with `make -C platform-e2e features-check` and `scripts/repo_doctor.py`.

## 2. Trim always-on enable flags (go-sdk v0.1.1)

Owns: `platform-core/packages/go-sdk/**`, `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md`, `platform-core/FEATURES.yaml` (one entry's acceptance lines), `team-sharing/**`.

- [x] 2.1 SDK: remove `Postgres.Enabled` (`DATABASE_ENABLED`) and `GRPC.ReflectionEnabled` (`GRPC_REFLECTION_ENABLED`). `Postgres.Validate` rejects an empty URL. `grpcx.NewServer` always registers reflection. Update the tests (rename `TestLoadRunsValidate` scenario input), `examples/hello` (`.env.example`, app wiring) and the ADR/README env tables. Verify `make -C platform-core/packages/go-sdk check`.
- [x] 2.2 Update the acceptance lines (not the title) of the FEATURES entry "Enabled database without a URL fails validation" in `platform-core/FEATURES.yaml` to match the modified scenario.
- [ ] 2.3 (orchestrator) Tag `go-sdk/v0.1.1` on the 2.1 commit and push the tag.
- [ ] 2.4 team-sharing: re-sync to `go-sdk/v0.1.1`, drop `DATABASE_ENABLED` from Settings use, `.env.example`, README and tests. The database is always opened; the local in-memory fallback still triggers on an unreachable database with `ENV=local`. Verify `make -C team-sharing check`.
