# [W1-T5] Finish wire-openfeature §3 — gitops values.env (platform-gitops)

## Role
SRE

## Objective
Complete the one genuinely un-started infra edit of the `wire-openfeature` change: add the Flipt env to the ArgoCD Helm values for team-order and team-frontend, so the kill-switch flag resolves in-cluster.

## Write-set (EXCLUSIVE)
- platform-gitops/argocd/apps/team-order.yaml — Helm `values.env`: FLIPT_ADDR, FEATURE_FLAGS_ENABLED (edit)
- platform-gitops/argocd/apps/team-frontend.yaml — Helm `values.env`: FLIPT_ADDR, FEATURE_FLAGS_ENABLED (edit)

## Read-only dependencies
- openspec/changes/wire-openfeature/tasks.md §3 (the exact keys/values)
- Existing env entries in these two files (mirror their structure/indentation)
- Compose values for reference: team-order `FLIPT_ADDR=flipt:9000` (gRPC), team-frontend `FLIPT_ADDR=flipt:8080` (REST); both `FEATURE_FLAGS_ENABLED=true`.

## Acceptance criteria
- [ ] Both files carry FLIPT_ADDR + FEATURE_FLAGS_ENABLED under the existing env block, matching each service's addr (order=9000 gRPC, frontend=8080 REST)
- [ ] YAML valid; Helm value path matches the chart's `env` shape used by sibling entries
- [ ] No unrelated keys changed

## Review (different agent)
SRE rubric. Config-only; verify addr/port correctness and that it matches the chart schema.

## Verify
Yaml lint (yamllint or `python -c 'import yaml,sys;yaml.safe_load(open(f))'`) on both files; if a cluster is reachable, `kubectl apply --dry-run=client -f` (else skip).

## Out of scope
- Do NOT edit docker-compose.services.yaml (integration wave batch).
- Do NOT touch any service repo, the chart templates, or other argocd apps.
- Do NOT sync/deploy — this is a code edit only; ArgoCD sync is a separate authorized step.
