## Context

Static, GitOps-only change. The cluster is a single-node kind cluster; raw manifests under `platform/<dir>` are synced by
ArgoCD Applications (`infra-shared` and `platform-modelserve` sync `platform/infra`, `platform-recsys` syncs
`platform/recsys`, both recursive), so new files in those directories are registered without a new Application.

## Decisions

- **Qdrant as a StatefulSet** with a `volumeClaimTemplates` PVC (2Gi, RWO), not a Deployment: stable storage identity.
  Image `qdrant/qdrant:v1.19.0`, the compose tag (the client rejects a server more than one minor version apart).
  Runs as UID/GID 1000, non-root, read-only root filesystem, with `/qdrant/storage` (PVC) and `/qdrant/snapshots` and
  `/tmp` (emptyDir) writable. Probes use HTTP `/readyz` and `/healthz` (the image has no curl).
- **Qdrant NetworkPolicy** (ingress only): sources `team-ai` (reader) and `platform-recsys` (writer) on 6333/6334.
- **Recsys NetworkPolicy** (Ingress+Egress, no ingress rules): egress DNS, `qdrant` 6333/6334, `valkey` 6379.
  **Nearline NetworkPolicy**: egress DNS, `redpanda` 9092, `valkey` 6379. Same DNS rule shape as the featurestore policy.
- **Nearline as a Deployment** (replicas 1: one consumer group member keeps the CTR counters single-writer), same image
  as the recsys batch job, `command` overriding the entrypoint. Env only, no secrets (valkey and redpanda are open).
  Redis DB stays the default 0, which is the DB team-ai reads (`RECS_NEARLINE_REDIS_URL=.../0`).
- **team-ai env** follows compose, with the cluster Redis host `valkey`: `RECS_ENABLED=true`, `RECS_BACKEND=qdrant`,
  `RECS_QDRANT_URL=http://qdrant:6333`, `RECS_QDRANT_COLLECTION=item_als_vectors`, cache prefix/schema/TTL,
  `RECS_FEATURESTORE_REDIS_URL=redis://valkey:6379/2`, `RECS_NEARLINE_REDIS_URL=redis://valkey:6379/0`,
  `RECS_RETRIEVE_TIMEOUT_MS=15`, plus `REDIS_ENABLED=true`, `REDIS_HOST=valkey`, `REDIS_PORT=6379` for the precomputed
  cache. The same env goes in the baseline and in the inline local block, which the local cluster actually renders.
- **Images**: `push-images.sh` gets `platform-featurestore` and `platform-recsys` (the nearline pod reuses the latter), and
  team-ai's source becomes `team-ai:local` (compose builds it with the `recs` extra).

## Risks

- Qdrant with a read-only root filesystem and non-root UID is verified only live (the cluster stays stopped).
- Spark `local[*]` inside the recsys pod talks to itself; the egress policy assumes the CNI allows pod-local traffic.
- RWO PVCs assume one node.
