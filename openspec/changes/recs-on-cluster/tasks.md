## 1. platform-gitops and deploy

- [ ] 1.1 Add `platform/infra/qdrant.yaml` (StatefulSet, PVC template, Service, NetworkPolicy). Verify: `roc_*` scenarios
  "Qdrant is pinned to the compose version", "Qdrant is persistent and hardened", "Qdrant accepts only its recommendation clients".
- [ ] 1.2 team-ai recs env in `envs/services/team-ai.yaml` and `argocd/apps/team-ai.yaml`. Verify: "team-ai points at the
  cluster stores", "team-ai keeps its ingress allow-list".
- [ ] 1.3 Add `platform/recsys/nearline.yaml` (Deployment, NetworkPolicy) and the recsys CronJob NetworkPolicy. Verify:
  "The nearline consumer is wired to the cluster brokers", "The recsys job and nearline egress is limited to their stores".
- [ ] 1.4 Image push list and local values. Verify: "The recommendation images are on the push list".
- [ ] 1.5 ArgoCD registration check, README layout update; `python3 scripts/check_identity_seed.py` passes. Verify:
  "Qdrant and nearline are synced by an Application".

## 2. platform-e2e

- [ ] 2.1 `ops/roc_recs_on_cluster.feature` + steps + binding (static, no stack) and FEATURES.yaml entries in
  platform-gitops. Verify: `pytest -k roc` green.
