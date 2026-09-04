# feature-flags Specification

## Purpose
TBD - created by archiving change add-flipt-infra. Update Purpose after archive.

## Requirements

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

### Requirement: OpenFeature + Flipt provider is wired into the Go services

The system SHALL initialize an OpenFeature client backed by the Flipt provider inside a Go
service's bootstrap/lifecycle (opened on startup, closed on shutdown), evaluating flags
against an **in-memory snapshot** that Flipt keeps fresh over a gRPC stream, so a flag check
is an in-process lookup rather than a per-request network call. `team-order` is the
representative service.

#### Scenario: The service starts with a working flag client

- **WHEN** `team-order` boots with `FLIPT_ADDR` pointing at the Flipt server
- **THEN** its bootstrap constructs an OpenFeature client with the Flipt provider, the client
  is available to handlers, and the provider maintains an in-memory flag snapshot streamed
  from Flipt (no per-evaluation round-trip)

#### Scenario: The flag client shuts down cleanly

- **WHEN** `team-order` shuts down
- **THEN** the OpenFeature provider / Flipt stream is closed as part of resource teardown

### Requirement: OpenFeature + Flipt provider is wired into the frontend, server-side only

The system SHALL evaluate feature flags in `team-frontend` using an OpenFeature client with
the Flipt provider **only** in server-side code (Server Components / Server Actions), never in
the browser, so the client never learns the Flipt endpoint and never evaluates flags itself.

#### Scenario: The browser never evaluates flags

- **WHEN** a page whose UI depends on a flag is rendered
- **THEN** the flag is evaluated on the server and the browser receives already-resolved
  markup, with no flag SDK, flag state, or Flipt address shipped to the client

### Requirement: Checkout has an emergency kill-switch that flips without redeploy

The system SHALL gate the checkout / place-order path on a boolean flag `checkout-enabled`
evaluated against Flipt, such that flipping the flag off in Flipt disables checkout — hidden
in the UI and rejected by `team-order` — within seconds, with no PR, image rebuild, or pod
restart, and flipping it back on restores checkout the same way. The flag SHALL default to
**on** so that a Flipt outage does not disable checkout.

#### Scenario: Kill-switch off blocks checkout end-to-end

- **WHEN** `checkout-enabled` is turned off in Flipt and a buyer tries to check out
- **THEN** the checkout / place-order entry point is hidden or disabled in the UI, and if a
  `CreateOrder` request reaches `team-order` it is rejected with a clear "checkout
  unavailable" gRPC error instead of running the purchase saga

#### Scenario: Kill-switch on allows checkout

- **WHEN** `checkout-enabled` is on (or Flipt is unreachable and the default applies)
- **THEN** the checkout entry point is shown and a buyer can place an order through the normal
  gateway → team-order path

#### Scenario: Toggling takes effect without a redeploy

- **WHEN** an operator flips `checkout-enabled` in the Flipt UI while the services and frontend
  are running unchanged
- **THEN** the new state takes effect within seconds through the streamed in-memory snapshot,
  with no code change, image rebuild, ArgoCD sync, or pod restart
