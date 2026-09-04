## ADDED Requirements

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
