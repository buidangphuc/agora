---
name: release
description: Release a built and reviewed change through the local GitOps plane (ASDLC v2 phase 8): gates green, tag images with the commit sha, bump the staging overlay, ArgoCD sync, smoke-staging, canary watch, human approval for prod, rollback path. Use after the Feature gate when the user says "release", "deploy to staging", "ship it", or after initiative-build converges. Needs the kind stack up; never runs against a stack that is not approved for this session.
---

Read `docs/asdlc/release.md` first (details, commands, failure table). The deploy plane is the local kind cluster + Gitea + ArgoCD. Never `kubectl apply` or `helm upgrade` by hand; change `platform-gitops` and let ArgoCD pull.

## Steps (stop at the first red; each line is `step -> check`)
1. Gates green -> Feature gate decision table has no veto; `make check` and e2e per the repo's CI config are green; working tree committed (tags are commit shas). Say which checks were run.
2. Confirm the stack with the human -> `make -C deploy status`; do not start/stop the cluster or compose stacks unless the user approves.
3. Build + tag -> images built from the commit; `make -C deploy release-tag DRY_RUN=1` first, then `make -C deploy release-tag` (TAG defaults to `git rev-parse --short=12 HEAD`, RELEASE_ENV=staging). Record the previous tag from `git diff platform-gitops/envs/staging/values.yaml`: that is the rollback target.
4. Publish -> `make -C deploy gitops-push` (local Gitea only; never commits to the agora repo).
5. Sync -> `make -C deploy status-staging`: apps `team-gateway-staging`, `team-frontend-staging` Synced/Healthy; if not, hand to `incident-triager`.
6. Smoke -> `make -C deploy smoke-staging` must be green.
7. Canary watch -> `kubectl argo rollouts get rollout team-gateway -n marketplace-staging -w` with traffic flowing (`docs/runbooks/rollback.md` step 4); expect AnalysisRun Successful and Healthy. A failed AnalysisRun auto-rolls back: stop, report the failing metric, run `incident-triager`.
8. Human approval for prod -> present: tag, previous tag, smoke result, canary result, Feature-gate table. WAIT for an explicit yes. There is no prod cluster locally: approval means "promote the same tag to `envs/prod/values.yaml` in a PR", which a human merges.
9. Rollback path (always stated up front) -> `scripts/rollback.sh <svc> --env staging --abort` (in-flight canary) or `--to <previous-tag>` then `make -C deploy gitops-push` (durable). Details: `docs/runbooks/rollback.md`.
10. Report -> files changed, checks run with results, tags (new/previous), open issues. Operate (phase 9) alerts: `docs/runbooks/alerts.md`.

Release gate exit criterion (plan phase 8): the rollback has been drilled live at least once (`docs/runbooks/rollback.md` Drill log). If the log is empty, say the gate is not met.
