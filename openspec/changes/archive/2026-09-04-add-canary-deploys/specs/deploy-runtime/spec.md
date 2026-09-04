## ADDED Requirements

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
