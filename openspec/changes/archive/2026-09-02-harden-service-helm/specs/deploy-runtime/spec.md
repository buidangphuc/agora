## ADDED Requirements

### Requirement: gRPC services are health-probed

The shared `charts/service` chart SHALL, when a service declares a `grpcPort`, render Kubernetes
native gRPC `readinessProbe`, `livenessProbe`, and `startupProbe` against that port, so a gRPC service
that is not yet ready is kept out of endpoints, a hung one is restarted, and a slow-starting one is not
killed during boot. The seven gRPC services (team-domain, team-identity, team-engagement, team-order,
team-payment, team-chat, team-notification) SHALL declare `grpcPort` and thereby receive these probes.
Because Kubernetes permits only one probe of each kind per container, `grpcPort` and `healthPath` are
mutually exclusive; a service that primarily serves gRPC (e.g. team-ai on :50060) is probed over gRPC.

#### Scenario: gRPC service renders all three probes

- **WHEN** `helm template` renders `charts/service` for a service with `grpcPort` set (e.g. team-domain :50051)
- **THEN** the Deployment container includes `readinessProbe`, `livenessProbe`, and `startupProbe`, each a
  `grpc:` probe on that port

#### Scenario: HTTP service also gets liveness and startup

- **WHEN** `helm template` renders `charts/service` for a service with `healthPath` set (e.g. team-gateway `/healthz`)
- **THEN** the container includes an HTTP `readinessProbe` (as today) plus HTTP `livenessProbe` and
  `startupProbe` on the same path and port

### Requirement: Services declare resource requests so autoscaling works

The chart SHALL support per-service `resources.requests`/`limits`, and every deployed service SHALL
declare at least a CPU request. When a service enables the CPU HPA (`autoscaling.enabled: true`,
`type: cpu`), the rendered `HorizontalPodAutoscaler` SHALL target CPU utilization against that request,
and the Deployment SHALL omit a static `replicas` so the HPA owns replica count.

#### Scenario: CPU HPA renders against a declared request

- **WHEN** a service sets `resources.requests.cpu` and `autoscaling: { enabled: true, type: cpu }`
- **THEN** `helm template` emits an `autoscaling/v2` HPA with a CPU-utilization target and the Deployment
  has no hard-coded `replicas`

#### Scenario: Chart renders and lints cleanly for both service shapes

- **WHEN** `helm lint` and `helm template` run for one HTTP service and one gRPC service, and the output
  is validated with `kubectl apply --dry-run`
- **THEN** both render valid manifests with the probes and resources present, and the dry-run is accepted
