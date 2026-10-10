## Why

The local GitOps cluster can train recommendations (featurestore and recsys CronJobs) but cannot serve them. Archived
change `featurestore-cluster-job` recorded the gaps: the cluster has no Qdrant, team-ai's cluster values enable no
recommendations backend or stores, no build or push list publishes the `platform-featurestore` and `platform-recsys`
images, the nearline CTR consumer has no Deployment, and the batch pods have no NetworkPolicy to reach their stores.
Only docker compose serves recommendations today.

## What Changes

- **platform-gitops:**
  - `platform/infra/qdrant.yaml`: Qdrant as a StatefulSet with a PVC, a ClusterIP Service `qdrant` (6333, 6334), resources,
    a hardened security context, and a NetworkPolicy; image tag pinned to the compose version (`v1.19.0`);
  - team-ai values (`envs/services/team-ai.yaml` and the inline block of `argocd/apps/team-ai.yaml`): recommendations
    enabled with the qdrant backend, the cluster Qdrant and valkey (`valkey`) endpoints, featurestore online features
    on Redis DB 2 and nearline signals on DB 0, and the Redis precomputed cache;
  - `platform/recsys/nearline.yaml`: a `platform-recsys-nearline` Deployment (the recsys image, entrypoint
    `python -m recsys.nearline`) consuming `analytics.events` into valkey DB 0;
  - NetworkPolicies for the recsys CronJob (egress DNS, qdrant, valkey) and nearline (egress DNS, redpanda, valkey);
  - `envs/local/values.yaml`: image key for `platform-featurestore`.
- **deploy:** `images/push-images.sh` publishes `platform-featurestore` and `platform-recsys` to `localhost:5001`, and
  publishes team-ai from the image built with the `recs` extra (`team-ai:local`).
- **platform-e2e:** static render scenarios `roc_*` (manifest parsing and `helm template`).

No change to service code, docker compose, or any proto. The existing team-ai ingress allow-list is unchanged.

## Capabilities

### New Capabilities
- `recs-on-cluster`: how the cluster stores, serves and keeps fresh the trained recommendations.

### Modified Capabilities
- None.

## Non-goals

- Starting the kind cluster; behaviour is verified only by the static render.
- Qdrant clustering, snapshots/backup, API-key auth (valkey and Qdrant run open inside the namespace, like compose).
- Prod/staging Qdrant (the ApplicationSet has no real cluster); the team-ai baseline carries the same env for parity.

## Impact

- `REDIS_ENABLED` becomes `true` for team-ai in the cluster (the precomputed cache needs it); `RATE_LIMIT_BACKEND` is untouched.
- team-ai pulls `team-ai:local` instead of `ai-platform:local` when pushed by `push-images.sh`: only the former is built
  with `UV_EXTRAS=recs` (qdrant-client), which `RECS_BACKEND=qdrant` needs.
- Known drift, not changed: the inline team-ai block in `argocd/apps/team-ai.yaml` allows only `team-gateway` and
  `prometheus`, while `envs/services/team-ai.yaml` also allows `team-search-indexer`.
