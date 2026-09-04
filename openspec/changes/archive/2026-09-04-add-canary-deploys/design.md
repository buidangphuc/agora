## Context

The shared `charts/service` chart renders a plain `Deployment` (`templates/deployment.yaml`) plus a
CPU `HorizontalPodAutoscaler` (`templates/hpa.yaml`) or a KEDA `ScaledObject` (`templates/scaledobject.yaml`).
`add-staging-prod-overlays` layered an env dimension on top via the `marketplace-services` ApplicationSet
(precedence: chart defaults → `envs/<env>/values.yaml` → `envs/services/<svc>.yaml` →
`envs/<env>/services.yaml` → `envs/<env>/services/<svc>.yaml` → app inline). Controllers (KEDA,
metrics-server) are declared as ArgoCD Applications pulling upstream helm charts at early sync-waves.
`add-prometheus-infra` put a real RED metrics source in-cluster: gateway per-upstream
`marketplace_rpc_client_*` series (labelled by target service) plus team-ai's `teamai_grpc_requests_total`,
in Prometheus at `http://prometheus.marketplace.svc:9090`.

This change adds progressive delivery on that foundation without forking the chart or touching app code.

## Decisions

### D1 — Argo Rollouts, not Argo CD sync-waves-only

Argo CD **sync-waves** only order the *apply* of resources within a single sync (wave N applies before
N+1); they cannot hold a release at a traffic percentage, run an analysis, or auto-rollback. They give
ordering, not progressive delivery. **Argo Rollouts** replaces the `Deployment` with a `Rollout` CR whose
`strategy.canary` runs `setWeight`/`pause` steps and, via an `AnalysisTemplate`, promotes or rolls back
from live metrics. Since the whole point is *staged traffic + metric-gated auto-rollback*, Rollouts is the
only option that meets the requirement; sync-waves stay as-is for cross-resource ordering (they compose,
they don't compete). Cost: a new controller + CRDs to run (declared as an ArgoCD app like KEDA).

### D2 — Opt-in `rollout.enabled` flag in the chart, shared pod template — not a separate chart/template

Options: (a) convert every service's `Deployment` to a `Rollout` unconditionally; (b) a second, parallel
Rollout-only template that duplicates the pod spec; (c) an opt-in `rollout.enabled` flag where the chart
renders *either* a Deployment *or* a Rollout, both built from **one shared pod/container named template**.
Chosen **(c)**. (a) forces canary blast radius onto local/kind and KEDA services that don't want it; (b)
duplicates the hardened probes/resources/env block and will drift the moment one side is edited. (c) keeps
a single source of truth: extract the container spec (image, ports, probes, resources, envFrom/env,
serviceAccount) into `templates/_container.tpl`, and have both `deployment.yaml` (guarded
`{{- if not .Values.rollout.enabled }}`) and the new `rollout.yaml` (`{{- if .Values.rollout.enabled }}`)
`include` it. Default `rollout.enabled: false` → the Deployment renders byte-identical to today for every
un-opted service and all of local.

### D3 — Basic (replica-weighted) canary, no service mesh

The `strategy.canary` uses the default replica-weighted mechanism: with no `trafficRouting`, Rollouts
approximates each `setWeight` by adjusting the canary vs stable replica counts. This needs no mesh and no
new networking. Representative steps: `setWeight 20 → pause+analysis → setWeight 50 → pause+analysis →
setWeight 100`. Weight granularity is limited by replica count (20% needs ≥5 pods to be exact), which is
acceptable for a coarse safety gate; a mesh-backed fine-grained weighted canary is a non-goal.

### D4 — AnalysisTemplate queries the Prometheus that already exists

A chart `templates/analysistemplate.yaml` renders an `AnalysisTemplate` with `provider.prometheus.address`
= `http://prometheus.marketplace.svc:9090` (overridable via `rollout.analysis.prometheusAddress`) and
metrics driven by `rollout.analysis.*`:
- **error-rate** — `successCondition` on `sum(rate(marketplace_rpc_client_requests_total{service="<svc>",grpc_code!="OK"}[...])) / sum(rate(marketplace_rpc_client_requests_total{service="<svc>"}[...]))` below a threshold;
- **latency** — p95 of the gateway client duration histogram for the service below a threshold;
- **rps-sanity** — canary is actually receiving traffic (guards a false "healthy because idle").

The gateway is the single edge emitting per-upstream RED (rules 1–2), so one Prometheus source covers
every canaried upstream; the gateway's own canary uses its server-side series. Queries/thresholds are
values, not hard-coded, so each service tunes its own SLO. The Rollout references the template from a
`pause`/`analysis` step; `failureLimit` breaches → automatic rollback to stable; passing → auto-promote to
the next weight.

### D5 — HPA targets the Rollout; Rollout owns replicas

Argo Rollouts is HPA-compatible **only if** the HPA `scaleTargetRef` points at the `Rollout`
(`apiVersion: argoproj.io/v1alpha1`, `kind: Rollout`), not the Deployment. So `hpa.yaml` switches its
`scaleTargetRef` when `rollout.enabled`. The Rollout omits a static `replicas` for the same reason the
Deployment does today (HPA owns count). KEDA `ScaledObject` would likewise need `scaleTargetRef` → Rollout,
but team-ai (the only KEDA service) stays a Deployment in this change, so `scaledobject.yaml` is left
untouched and that combination is called out as a follow-up.

### D6 — Enabled in staging/prod only; controller declared, not installed

`rollout.enabled: true` is set only in `envs/staging/services/team-gateway.yaml` and
`envs/prod/services/team-gateway.yaml` (the representative stateless CPU-HPA service). `envs/local`, the
per-service baseline, and all other services keep plain Deployments. The Argo Rollouts controller is
declared as `argocd/apps/argo-rollouts.yaml` (upstream `argo-rollouts` chart, `crds.install: true`,
early sync-wave, `ServerSideApply=true` — mirroring `keda.yaml`); actually syncing it and installing the
CRDs on a live cluster is a one-time bootstrap, cluster-gated and out of scope.

## Risks / Trade-offs

- **Replica-weighted accuracy** — without traffic routing, `setWeight: 20` is only approximate at low
  replica counts; a canary at 1–2 pods over-serves. Mitigate with a sane HPA `minReplicas` on canaried
  services and coarse weights; exact splitting is deferred to a mesh (non-goal).
- **CRD dependency ordering** — a `Rollout` manifest is invalid until the Rollouts CRDs exist. If a
  service opts in before the controller Application has synced, its ArgoCD app degrades. Mitigate:
  controller at an earlier sync-wave than the services (like KEDA), and opt-in scoped to staging/prod
  where the bootstrap has run.
- **Analysis false-positives/negatives** — thresholds too tight roll back good releases; too loose let bad
  ones through. Queries + thresholds are per-service values so they can be tuned; the rps-sanity metric
  guards the "healthy because no traffic" failure mode.
- **Metric dependency** — canary analysis is only as good as `add-prometheus-infra`; if the gateway meter
  is off (`OTEL_ENABLED=false`) the series are empty and analysis has nothing to gate on. The rps-sanity
  metric surfaces this as an inconclusive/failed analysis rather than a silent auto-promote.
- **Verification is cluster-gated** — offline we can only `helm lint` / `helm template` /
  `kubectl apply --dry-run` the Rollout + AnalysisTemplate (schema validity). A real staged rollout with a
  live analysis run needs the controller + Prometheus + traffic on a cluster; recorded in tasks as
  deferred, not run here.
