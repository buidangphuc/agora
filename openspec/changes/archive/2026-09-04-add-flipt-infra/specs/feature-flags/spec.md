## ADDED Requirements

### Requirement: A self-hosted flag server runs in local infra

The system SHALL run a Flipt flag server as part of the shared local infra stack
(`platform-core/infra/docker-compose.yaml`), on the `platform-core_default` network,
exposing a gRPC evaluation endpoint and an HTTP/UI endpoint, with a **persisted flag
store** so flag definitions and toggle state survive a container restart.

#### Scenario: Flipt starts with the local stack and is reachable in-network

- **WHEN** an operator brings up the local infra (`docker compose -p platform-core up -d`)
- **THEN** a `flipt` container is running and healthy, its gRPC endpoint is reachable at
  `flipt:9000` and its HTTP/UI at `flipt:8080` from other containers on
  `platform-core_default`, and its flag store is backed by a named volume

#### Scenario: Toggle state survives a restart

- **WHEN** a flag's state is changed in Flipt and the `flipt` container is then restarted
- **THEN** the previously saved flag definition and its state are still present (the store
  is persisted on a volume, not ephemeral)

### Requirement: The flag server is deployed via GitOps

The system SHALL deploy the Flipt flag server to the `marketplace` namespace through
GitOps (a manifest under `platform-gitops/platform/infra/` reconciled by the existing
`infra-shared` ArgoCD Application), with a Kubernetes Service exposing its gRPC and HTTP
ports and a persistent flag store, and it SHALL become ready before the services that
will evaluate flags.

#### Scenario: ArgoCD reconciles Flipt as shared infra

- **WHEN** the GitOps repo is synced by ArgoCD
- **THEN** a `flipt` Deployment and Service exist in the `marketplace` namespace, the
  Service resolves as `flipt.marketplace.svc` on its gRPC and HTTP ports, and — because it
  is reconciled at the infra sync-wave (0) ahead of the services (wave 4) — Flipt is
  available before any service that evaluates a flag starts

#### Scenario: The deployed flag store is persistent

- **WHEN** the Flipt pod is rescheduled or restarted in the cluster
- **THEN** its flag store is backed by a PersistentVolumeClaim, so saved flags and their
  state are retained across pod restarts
