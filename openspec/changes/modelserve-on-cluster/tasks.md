## 1. Code track (platform-gitops, deploy)

- [x] 1.1 Move `platform/infra/modelserve.yaml` → `platform/modelserve/modelserve.yaml` + `kustomization.yaml`; router image `localhost:5001/platform-modelserve`; tei-embed/tei-rerank image `localhost:5001/tei-fake` with `args: ["--host","0.0.0.0","--port","<port>"]`, `/healthz` probes, no models-cache volume/PVC, default strategy. Verify: "The modelserve images come from the local registry"; `kubectl kustomize platform/modelserve`.
- [x] 1.2 `argocd/apps/platform-modelserve.yaml` → `path: platform/modelserve`, no `directory`. Verify: "No two Applications sync the same path".
- [x] 1.3 `deploy/images/push-images.sh` MAP + kustomization writer; `envs/local/values.yaml` keys. Verify: lcw "The push list names real compose images", "Kustomize tags follow the local values".
- [x] 1.4 `platform-gitops/FEATURES.yaml` entries; README notes (platform-gitops, deploy).

## 2. e2e / static-test track (platform-e2e)

- [x] 2.1 Two scenarios in `ops/lcw_local_cluster_image_wiring.feature` + steps; existing lcw / modelserve / roc static steps follow the path move. Verify: pytest on lcw, roc, fcj, modelserve, pear_authz gitops.

## Evidence

- Static tests: lcw (incl. modelserve in RAW_APPS), roc, fcj, modelserve, pear_authz gitops — 40 passed; black/ruff on changed steps; openspec change validates --strict; features.py --strict; spec_sync; repo_doctor 2 pre-existing warnings; `bash -n` push-images.sh; `kubectl kustomize platform/modelserve` renders.
- Review follow-ups applied: router `VLLM_URL` → tei-fake (no vLLM on the local cluster, as in compose); `MODEL_VERSION` `tei-fake-v1` as in compose.
- Not verified yet: image push and ArgoCD sync on the cluster (run after commit).
