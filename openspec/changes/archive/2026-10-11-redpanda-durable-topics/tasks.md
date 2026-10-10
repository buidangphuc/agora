## 1. Code track (platform-gitops)

- [x] 1.1 `infra/redpanda.yaml`: PVC `redpanda-data` (RWO, 2Gi), volume mounted at `/var/lib/redpanda/data`, `securityContext.fsGroup: 101`, `strategy: { type: Recreate }`; header comment. Verify: "The broker data dir is on a claim", "A single-writer volume is replaced, not surged".
- [x] 1.3 `rollingUpdate: null` on redpanda, team-analytics, tei-embed, tei-rerank. Verify: YAML parse; `kubectl kustomize platform/team-analytics` keeps the null.
- [x] 1.2 `platform-gitops/FEATURES.yaml`: entry for the new scenario.

## 2. e2e / static-test track (platform-e2e)

- [x] 2.1 Scenario "The broker data dir is on a claim" in `ops/lcw_local_cluster_image_wiring.feature` + steps. Verify: pytest on lcw.

## Evidence

- Static tests: lcw 6/6, roc, fcj, modelserve, pear_authz gitops (25 passed); black/ruff on the steps file; openspec change and specs validate --strict; features.py --strict; spec_sync 1/1; repo_doctor 2 pre-existing warnings.
- Not verified (cluster stopped; gitops-push is run by the user): ArgoCD sync of the claim and the strategy switch, topics surviving a node restart.
