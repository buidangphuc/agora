## Why

`harden-service-helm` gave the shared `charts/service` chart real probes/resources/HPA, and
`add-staging-prod-overlays` added `envs/staging` + `envs/prod` and the `marketplace-services`
ApplicationSet. But every service still deploys as a plain `Deployment`: a new image tag is rolled
out by Kubernetes' rolling update straight to 100% of traffic, with no staged exposure and no
automated safety check. A bad release in staging/prod is only caught after it is already serving
everyone, and rollback is a manual `argocd rollback` / image re-bump.

There is also now a real metrics source to gate on: `add-prometheus-infra` routes per-upstream RED
metrics (`marketplace_rpc_client_*`, labelled by target service) from the gateway into the in-cluster
Prometheus at `http://prometheus.marketplace.svc:9090` (which already carries team-ai's
`teamai_grpc_requests_total`). That is exactly the RPS / error-rate / latency signal a canary needs
to decide promote-or-rollback.

This change introduces **progressive delivery via Argo Rollouts**: an opt-in variant of the shared
chart renders a `Rollout` with a canary strategy (staged traffic weights + pauses) instead of a
`Deployment`, and an `AnalysisTemplate` queries Prometheus to auto-promote a healthy canary or
auto-rollback a bad one. It is enabled for the representative service in staging/prod only; local and
every non-opted service are byte-identical to today.

## What Changes

- **platform-gitops (`charts/service`)** — add an opt-in `rollout.enabled` flag. When set, the chart
  renders an `argoproj.io/v1alpha1` `Rollout` (canary strategy: `setWeight`/`pause` steps) in place of
  the `Deployment`; when unset (default) the `Deployment` renders exactly as today. The pod/container
  spec (image, ports, probes, resources, env, secrets) is factored into a shared named template so
  Deployment and Rollout never drift.
- **platform-gitops (`charts/service/templates/analysistemplate.yaml`, new)** — render an
  `AnalysisTemplate` (Prometheus provider at `http://prometheus.marketplace.svc:9090`) with
  error-rate + latency (and RPS-sanity) metrics driven by `rollout.analysis.*` values; the Rollout's
  canary steps reference it so a canary that breaches thresholds is rolled back automatically.
- **platform-gitops (`charts/service/templates/hpa.yaml`)** — when `rollout.enabled`, point the HPA
  `scaleTargetRef` at the `Rollout` (kind `Rollout`, `apiVersion argoproj.io/v1alpha1`) instead of the
  `Deployment`, so the Rollout owns replica count and HPA still scales on CPU.
- **platform-gitops (`charts/service/values.yaml`)** — document `rollout.*` (enabled, canary steps,
  analysis queries/thresholds, prometheus address) defaulting to disabled so nothing regresses.
- **platform-gitops (`argocd/apps/argo-rollouts.yaml`, new)** — declare the Argo Rollouts controller as
  an ArgoCD Application (upstream `argo-rollouts` helm chart, early sync-wave), mirroring how
  `keda.yaml` / `metrics-server.yaml` declare their controllers. Declaring it is in scope; installing/
  bootstrapping it on a real cluster is not (see Non-goals).
- **platform-gitops (`envs/staging/services/team-gateway.yaml`, `envs/prod/services/team-gateway.yaml`)**
  — turn on `rollout.enabled` (+ canary steps/analysis) for the representative stateless service in
  staging and prod only. `envs/local`, the per-service baseline, and every other service are untouched.
- **E2E (platform-e2e)** — none. This is infra/manifests, non-user-facing (mirrors `harden-service-helm`):
  no `FEATURES.yaml`/`.feature`. The verification gate is `helm lint` + `helm template` +
  `kubectl apply --dry-run` of the Rollout + AnalysisTemplate, plus a staged rollout + analysis run on a
  cluster — cluster-gated.

## Non-goals

- **Installing / bootstrapping the Argo Rollouts controller on a cluster.** This change *declares* the
  controller Application (and its CRDs) in Git; actually syncing it, and the one-time CRD install, is a
  cluster bootstrap step — cluster-gated, out of scope here.
- **Service-mesh traffic splitting** (Istio/SMI/Gateway-API `trafficRouting`). The canary uses the basic
  replica-weighted strategy (Rollout adjusts stable/canary replica counts); no mesh is introduced. A
  mesh-backed weighted canary is a later change if/when a mesh exists.
- **Enabling canary in `local`** or on every service. Local kind is single-node/ephemeral and has no
  need for staged rollout; only the representative service in staging/prod opts in. KEDA-scaled team-ai
  stays a `Deployment` for now.
- **No `team-*` application-code change**, no proto change, no new user-facing capability.
- **No new environments** (`add-staging-prod-overlays`), **no Prometheus/metrics install**
  (`add-prometheus-infra` / `metrics-server`) — this change consumes them, it does not add them.
