# OpenSpec — Archive status (unarchived changes)

_Generated 2026-09-04. Rule in force: **do not force-archive incomplete changes.**_

All 15 active changes are **implementation-complete but verification-gated** — the
only unchecked tasks are environment-bound (running stack, CI/Docker Go build, live
cluster) plus the final `openspec archive` step. None are archivable yet.

| Change | Tasks | Primary blocker(s) |
|---|---|---|
| add-canary-deploys | 15/19 | C live-cluster (Argo Rollouts CRDs + staged rollback), spec-sync, archive |
| add-domain-transactional-outbox | 13/18 | B go build/test (Docker), A e2e + features-check, archive |
| add-flipt-infra | 9/14 | C flipt Ready on cluster, A features-check, archive |
| add-postgres-backup-cronjob | 9/13 | C ArgoCD sync + manual CronJob trigger, A features-check, archive |
| add-prometheus-infra | 7/10 | gateway OTEL wiring (blocked), optional edge instrument, archive |
| add-recommendation-contract | 8/11 | B buf lint/breaking in CI + importable, E archive after proto merged |
| add-staging-prod-overlays | 12/16 | A features-check, CI-owner image-tag bump flag, archive |
| build-als-training-job | 12/16 | A e2e (Spark→Qdrant) + features-check + end-to-end, archive |
| build-warehouse-writer | 14/20 | B go build/test, D compose (reserved), A e2e/features-check, archive |
| emit-tracking-events | 13/18 | D compose KAFKA_ENABLED, B go build, A features-check + flip flag, archive |
| migrate-jwt-rs256-jwks | 20/28 | D compose (reserved), A login e2e + RS256 assert, FEATURES.yaml, CI build, archive |
| replace-cockpit-mock-metrics | 8/12 | A run vs stack + flip flag + e2e + features-check, archive |
| serve-recommendations-teamai | 12/15 | E deps (add-recommendation-contract + build-als), RECS_ENABLED boot, archive |
| surface-recommendations | 14/20 | D compose UPSTREAM_RECOMMENDATION_ADDR, E dep (serve-recs), A e2e/features-check, archive |
| wire-openfeature | 12/21 | D compose (order+frontend FLIPT_ADDR), B go build, gitops FLIPT_ADDR, A stack + kill-switch e2e, archive |

## Blocker legend & how to clear

- **A. Running-stack e2e + `make -C platform-e2e features-check`** — the dominant
  blocker (13/15). Bring the full stack up (`docker compose -f docker-compose.services.yaml
  up --build`), run the suite, flip the named feature flags to `automated`.
- **B. `go build`/`go test ./...` in Docker/CI** — Go code lands, but Go isn't on this
  host; these are checked by the service Dockerfiles / CI, not locally.
- **C. Live-cluster verification** — canary (needs Argo Rollouts CRDs), prometheus OTEL,
  flipt Ready, postgres-backup CronJob, staging/prod overlays. **Gated on the kind
  cluster reaching 21/21** — currently parked on the pending `platform-gitops` push
  (RS256/JWKS wiring + team-ai Vault onboarding). Once ArgoCD syncs that, this bucket
  unblocks.
- **D. `docker-compose.services.yaml`** — several changes need env added there, but that
  file is orchestrator-reserved (single-writer). Batch these edits in one integration pass.
- **E. Cross-change dependencies** — `serve-recommendations-teamai` waits on
  `add-recommendation-contract` + `build-als-training-job`; `surface-recommendations`
  waits on `serve-recommendations-teamai`. Archive the chain in dependency order.
- **F. Final step** — every change's last task is `openspec archive <id>`; run it only
  after that change's A–E are all green.

## Note on `migrate-jwt-rs256-jwks`
The gitops half of this change (gateway `JWKS_URL`, identity `jwtShared` →
`JWT_PRIVATE_KEY`/`JWT_KID` from Vault `shared/jwt`) was corrected this session in
`platform-gitops` (commits on `main`, push pending). Its remaining tasks are the
stack-level login/RS256 e2e asserts — bucket A/C, unblocked once the cluster is 21/21.

## Verdict
**0/15 archivable today.** Clear bucket C first (finish the cluster push → 21/21),
then A (stack e2e) across the batch, then archive in dependency order (E). No change
should be archived before its tasks.md is fully checked.
