## Why

`platform-featurestore` can materialise features and build the governed `als_interactions@v1` dataset, and
`platform-recsys` refuses to train without it. In the cluster nothing runs the featurestore: the recsys CronJob mounts
an `emptyDir` at `/features`, so it never has a dataset and never trains. Only the docker compose profiles
(`featurestore-job`, `featurestore-dataset`) produce one.

## What Changes

- **platform-gitops:**
  - new raw-manifest Application `featurestore` (`platform/featurestore/`, sync wave 4) with a CronJob that runs
    `materialize`, then `parity`, then `dataset` in one pod, in that order, nightly before recsys trains;
  - a PVC `featurestore-data` the CronJob writes and the recsys CronJob mounts read-only at `/features` (replacing the
    `emptyDir`);
  - a PVC `analytics-data` for team-analytics, which now exports its Parquet files there (`PARQUET_EXPORT_PATH`);
    the featurestore CronJob mounts it read-only as its input;
  - a NetworkPolicy (egress: DNS and valkey only, no ingress), resources, a non-root, read-only-root-filesystem
    security context, and non-secret env only;
  - a freshness guard on the recsys CronJob: an init container fails the pod when the dataset is absent or older than
    `MAX_DATASET_AGE_MINUTES`, so recsys stays fail-closed for stale data as well as absent data.
- **platform-e2e:** static render scenarios `fcj_*` that parse the manifests and run the guard script.

No change to featurestore or recsys code, to the docker compose stack, or to any proto.

## Capabilities

### New Capabilities
- `featurestore-cluster-job`: how the featurestore runs in the cluster and hands its dataset to recsys.

### Modified Capabilities
- None.

## Non-goals

- Object storage / BigQuery datasets for real staging and prod (the existing follow-up).
- Staleness checking inside the recsys code (it is enforced at the Pod level here).
- Starting the kind cluster; cluster behaviour is verified only by the static render.

## Impact

- team-analytics' DuckDB file and exports move from an `emptyDir` to a PVC, so they now survive pod restarts.
- Both PVCs are `ReadWriteOnce`: the writer and its readers must share a node. Fine on the single-node local kind
  cluster; a multi-node cluster needs RWX storage or object storage (follow-up).
- Existing mismatch, not changed here: `recsys-config` uses `REDIS_HOST: redis`, while the cluster Service is `valkey`.
