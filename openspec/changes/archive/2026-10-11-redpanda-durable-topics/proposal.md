## Why

The recs-on-cluster item 5 run (2026-10-11) restarted the kind node and found Redpanda with no topics at all. The
broker in `platform-gitops/platform/infra/redpanda.yaml` has no volume, so its data dir (topics, records, consumer
group offsets) lives in the container layer and is lost whenever the pod is recreated. Topics are created only by the
`redpanda-init` ArgoCD PostSync hook, which does not run again on a node or pod restart. Producers that do not
auto-create topics (team-gateway's `/api/track`) then time out after 60s and every analytics event is dropped.

## What Changes

- **platform-gitops:** `infra/redpanda.yaml` adds a `ReadWriteOnce` claim `redpanda-data` mounted at
  `/var/lib/redpanda/data`, `fsGroup: 101` (the image's `redpanda` user) and the `Recreate` strategy (one broker owns
  the volume). Topics, records and committed offsets now survive a pod or node restart. The PostSync hook stays: it
  still provisions a fresh cluster and is idempotent.
- **platform-gitops (sync safety):** the Deployments switched to `Recreate` after their first apply (`redpanda`,
  `team-analytics`, `tei-embed`, `tei-rerank`) set `rollingUpdate: null`, so ArgoCD's client-side apply removes the
  server-defaulted `rollingUpdate` instead of failing with "may not be specified when strategy type is Recreate".
- **platform-e2e:** a static scenario in `ops/lcw_local_cluster_image_wiring.feature` asserts the broker data dir is on
  a claim; the existing RWO scenario covers the strategy.
- **FEATURES.yaml (platform-gitops):** one entry.

No service code, compose file, or proto changes.

## Capabilities

### New Capabilities

### Modified Capabilities
- `local-cluster-image-wiring`: adds "The broker keeps its topics across a restart".

## Impact

- Existing clusters: the first sync replaces the broker pod with an empty volume; the PostSync hook of that same sync
  recreates the topics. Data already in the old pod is lost once (it was already lost on any restart).
- Disk: one 2Gi local-path claim on the kind node.
