## 1. platform-gitops and deploy

- [x] 1.1 Add `platform/infra/qdrant.yaml` (StatefulSet, PVC template, Service, NetworkPolicy). Verify: `roc_*` scenarios
  "Qdrant is pinned to the compose version", "Qdrant is persistent and hardened", "Qdrant accepts only its recommendation clients".
- [x] 1.2 team-ai recs env in `envs/services/team-ai.yaml` and `argocd/apps/team-ai.yaml`. Verify: "team-ai points at the
  cluster stores", "team-ai keeps its ingress allow-list".
- [x] 1.3 Add `platform/recsys/nearline.yaml` (Deployment, NetworkPolicy) and the recsys CronJob NetworkPolicy. Verify:
  "The nearline consumer is wired to the cluster brokers", "The recsys job and nearline egress is limited to their stores".
- [x] 1.4 Image push list and local values. Verify: "The recommendation images are on the push list".
- [x] 1.5 ArgoCD registration check, README layout update; `python3 scripts/check_identity_seed.py` passes. Verify:
  "Qdrant and nearline are synced by an Application".

## 2. platform-e2e

- [x] 2.1 `ops/roc_recs_on_cluster.feature` + steps + binding (static, no stack) and FEATURES.yaml entries in
  platform-gitops. Verify: `pytest -k roc` green.

## Evidence (2026-10-10)

- Static render tests:
  - test_roc_recs_on_cluster 9/9;
  - the existing fcj 8/8, pear_authz_gitops 2/2 and the modelserve gitops scenarios stay green (20 passed in the
    final run).
- spec_sync --strict, features.py --strict, check_identity_seed.py and repo_doctor pass.
- Integrator fix: the inline local team-ai allow-list (argocd/apps/team-ai.yaml) now admits team-search-indexer, as
  envs/services/team-ai.yaml and the deploy-runtime spec do.
- Not verified, because the kind cluster stays stopped:
  - Qdrant starting non-root with a read-only root filesystem, and its /readyz probe;
  - PVC binding;
  - the CNI enforcing the new egress policies;
  - the nearline consumer reaching redpanda and valkey;
  - team-ai Recommend being non-empty after a featurestore and recsys run.
