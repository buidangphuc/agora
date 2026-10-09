## 1. platform-gitops

- [ ] 1.1 Add `platform/featurestore/` (ConfigMap, PVC `featurestore-data`, CronJob, NetworkPolicy) and
  `argocd/apps/featurestore.yaml`. Verify: `fcj_*` render scenarios "The three steps run in order", "The featurestore is
  scheduled before recsys", "The job is hardened and isolated".
- [ ] 1.2 Move team-analytics `/data` to PVC `analytics-data` and set `PARQUET_EXPORT_PATH`. Verify: scenario "The
  analytics export reaches the featurestore".
- [ ] 1.3 Recsys CronJob: mount `featurestore-data` read-only, add the freshness guard init container and
  `MAX_DATASET_AGE_MINUTES`. Verify: scenarios "The dataset volume is shared", "Absent/Stale dataset blocks training",
  "Fresh dataset passes the guard".
- [ ] 1.4 README layout/known-gaps update; `python3 scripts/check_identity_seed.py` passes.

## 2. platform-e2e

- [ ] 2.1 `ops/fcj_featurestore_cluster.feature` + steps + binding (static, no stack) and FEATURES.yaml entries in
  platform-gitops. Verify: `pytest -k fcj` green.
