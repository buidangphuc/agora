## 1. Code track (platform-gitops)

- [x] 1.1 minio-init, bucket-init, fetch-mc: pinned bitnami mc image, `IfNotPresent`, `MC_CONFIG_DIR` on minio-init, fetch-mc copy path. Verify: "No third-party image floats on latest".
- [x] 1.2 `platform-gitops/FEATURES.yaml` entry.

## 2. e2e / static-test track (platform-e2e)

- [x] 2.1 Scenario in `ops/lcw_local_cluster_image_wiring.feature` + steps. Verify: pytest on lcw.

## Evidence

- Static tests: lcw (new scenario fails on the old bucket-init manifest, passes after), roc, fcj, modelserve, pear_authz gitops — 41 passed; black/ruff on the steps file; openspec validate --strict; features.py --strict; spec_sync; repo_doctor.
- mc binary of the pinned image is a static aarch64 Go build (`file`), so fetch-mc can copy it into postgres:16-alpine.
- Live: minio-init on the cluster after gitops-push (below, in the archive commit if run).
