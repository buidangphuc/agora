Feature: The cluster stores, serves and keeps fresh the trained recommendations
  Static, no stack needed: the raw manifests of platform-gitops are parsed, and charts/service is
  rendered with `helm template` for team-ai. (recs-on-cluster / recs-on-cluster)

  Scenario: Qdrant is pinned to the compose version
    When the Qdrant manifest and the compose infra file are read
    Then the Qdrant StatefulSet image tag equals the compose qdrant image tag and is not latest

  Scenario: Qdrant is persistent and hardened
    When the Qdrant manifest is rendered
    Then it has a volume claim template mounted at /qdrant/storage, resource requests and limits, and a non-root, read-only-root-filesystem security context with all capabilities dropped, and a Service qdrant exposes 6333 and 6334

  Scenario: Qdrant accepts only its recommendation clients
    When the Qdrant NetworkPolicy is rendered
    Then its only ingress sources are app team-ai and app platform-recsys

  Scenario: team-ai points at the cluster stores
    When charts/service is rendered for team-ai with its recs service values
    Then the container env enables the qdrant backend on Service qdrant and the recsys collection, reads featurestore features from valkey DB 2 and nearline signals from valkey DB 0, and the cache prefix, schema and Redis host match the recsys job config

  Scenario: team-ai keeps its ingress allow-list
    When charts/service is rendered for team-ai with its recs service values
    Then its NetworkPolicy ingress sources are exactly team-gateway, team-search-indexer and prometheus

  Scenario: The nearline consumer is wired to the cluster brokers
    When the nearline manifest is rendered
    Then the Deployment has one replica, runs python -m recsys.nearline from the platform-recsys image, and its env names the redpanda broker, topic analytics.events and Redis host valkey, which exist as Services in the cluster

  Scenario: The recsys job and nearline egress is limited to their stores
    When the recsys and nearline NetworkPolicies are rendered
    Then each allows no ingress, and its egress targets are DNS plus exactly the store pods and ports named above

  Scenario: The recommendation images are on the push list
    When the image push script and the local values are read
    Then the push list names platform-featurestore and platform-recsys, the local values carry both image keys, and every localhost:5001 image the recsys and featurestore manifests use is on the push list

  Scenario: Qdrant and nearline are synced by an Application
    When the manifest paths and the Applications are read
    Then an Application syncs the directory holding the Qdrant manifest and one syncs the directory holding the nearline manifest
