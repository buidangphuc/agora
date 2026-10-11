## 1. platform-gitops

- [x] 1.1 Add `platform/featurestore/` (ConfigMap, PVC `featurestore-data`, CronJob, NetworkPolicy) and
  `argocd/apps/featurestore.yaml`. Verify: `fcj_*` render scenarios "The three steps run in order", "The featurestore is
  scheduled before recsys", "The job is hardened and isolated".
- [x] 1.2 Move team-analytics `/data` to PVC `analytics-data` and set `PARQUET_EXPORT_PATH`. Verify: scenario "The
  analytics export reaches the featurestore".
- [x] 1.3 Recsys CronJob: mount `featurestore-data` read-only, add the freshness guard init container and
  `MAX_DATASET_AGE_MINUTES`. Verify: scenarios "The dataset volume is shared", "Absent/Stale dataset blocks training",
  "Fresh dataset passes the guard".
- [x] 1.4 README layout/known-gaps update; `python3 scripts/check_identity_seed.py` passes.

## 2. platform-e2e

- [x] 2.1 `ops/fcj_featurestore_cluster.feature` + steps + binding (static, no stack) and FEATURES.yaml entries in
  platform-gitops. Verify: `pytest -k fcj` green.

## Evidence (2026-10-09)

- Commits on port/fcj, merged: 148ee95f (spec), 2422f1ed, 6946b2c8, 6f6630e7, 3876deca. Integrator fix 908e2e76: recsys `REDIS_HOST` changed from `redis` to `valkey`, because the cluster has no `redis` Service.
- Checks:
  - `test_fcj_featurestore_cluster.py` 8/8 (static manifest assertions; the guard script runs under `sh` against temp dirs);
  - `test_pear_authz_gitops.py` 2/2;
  - spec_sync 8/8;
  - `features.py --strict`;
  - repo_doctor;
  - `scripts/check_identity_seed.py`.
- Not verified, because the cluster stays stopped: init container ordering in a live pod, PVC binding (RWO, which assumes a single node), CNI enforcement of the NetworkPolicy, and busybox `find -mmin`.
- Gaps found, out of scope here:
  - The cluster has no Qdrant, and team-ai's cluster values set no recs backend or stores. Serving recs in the cluster needs its own change.
  - No build or push list publishes the `platform-featurestore` and `platform-recsys` images to `localhost:5001`.
  - modelserve points at `redis.marketplace.svc`, which does not exist in the cluster.
