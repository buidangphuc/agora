# deploy-runtime Specification

## Purpose

The deploy-runtime capability governs how the shared Helm chart `platform-gitops/charts/service`
renders health probes and resources so services are safe to run beyond a local kind cluster. It
covers both service shapes on the platform — HTTP edge services (team-gateway, team-frontend,
team-ai HTTP) and gRPC services (team-domain, team-identity, team-engagement, team-order,
team-payment, team-chat, team-notification) — ensuring each is health-probed, restartable, and
autoscalable. Environments (staging/prod overlays) and progressive delivery (canary) are separate
capabilities/changes.

## Requirements

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

### Requirement: Per-environment value overlays exist for staging and prod

The `platform-gitops/envs/` tree SHALL provide `staging` and `prod` overlays alongside `local`, each
supplying the same value shape `local` does — `global.registry`, `global.imagePullPolicy`, and an
`images: { <service>: <tag> }` map keyed by service — so the shared `charts/service` chart resolves a
per-environment image tag without any chart template change. Each environment SHALL be able to set
per-service resource requests/limits, replica counts, autoscaling bounds/targets, and ingress hosts
that differ from `local`, without editing the chart.

#### Scenario: Each environment resolves its own image tag

- **WHEN** `charts/service` is rendered for a service against `envs/staging/values.yaml` and again
  against `envs/prod/values.yaml`
- **THEN** the container image tag resolves from that environment's `images.<service>` value (not
  `local`), using the environment's `global.registry`

#### Scenario: An environment overrides sizing without a chart change

- **WHEN** an environment sets a higher `resources.requests`/`replicas`/`autoscaling` bound for a
  service in its overlay
- **THEN** `helm template` for that env renders the overridden values while other environments are
  unaffected, and no `charts/service` template was modified

### Requirement: ArgoCD targets an environment overlay deterministically

The ArgoCD Applications SHALL target a chosen environment's overlay (via an ApplicationSet over
`envs/*` or per-env Application directories) so that staging and prod Applications reference
`$values/envs/<env>/values.yaml` and layer per-env service overrides in a defined precedence
(chart defaults → env image values → per-env service overlay → app inline defaults). The existing
`local` deployment path SHALL keep working unchanged.

#### Scenario: Staging and prod Applications point at their own overlay

- **WHEN** the ArgoCD Applications are generated for staging and for prod
- **THEN** each Application's Helm source references `$values/envs/<env>/values.yaml` for its
  environment, and value precedence resolves image tag and per-env sizing deterministically

#### Scenario: Local deployment is unchanged

- **WHEN** the platform is synced for the `local` environment after this change
- **THEN** the rendered manifests for `local` are equivalent to before (same image tags, resources,
  and probes), i.e. adding staging/prod overlays does not regress local

#### Scenario: Overlays render and validate for both service shapes

- **WHEN** `helm template` and `helm lint` run for one HTTP service and one gRPC service against the
  staging and prod overlays, and the output is checked with `kubectl apply --dry-run`
- **THEN** both render valid manifests carrying the hardened probes/resources/HPA from
  `deploy-runtime`, with the env-specific tags and sizing applied, and the dry-run is accepted

### Requirement: Chart renders a canary Rollout when progressive delivery is opted in

The shared `charts/service` chart SHALL support an opt-in `rollout.enabled` flag. When set, the chart
SHALL render an `argoproj.io/v1alpha1` `Rollout` (in place of the `Deployment`) whose `strategy.canary`
advances through staged `setWeight`/`pause` steps, and SHALL NOT render a `Deployment`. When
`rollout.enabled` is unset (the default), the chart SHALL render the `Deployment` exactly as today and no
`Rollout`. The `Rollout` and `Deployment` SHALL share one pod/container spec (image, ports, probes,
resources, env, secrets) so the two shapes cannot drift.

#### Scenario: Opting in renders a Rollout, not a Deployment

- **WHEN** `helm template` renders `charts/service` for a service with `rollout.enabled: true`
- **THEN** the output contains a `kind: Rollout` (`apiVersion: argoproj.io/v1alpha1`) with a
  `strategy.canary` block of `setWeight`/`pause` steps, and contains no `kind: Deployment`

#### Scenario: Default render is unchanged

- **WHEN** `helm template` renders `charts/service` for a service that does not set `rollout.enabled`
- **THEN** the output contains a `kind: Deployment` with the same probes and resources as today and no
  `kind: Rollout`

### Requirement: Canary is analysed against Prometheus and auto-promotes or auto-rolls-back

When `rollout.enabled`, the chart SHALL render an `AnalysisTemplate` whose Prometheus provider points at
the in-cluster Prometheus (`http://prometheus.marketplace.svc:9090`, overridable) and whose metrics cover
error-rate, latency, and request-rate sanity for the service, driven by `rollout.analysis.*` values. The
`Rollout` SHALL reference this `AnalysisTemplate` from its canary steps so that a canary breaching a
metric's `failureLimit` is rolled back to the stable version automatically, and a canary that passes is
promoted to the next weight automatically.

#### Scenario: Rollout wires canary analysis to Prometheus

- **WHEN** `helm template` renders `charts/service` for a service with `rollout.enabled: true`
- **THEN** the output includes a `kind: AnalysisTemplate` with a `prometheus` provider addressing
  `http://prometheus.marketplace.svc:9090` and error-rate/latency/rps metrics, and the `Rollout`'s canary
  steps reference that template by name with a `failureLimit` that triggers rollback

### Requirement: A canary Rollout composes with the HPA

When `rollout.enabled` and CPU autoscaling is enabled, the rendered `HorizontalPodAutoscaler`
`scaleTargetRef` SHALL point at the `Rollout` (`apiVersion: argoproj.io/v1alpha1`, `kind: Rollout`), not
the `Deployment`, and the `Rollout` SHALL omit a static `replicas` so the HPA owns replica count.

#### Scenario: HPA targets the Rollout

- **WHEN** a service sets `rollout.enabled: true` and `autoscaling: { enabled: true, type: cpu }`
- **THEN** `helm template` emits an `autoscaling/v2` HPA whose `scaleTargetRef` is `kind: Rollout`
  (`apiVersion: argoproj.io/v1alpha1`) and the `Rollout` has no hard-coded `replicas`

### Requirement: Progressive delivery is scoped to staging/prod with the controller declared in Git

Canary progressive delivery SHALL be enabled only for the representative service in `staging` and `prod`
(via `envs/staging/services/<svc>.yaml` and `envs/prod/services/<svc>.yaml`); `envs/local`, the per-service
baseline, and non-opted services SHALL keep plain `Deployment`s. The Argo Rollouts controller SHALL be
declared as an ArgoCD `Application` (upstream `argo-rollouts` chart, CRDs installed, at an earlier
sync-wave than the services), mirroring the KEDA/metrics-server controller Applications.

#### Scenario: Local render is a plain Deployment

- **WHEN** `helm template` renders `charts/service` for a service using the `local` overlay
- **THEN** the output is a `kind: Deployment` (no `Rollout`, no `AnalysisTemplate`), unchanged from today

#### Scenario: Controller Application is declared

- **WHEN** the `argocd/apps` directory is rendered/synced
- **THEN** an `Application` for the Argo Rollouts controller exists (upstream chart, `crds.install: true`,
  sync-wave earlier than the marketplace services), analogous to `argocd/apps/keda.yaml`
