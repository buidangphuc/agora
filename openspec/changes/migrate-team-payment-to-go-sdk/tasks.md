Code track only: there is no user-facing behaviour change. Every scenario gets a `not-testable` entry in `team-payment/FEATURES.yaml`.

## 1. Migrate team-payment (single packet)

Owns: `team-payment/**`, the team-payment blocks in `docker-compose.services.yaml`, `platform-gitops/argocd/apps/team-payment.yaml`, `platform-gitops/envs/services/team-payment.yaml`, and the box ticks.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.2.1 team-payment`. Verify `go build ./...`.
- [x] 1.2 Write the spec tests first and see them red.
- [x] 1.3 Restructure per the proposal. Move the existing tests with their packages. Verify `make check` (Docker).
- [x] 1.4 Remove the flags from Settings, `.env.example`, the README, compose and gitops. Add the FEATURES entries. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, `red_first`, and `make docker` (then remove the image).

## 2. Boot validation reports every invalid key

Owns: `team-payment/internal/config/**`, and the box ticks.

- [x] 2.1 Write `TestValidateNamesEveryInvalidSetting` first and see it red (an empty `DATABASE_URL` plus an invalid `PAYOUT_HOLD_WINDOW`, and `MOCK_PAYMENTS=true` in production).
- [x] 2.2 Make `Settings.Validate` collect every error with `errors.Join` (GRPC, Postgres, payout hold keys, mock-payments guard) instead of returning the first. Verify `make check` (Docker).
- [x] 2.3 Add the `not-testable` FEATURES entry. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, `red_first`.

## 3. Use existing SDK packages instead of local copies

Behaviour, wire messages and env names stay the same; existing tests keep their assertions. Where a switch would change a client-visible message or a retry rule, leave that site and note it in the report.

Owns: `team-payment/**` except `third_party/**`.

- [x] 3.1 Use `pgx.WithTx` for hand-rolled transactions. Verify `make check`.
- [x] 3.2 Full-apply migration tests use `migrate.Up` (partial tests keep raw SQL). Verify `make check`.
- [x] 3.3 Use `grpcx.Status` + `ErrorTable` where the client message stays identical. Verify `make check`.

## 4. Adopt go-sdk v1.0.0

Follow `platform-core/packages/go-sdk/README.md` "Upgrading to v1" and "Outbox table migration" at tag `go-sdk/v1.0.0`. Behaviour, wire messages, gRPC codes and Kafka payloads stay the same unless a step below says otherwise; existing test assertions are not loosened. Any earlier unchecked "Use existing SDK packages" items are done here against v1.

Owns: `team-payment/**`, the team-payment blocks of `docker-compose*.yaml` and `platform-gitops/**` for team-payment.

- [x] 4.1 Sync `go-sdk/v1.0.0` into team-payment only and fix every breaking change (package renames, ErrorTable, constructors returning errors, ENV required, renamed duration env keys and removed `OUTBOX_MAX_ATTEMPTS`, outbox `PublishRecord`, consume Record and DLQ headers, .env.example, compose and gitops).
- [x] 4.2 Outbox table: forward-only migrations implementing `outbox.UpgradeSQL` (Prepare, Backfill, Switch in one transaction, Finish; `CONCURRENTLY` statements in their own migration files), a test that applies them to a table in the previous schema with rows and checks order, and a README deploy note (stop v0 relayers, purge published rows, requeue or delete v0 `failed` rows).
- [x] 4.3 Replace local copies with SDK packages where the SDK now has them (as in the earlier task list, against v1). Sites left local get a one-line reason in the report.
- [x] 4.4 Verify `make check`, `docker build`, red_first, openspec validate.
