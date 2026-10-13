Code track only: there is no user-facing behaviour change. Every scenario names its test and gets a `not-testable` entry in `team-order/FEATURES.yaml`. The orchestrator also runs the automated order e2e features later, when the stack is available (not in this change's gate).

## 1. Migrate team-order (single packet)

Owns: `team-order/**`, the team-order service block in `docker-compose.services.yaml`, `platform-gitops/argocd/apps/team-order.yaml`, `platform-gitops/envs/services/team-order.yaml`, and the box ticks.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.2.0 team-order` (it now runs `buf generate` itself). Verify `go build ./...`.
- [x] 1.2 Write the new spec tests first and see them red. This includes migration `0008` and its test.
- [x] 1.3 Restructure per the proposal table, replacing the outbox, consumer, dedupe, flags and upstream internals with SDK packages. Move the existing tests with their packages; their assertions are unchanged. Verify `make check` (Docker).
- [x] 1.4 Remove the flags from Settings, `.env.example`, the README, compose and gitops (gitops `env:` keeps the remaining keys). Add the FEATURES entries. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, the `red_first` script, and `make docker` (then remove the image).

## 2. Review fixes on go-sdk v0.2.1

Owns: same as `## 1`.

- [x] 2.1 Re-sync to `go-sdk/v0.2.1`. Replace `scopedConn` with `upstream.WithServicePrincipal` where it now suffices, and use the idempotent `EnqueueTx`.
- [x] 2.2 Restore the voucher flow per the proposal (transition and outbox in one transaction, then the voucher commit, then mark processed). Red first: `TestVoucherCommitFailureKeepsOrderPaidAndRetriesCommit`. Remove `TestVoucherCommitFailureRollsBackAndRedeliveryCommitsOnce`, which encoded the rejected behaviour.
- [x] 2.3 `0008` keeps a unique index on `event_id`. Red first: `TestMigration0008KeepsPreviousReleaseInsertWorking`. Remove the rollout warning from the README.
- [x] 2.4 Add FEATURES entries for the two new scenarios. Verify `make check`, `features-check`, `spec_sync --strict` and `red_first`.

## 3. Use existing SDK packages instead of local copies

Behaviour, wire messages and env names stay the same; existing tests keep their assertions. Where a switch would change a client-visible message or a retry rule, leave that site and note it in the report.

Owns: `team-order/**` except `third_party/**`.

- [ ] 3.1 Use `pgx.WithTx` for hand-rolled transactions. Verify `make check`.
- [ ] 3.2 Full-apply migration tests use `migrate.Up` (partial from/to tests keep raw SQL). Verify `make check`.
- [ ] 3.3 `releaseReservationWithRetry` uses `resilience.Do(ctx, fn, resilience.Retry(...))` if the retried error set stays the same. Verify `make check`.
- [ ] 3.4 Use `grpcx.Status` + `ErrorTable` where the client message stays identical. Verify `make check`.

## 4. Adopt go-sdk v1.0.0

Follow `platform-core/packages/go-sdk/README.md` "Upgrading to v1" and "Outbox table migration" at tag `go-sdk/v1.0.0`. Behaviour, wire messages, gRPC codes and Kafka payloads stay the same unless a step below says otherwise; existing test assertions are not loosened. Any earlier unchecked "Use existing SDK packages" items are done here against v1.

Owns: `team-order/**`, the team-order blocks of `docker-compose*.yaml` and `platform-gitops/**` for team-order.

- [ ] 4.1 Sync `go-sdk/v1.0.0` into team-order only and fix every breaking change (package renames, ErrorTable, constructors returning errors, ENV required, renamed duration env keys and removed `OUTBOX_MAX_ATTEMPTS`, outbox `PublishRecord`, consume Record and DLQ headers, .env.example, compose and gitops).
- [ ] 4.2 Outbox table: forward-only migrations implementing `outbox.UpgradeSQL` (Prepare, Backfill, Switch in one transaction, Finish; `CONCURRENTLY` statements in their own migration files), a test that applies them to a table in the previous schema with rows and checks order, and a README deploy note (stop v0 relayers, purge published rows, requeue or delete v0 `failed` rows).
- [ ] 4.3 Replace local copies with SDK packages where the SDK now has them (as in the earlier task list, against v1). Sites left local get a one-line reason in the report.
- [ ] 4.4 Verify `make check`, `docker build`, red_first, openspec validate.
