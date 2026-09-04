## Why

The shared Helm chart `platform-gitops/charts/service` is the single template every marketplace
service deploys from, but its health/scaling wiring is thin and unsafe for anything past a local
kind cluster:

- **Only a readiness probe exists**, and only when `healthPath` is set (HTTP `httpGet`). The gRPC
  services — team-domain :50051, team-identity :50053, team-engagement :50054, team-order :50055,
  team-payment :50056, team-chat :50057, team-notification :50058 — set no `healthPath`, so they get
  **no probe at all**: a wedged gRPC pod is never restarted or pulled from endpoints.
- **No `livenessProbe`, no `startupProbe`** anywhere — slow starters can be killed before they boot,
  and hung processes are never recycled.
- **`resources: {}` by default** — only team-ai declares requests/limits. Without a CPU *request* the
  CPU HPA (`templates/hpa.yaml`, already present) cannot compute utilization, so autoscaling is inert
  for every service.

This change hardens the generic chart so probes and resources are first-class and gRPC services are
covered, then turns them on per service. It is the SRE "Beta → staging-ready" fix from the review
(the 7.1 axis). It is scoped to the chart + per-service values; new environments and canary come later.

## What Changes

- **platform-gitops (`charts/service/templates/deployment.yaml`)** — add a `grpcPort` value; when set,
  emit native Kubernetes **gRPC** `readinessProbe` + `livenessProbe` + `startupProbe` (`grpc:` probe
  type). When `healthPath` is set, additionally emit HTTP `livenessProbe` + `startupProbe` (today only
  readiness is rendered). Keep the existing HTTP readiness behavior backward-compatible.
- **platform-gitops (`charts/service/values.yaml`)** — document `grpcPort`, and add sane default
  `resources.requests/limits` guidance (kept as `{}` default so nothing regresses, filled in per
  service below). Probe timing knobs (`initialDelaySeconds`, `periodSeconds`, `failureThreshold`)
  exposed as values with the current defaults.
- **platform-gitops (`envs/local/values.yaml` and/or `argocd/apps/<svc>.yaml`)** — set `grpcPort` for
  the 7 gRPC services, `healthPath` already-set services keep theirs, and declare `resources.requests`
  (at least CPU) for every service so the HPA path is usable. Enable the CPU HPA
  (`autoscaling.enabled: true, type: cpu`) on the stateless request-servers (gateway, and the read-heavy
  services) as the representative rollout; leave team-ai on its existing KEDA config.
- **Verification** — `helm template` + `helm lint` render valid probes for one HTTP service
  (team-gateway) and one gRPC service (team-domain); `kubectl apply --dry-run=server` (or kind) accepts
  the manifests; ArgoCD diff is clean.

## Non-goals

- **No new environments.** `envs/staging` / `envs/prod` overlays are `add-staging-prod-overlays`.
- **No canary / progressive delivery** (Argo Rollouts) — that is `add-canary-deploys`.
- **No app-code health endpoints added** — services that need an HTTP `/healthz` or a gRPC health
  service already expose one; this change only wires probes to existing endpoints. If a gRPC service
  lacks the standard gRPC Health service, note it in tasks (it does not block the chart change).
- **No backup, Prometheus, or metrics-server install** — `add-postgres-backup-cronjob` /
  `add-prometheus-infra` handle those. (Note the CPU HPA needs metrics-server; call it out, don't install here.)
- **No changes to any `team-*` application code.**
