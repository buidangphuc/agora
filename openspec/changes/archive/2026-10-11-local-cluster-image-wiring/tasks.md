## 1. Code track (deploy, platform-gitops)

- [x] 1.1 `push-images.sh`: compose-derived mapping for every values key, agora-/bare resolution, content tag, writes values and kustomizations. Verify: "The push list names real compose images".
- [x] 1.2 Raw-manifest apps: tagless images, `kustomization.yaml`, Applications drop `directory`, indexer `command`. Verify: "No recsys manifest hardcodes a local tag", "Kustomize tags follow the local values"; `kubectl kustomize` renders.
- [x] 1.3 `gitea/publish-gitops.sh`; `gitops-push` and `bootstrap-gitea.sh` use it. Verify: "gitops-push does not target origin"; `bash -n`.
- [x] 1.4 README (platform-gitops, deploy) describe the flow.

## 2. e2e / static-test track (platform-e2e)

- [x] 2.1 `ops/lcw_local_cluster_image_wiring.feature` + steps + binding; roc image/sync scenarios updated; FEATURES.yaml entries (platform-gitops). Verify: pytest on lcw, roc, fcj, gitops_render.

## Evidence

- Static tests: lcw 4/4, roc 9/9, fcj, modelserve gitops, pear_authz (31 passed with pyyaml added ad hoc).
- `kubectl kustomize` renders the four raw-manifest apps; `bash -n` on the three scripts; openspec change and specs validate --strict; features.py --strict and repo_doctor pass.
- Not verified (cluster stays stopped): ArgoCD kustomize sync of the four apps, the actual Gitea push, shellcheck (not installed).
