# Tasks

## 1. Code — platform-gitops (charts/service)
- [x] `charts/service/values.yaml`: add `grpcPort: 0` (0/empty = disabled) with a doc comment; add
      probe-timing knobs (`probes.initialDelaySeconds`, `probes.periodSeconds`, `probes.failureThreshold`,
      `probes.startup.failureThreshold`) defaulting to current values (5/5) and a startup budget; keep
      `resources: {}` default but expand the example comment to a real requests/limits block.
- [x] `charts/service/templates/deployment.yaml`:
      - When `.Values.grpcPort` is set, render `readinessProbe` + `livenessProbe` + `startupProbe` as
        `grpc:` probes on `grpcPort`.
      - When `.Values.healthPath` is set, render HTTP `livenessProbe` + `startupProbe` in addition to the
        existing readiness probe.
      - Drive `initialDelaySeconds`/`periodSeconds`/`failureThreshold` from the new `probes.*` values.
      - NOTE: grpcPort and healthPath made mutually exclusive (`if grpcPort > 0 … else if healthPath`)
        because K8s permits only one readiness/liveness/startup probe per container. Only team-ai sets
        both; it now gets gRPC probes (its primary :50060 interface) instead of the old HTTP readiness.
- [x] Confirm `charts/service/templates/hpa.yaml` targets `resources.requests.cpu`; no change expected,
      just verify it composes with the new resource defaults. (Verified: unchanged, renders CPU
      Utilization HPA and Deployment omits `replicas` when autoscaling enabled.)
- [x] `helm lint charts/service` clean; `helm template` for a gRPC service (grpcPort=50051) and an HTTP
      service (healthPath=/healthz) shows the expected probes; `kubectl apply --dry-run=client` accepts both.
      NOTE: used `--dry-run=client` (no live cluster here); `--dry-run=server` deferred (see below).

## 2. Code — platform-gitops (per-service enablement)
- [x] Set `grpcPort` for the 7 gRPC services via their `argocd/apps/<svc>.yaml` inline helm values
      (team-domain 50051, team-identity 50053, team-engagement 50054, team-order 50055, team-payment 50056,
      team-chat 50057, team-notification 50058). (Override mechanism is inline `helm.values` per app, not
      envs/local/values.yaml which only holds image tags.)
- [x] Declare `resources.requests` (≥ CPU) for every chart-based service; kept team-ai's existing values.
      Added CPU+mem requests/limits to the 7 gRPC apps, team-gateway, team-frontend, and team-search.
      (team-search is a gRPC :50052 service but NOT in the enumerated 7 — grpcPort deferred as follow-up,
      CPU request added now. search-indexer is a raw-manifest app, not chart-based — out of scope.)
- [x] Enable `autoscaling: { enabled: true, type: cpu }` on the representative stateless service
      (team-gateway); left team-ai on KEDA.
- [ ] **Precondition note (do not install here):** the CPU HPA needs metrics-server in the cluster, and
      each gRPC service needs the standard gRPC Health service on `grpcPort` for the gRPC probe to pass —
      audit which of the 7 expose it; file follow-ups for any that don't. DEFERRED — needs app-code audit
      + live cluster; metrics-server Application already exists at argocd/apps/metrics-server.yaml.
- [ ] `argocd app diff` (or kind sync) clean for the touched Applications. DEFERRED — no live cluster /
      ArgoCD here. Offline equivalent done: `helm template` end-to-end + `kubectl apply --dry-run=client`
      clean for team-domain, team-gateway, team-notification, and team-ai.

## 3. E2E (platform-e2e)
- [ ] **None — infrastructure/manifests, non-user-facing.** No `FEATURES.yaml`/`.feature`. Verification is
      `helm lint` + `helm template` + `--dry-run=server` (and a kind smoke sync) in tracks 1–2. Recorded so
      the archive gate reads "no scenarios", not "missing scenarios".

## 4. Archive
- [x] Probes render for both service shapes; HPA composes with the declared CPU request. (Verified via
      helm template + kubectl --dry-run=client.) ArgoCD diff DEFERRED — no live cluster.
- [x] Sync the `deploy-runtime` spec delta into `openspec/specs/deploy-runtime/spec.md`. (New capability spec created.)
- [x] Archive the change — moved to `changes/archive/2026-09-02-harden-service-helm/`.
      (openspec CLI not installed; archived manually in the established format.)
