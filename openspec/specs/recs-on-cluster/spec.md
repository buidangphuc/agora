# recs-on-cluster Specification

## Purpose
TBD - created by archiving change recs-on-cluster. Update Purpose after archive.

## Requirements

### Requirement: The cluster runs Qdrant for recommendations

The GitOps repo SHALL define Qdrant as a StatefulSet in `platform/infra` with a persistent volume claim template mounted at
`/qdrant/storage`, a ClusterIP Service named `qdrant` exposing 6333 and 6334, resource requests and limits, a non-root
security context with a read-only root filesystem, and an image tag equal to the one docker compose pins. A
NetworkPolicy SHALL allow ingress to it only from `team-ai` and `platform-recsys`.

#### Scenario: Qdrant is pinned to the compose version

- **WHEN** the Qdrant manifest and the compose infra file are read
- **THEN** the Qdrant StatefulSet image tag equals the compose `qdrant` image tag and is not `latest`

#### Scenario: Qdrant is persistent and hardened

- **WHEN** the Qdrant manifest is rendered
- **THEN** it has a volume claim template mounted at `/qdrant/storage`, resource requests and limits, and a non-root,
  read-only-root-filesystem security context with all capabilities dropped, and a Service `qdrant` exposes 6333 and 6334

#### Scenario: Qdrant accepts only its recommendation clients

- **WHEN** the Qdrant NetworkPolicy is rendered
- **THEN** its only ingress sources are `app: team-ai` and `app: platform-recsys`

### Requirement: team-ai serves recommendations from the cluster stores

team-ai's cluster values SHALL enable recommendations with the qdrant backend and point at the in-cluster Qdrant and valkey
Services: the collection and cache contract the recsys job writes, featurestore online features on Redis DB 2, and nearline
signals on Redis DB 0. team-ai's ingress allow-list SHALL stay `team-gateway`, `team-search-indexer` and `prometheus`.

#### Scenario: team-ai points at the cluster stores

- **WHEN** `charts/service` is rendered for team-ai with its service values
- **THEN** the container env has `RECS_ENABLED` true, `RECS_BACKEND` qdrant, `RECS_QDRANT_URL` naming Service `qdrant`
  and `RECS_QDRANT_COLLECTION` equal to the recsys item collection, `RECS_FEATURESTORE_REDIS_URL` on `valkey` DB 2,
  `RECS_NEARLINE_REDIS_URL` on `valkey` DB 0, and the cache prefix, schema and Redis host match the recsys job config

#### Scenario: team-ai keeps its ingress allow-list

- **WHEN** `charts/service` is rendered for team-ai with its service values
- **THEN** its NetworkPolicy ingress sources are exactly `team-gateway`, `team-search-indexer` and `prometheus`

### Requirement: A nearline consumer keeps CTR signals fresh in the cluster

The GitOps repo SHALL define a `platform-recsys-nearline` Deployment with one replica running the platform-recsys image
with entrypoint `python -m recsys.nearline`, consuming `analytics.events` from the `redpanda` Service and writing to the `valkey`
Service on DB 0, with resource requests and limits, a non-root security context, and no secret values.

#### Scenario: The nearline consumer is wired to the cluster brokers

- **WHEN** the nearline manifest is rendered
- **THEN** the Deployment has one replica, runs `python -m recsys.nearline` from the `platform-recsys` image, and its env
  names the `redpanda` broker, topic `analytics.events` and Redis host `valkey`, which exist as Services in the cluster

### Requirement: Recommendation pods reach only the stores they need

The recsys CronJob pods and the nearline pods SHALL each have a NetworkPolicy with no ingress and egress limited to DNS and
their stores: recsys to `qdrant` and `valkey`, nearline to `redpanda` and `valkey`.

#### Scenario: The recsys job and nearline egress is limited to their stores

- **WHEN** the recsys and nearline NetworkPolicies are rendered
- **THEN** each allows no ingress, and its egress targets are DNS plus exactly the store pods and ports named above

### Requirement: The recommendation images are published to the local registry

The image push list SHALL publish `platform-featurestore` and `platform-recsys` to the local registry, record both in the
local image-tag values, and publish team-ai from the image built with the recommendations extra.

#### Scenario: The recommendation images are on the push list

- **WHEN** the image push script and the local values are read
- **THEN** the push list names `platform-featurestore:local` and `platform-recsys:local`, the local values carry both image
  keys, and every `localhost:5001/` image the recsys and featurestore manifests use is on the push list

### Requirement: The new workloads are registered with ArgoCD

The Qdrant manifest SHALL live in a directory synced by an Application under `argocd/apps/`, and so SHALL the nearline
manifest.

#### Scenario: Qdrant and nearline are synced by an Application

- **WHEN** the manifest paths and the Applications are read
- **THEN** an Application syncs the directory holding the Qdrant manifest and one syncs the directory holding the nearline manifest
