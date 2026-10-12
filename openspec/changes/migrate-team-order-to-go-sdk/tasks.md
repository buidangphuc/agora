Code track only: there is no user-facing behaviour change. Every scenario names its test and gets a `not-testable` entry in `team-order/FEATURES.yaml`. The orchestrator also runs the automated order e2e features later, when the stack is available (not in this change's gate).

## 1. Migrate team-order (single packet)

Owns: `team-order/**`, the team-order service block in `docker-compose.services.yaml`, `platform-gitops/argocd/apps/team-order.yaml`, `platform-gitops/envs/services/team-order.yaml`, and the box ticks.

- [x] 1.1 Run `platform-core/tools/sync-go-sdk.sh go-sdk/v0.2.0 team-order` (it now runs `buf generate` itself). Verify `go build ./...`.
- [x] 1.2 Write the new spec tests first and see them red. This includes migration `0008` and its test.
- [x] 1.3 Restructure per the proposal table, replacing the outbox, consumer, dedupe, flags and upstream internals with SDK packages. Move the existing tests with their packages; their assertions are unchanged. Verify `make check` (Docker).
- [x] 1.4 Remove the flags from Settings, `.env.example`, the README, compose and gitops (gitops `env:` keeps the remaining keys). Add the FEATURES entries. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, the `red_first` script, and `make docker` (then remove the image).

## 2. Review fixes on go-sdk v0.2.1

Owns: same as `## 1`.

- [ ] 2.1 Re-sync to `go-sdk/v0.2.1`. Replace `scopedConn` with `upstream.WithServicePrincipal` where it now suffices, and use the idempotent `EnqueueTx`.
- [ ] 2.2 Restore the voucher flow per the proposal (transition and outbox in one transaction, then the voucher commit, then mark processed). Red first: `TestVoucherCommitFailureKeepsOrderPaidAndRetriesCommit`. Remove `TestVoucherCommitFailureRollsBackAndRedeliveryCommitsOnce`, which encoded the rejected behaviour.
- [ ] 2.3 `0008` keeps a unique index on `event_id`. Red first: `TestMigration0008KeepsPreviousReleaseInsertWorking`. Remove the rollout warning from the README.
- [ ] 2.4 Add FEATURES entries for the two new scenarios. Verify `make check`, `features-check`, `spec_sync --strict` and `red_first`.
