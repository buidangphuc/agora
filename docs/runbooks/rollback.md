# Runbook: rollback and canary-failure drill

Scope: the kind staging overlay (`marketplace-staging`). Only `team-gateway` runs as an Argo Rollout (canary, AnalysisTemplate `team-gateway-canary`); every other service is a plain Deployment.

## One-line rollback
```bash
scripts/rollback.sh team-gateway --env staging --abort            # stop an in-flight canary (stable keeps serving, durable)
scripts/rollback.sh team-gateway --env staging                    # undo to previous revision (immediate, NOT durable)
scripts/rollback.sh team-gateway --env staging --to <good-tag>    # durable: edits images.team-gateway in git
make -C deploy gitops-push                                        # publish the durable one; ArgoCD rolls back
```
Why two paths: ArgoCD `selfHeal` re-applies git, so `undo` alone is rolled forward again within minutes. Use `undo` to stop the bleeding, then the `--to <good-tag>` path so git agrees. `abort` lives in Rollout status, which ArgoCD does not revert. Add `--dry-run` to print the commands. Needs the plugin: `brew install argoproj/tap/kubectl-argo-rollouts`.

Plain Deployment services have no canary: `scripts/rollback.sh <svc> --env staging --to <good-tag>` then `gitops-push`. The previous good tag is in `git log -p platform-gitops/envs/staging/values.yaml`.

## Canary-failure drill (human-approved live run)
Goal: prove a failing canary is rolled back automatically. Prereqs: kind stack up, staging overlay Healthy (`make -C deploy status-staging`), `otel-collector` and `prometheus` Healthy in `marketplace` (the AnalysisTemplate reads `marketplace_rpc_client_duration_milliseconds_*` through them).

1. Baseline: `kubectl argo rollouts get rollout team-gateway -n marketplace-staging` shows `Healthy`, note the stable revision/tag.
2. Make the analysis fail on purpose, in `platform-gitops/envs/staging/kind/team-gateway.yaml`, add:
   ```yaml
   rollout:
     analysis:
       latencyP95Ms: "0.001"   # p95 can never be below 0.001 ms -> latency-p95 metric fails
   ```
   (alternative: `minRps: "100000"` fails `rps-sanity`.) This changes only the AnalysisTemplate, not the pod template, so it does not roll anything yet.
3. Trigger a canary with a new tag (retag the same image under a new fake sha; no code change needed): `TAG=d0d0d0d0d0d0 ALLOW_DIRTY=1 make -C deploy release-tag` then `make -C deploy gitops-push`.
4. Send traffic so metrics exist (separate terminal, keep running): `while true; do curl -s -o /dev/null http://api.staging.localtest.me/healthz; sleep 0.2; done`.
5. Watch: `kubectl argo rollouts get rollout team-gateway -n marketplace-staging -w`. Expected: step 25% -> pause 60s -> AnalysisRun `team-gateway-<hash>-...` runs `error-rate`, `latency-p95`, `rps-sanity` every 30s; `latency-p95` breaches `failureLimit: 1` after 2 measurements (~1-2 min) -> AnalysisRun `Failed` -> Rollout `Degraded`/aborted, canary scaled to 0, 100% of traffic on the stable ReplicaSet.
   Evidence commands: `kubectl -n marketplace-staging get analysisrun`, `kubectl -n marketplace-staging describe analysisrun <name>`.
6. Verify service is still up: `curl -s -o /dev/null -w '%{http_code}\n' http://api.staging.localtest.me/healthz` -> 200, and `smoke-staging` still green.
7. Clean up (important, otherwise the bad template stays in git): remove the `rollout:` block from step 2, set `images.team-gateway` back to the stable tag from step 1 (`scripts/rollback.sh team-gateway --env staging --to <stable-tag>`), `make -C deploy gitops-push`, confirm `Healthy`.
8. Also exercise the manual path once: repeat steps 3-4, and while the canary is paused run `scripts/rollback.sh team-gateway --env staging --abort`; expect immediate return to stable.
9. Fill in the Drill log below.

Known limitation: with `ENV=staging` + OTEL, metrics only exist if the gateway receives real gRPC traffic to upstreams; `/healthz` alone may not produce `marketplace_rpc_client_*` series. If `rps-sanity` fails instead of `latency-p95`, send a request that hits an upstream (e.g. `curl http://api.staging.localtest.me/api/...` a public listing/search route). Any failed metric proves the rollback path; record which one.

## Drill log
<!-- Fill during the live run: date, who, tag stable -> tag canary, failing metric, time from canary start to Failed, final state, anything surprising. -->

| Date | Operator | Drill | Stable tag | Canary tag | Failing metric | Time to rollback | Result | Notes |
|---|---|---|---|---|---|---|---|---|
| 2026-10-11 | Claude (user-approved) | Auto canary rollback | 01a53993082a | d0d0d0d0d0d0 | error-rate 0.136/0.133 (real upstream errors) + latency-p95 21.3/19.0 ms vs planted 0.001 ms; rps-sanity ok | ~100 s from canary pod start to RolloutAborted | Aborted at step 0/7, stable kept serving, /healthz 200 | First attempt never started: gitops-push skipped the values file (same size, rsync quick-check) — fixed with `rsync --checksum` |
| 2026-10-11 | Claude (user-approved) | Durable manual rollback | d0d0d0d0d0d0 (aborted) | 01a53993082a | — | ~2 min (gitops-push + ArgoCD sync) | `rollback.sh team-gateway --env staging --to 01a53993082a` + gitops-push → Rollout Healthy on 01a53993082a | `--abort` path not exercised separately (the auto-abort already held the canary) |
