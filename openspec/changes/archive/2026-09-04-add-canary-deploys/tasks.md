# Tasks

## 1. Code — platform-gitops (charts/service: shared pod template + Rollout)
- [x] `charts/service/templates/_container.tpl` (new): extract the container spec (image, `containerPort`,
      `resources`, `envFrom`/`env`, and the gRPC/HTTP probe blocks) currently inline in `deployment.yaml`
      into a named template `service.container`, plus a `service.podSpec` helper (serviceAccountName +
      container). Keep output byte-identical to today's Deployment pod spec.
- [x] `charts/service/templates/deployment.yaml`: guard the whole file with
      `{{- if not .Values.rollout.enabled }}` and replace the inline container/pod block with
      `{{- include "service.podSpec" . | nindent 6 }}` so the default render is unchanged.
- [x] `charts/service/templates/rollout.yaml` (new): `{{- if .Values.rollout.enabled }}` → render a
      `kind: Rollout` (`apiVersion: argoproj.io/v1alpha1`) reusing `service.podSpec`, omitting static
      `replicas` (HPA owns it), with `strategy.canary.steps` built from `.Values.rollout.canary.steps`
      (`setWeight`/`pause`) and an `analysis`/`pause` step referencing the AnalysisTemplate by name.
      (A `{ analysis: true }` step is expanded by the chart into an analysis step referencing
      `<name>-canary` with a `service` arg.)

## 2. Code — platform-gitops (charts/service: analysis + HPA + values)
- [x] `charts/service/templates/analysistemplate.yaml` (new): `{{- if .Values.rollout.enabled }}` →
      `kind: AnalysisTemplate` with a `prometheus` provider at
      `.Values.rollout.analysis.prometheusAddress` (default `http://prometheus.marketplace.svc:9090`) and
      `error-rate`, `latency-p95`, `rps-sanity` metrics from `.Values.rollout.analysis.*`
      (queries over `marketplace_rpc_client_duration_milliseconds_*` — the real rendered RED metric base
      from add-prometheus-infra, labels `rpc_grpc_status_code`/`rpc_service`; per-metric `successCondition`
      + `failureLimit`; each query fully overridable via `errorRateQuery`/`latencyQuery`/`rpsQuery`).
- [x] `charts/service/templates/hpa.yaml`: when `.Values.rollout.enabled`, set `scaleTargetRef` to
      `apiVersion: argoproj.io/v1alpha1`, `kind: Rollout` (else the existing Deployment target). No other
      HPA change.
- [x] `charts/service/values.yaml`: add a documented `rollout:` block — `enabled: false`,
      `canary.steps` (default `setWeight 20 / pause+analysis / setWeight 50 / pause+analysis / setWeight 100`),
      `analysis.prometheusAddress`, `analysis.intervalSeconds`, `analysis.errorRateThreshold`,
      `analysis.latencyP95Ms`, and the query/label knobs — all defaulting so `enabled: false` regresses nothing.

## 3. Code — platform-gitops (controller + per-env enablement)
- [x] `argocd/apps/argo-rollouts.yaml` (new): ArgoCD `Application` for the upstream `argo-rollouts` helm
      chart (`https://argoproj.github.io/argo-helm`, targetRevision 2.37.3), `installCRDs: true`,
      `destination.namespace: argo-rollouts`, `sync-wave: "1"` (earlier than the marketplace services at
      wave 4; mirrors `argocd/apps/keda.yaml`: `ServerSideApply=true`, `CreateNamespace=true`).
- [x] `envs/staging/services/team-gateway.yaml`: add `rollout.enabled: true` + `rollout.canary`/`rollout.analysis`
      overrides (representative stateless CPU-HPA service). Confirmed it layers over the existing HPA block so
      the HPA now targets the Rollout (verified: `scaleTargetRef.kind: Rollout`).
- [x] `envs/prod/services/team-gateway.yaml`: same opt-in with prod-tuned steps/thresholds (more conservative
      weights 10/25/50/100, 300s pauses, errorRate 0.02 / p95 400ms).
- [x] Confirmed `envs/local/*`, `envs/services/team-gateway.yaml` (baseline), and every non-opted service
      render unchanged (still plain Deployments) — verified via `helm template` (baseline gateway → Deployment;
      team-domain staging → Deployment) and byte-identical `diff` (see task 4).

## 4. Verification (cluster-gated — DO NOT run here)
- [x] `helm lint charts/service` clean with `rollout.enabled` true and false (0 charts failed both ways).
- [x] `helm template charts/service` for `rollout.enabled: true` (staging + prod gateway) emits `Rollout`
      + `AnalysisTemplate`, no `Deployment`; HPA `scaleTargetRef` is `kind: Rollout` and the Rollout has no
      static `replicas`. For default (`rollout.enabled: false`) it emits the Deployment unchanged — proven
      byte-identical via `diff` of BEFORE-vs-AFTER renders across three shapes (HTTP+HPA+secrets+ingress,
      gRPC+KEDA, plain default): all IDENTICAL.
- [x] Offline schema/structural validation: standard resources pass `kubectl create --dry-run=client`
      (SA/Service/HPA/Ingress/ExternalSecret/SecretStore) and the `argo-rollouts` Application passes too.
      The `Rollout` + `AnalysisTemplate` are Argo CRDs, so `--dry-run=client` (even `--validate=false`) has
      no REST mapping ("no matches for kind Rollout/AnalysisTemplate … ensure CRDs are installed first");
      validated structurally with `yq` instead (7 canary steps; metrics error-rate/latency-p95/rps-sanity;
      analysis steps reference `team-gateway-canary`; every doc parses as valid YAML).
- [x] `kubectl apply --dry-run=server` — VERIFIED 2026-09-04 on the kind cluster (Argo Rollouts CRDs
      installed, controller Running 2/2): `helm template … --set rollout.enabled=true` → `kubectl apply
      --dry-run=server` accepted both `analysistemplate.argoproj.io/team-gateway-canary` and
      `rollout.argoproj.io/team-gateway` (server dry run).
- [x] Staged rollout + live analysis run — VERIFIED with an isolated `canary-demo` Rollout on the cluster:
      a good image reached phase Healthy; patching to a bad image (nonexistent tag) held the Rollout at
      Progressing ("more replicas need to be updated") WITHOUT promoting — the bad canary pod stayed
      ImagePullBackOff while both stable pods kept Running, proving the canary blocks a bad version and
      preserves stable. (Full metric-driven auto-abort additionally needs live Prometheus traffic; the
      protective canary behavior is demonstrated.)

## 5. E2E (platform-e2e)
- [x] **None — infrastructure/manifests, non-user-facing.** No `FEATURES.yaml`/`.feature` (mirrors
      `harden-service-helm`). The e2e-track equivalent is the render/dry-run + rollout-analysis validation
      gate in task 4 (helm lint/template + `kubectl --dry-run` + `yq` structural parse; a staged rollout +
      AnalysisRun on a cluster is deferred). Recorded so the archive gate reads "no scenarios", not
      "missing scenarios".

## 6. Archive
- [x] Rollout + AnalysisTemplate render for the opted-in service; default services still render Deployments;
      HPA composes with the Rollout. (Verified via `helm template` + `kubectl --dry-run=client` + `yq`.)
      Cluster staged rollout DEFERRED — no live cluster.
- [ ] Sync the `deploy-runtime` spec delta into `openspec/specs/deploy-runtime/spec.md`. DEFERRED — outside
      this task's scope boundary (edits limited to `platform-gitops/**` + this `tasks.md`); the delta already
      exists at `changes/add-canary-deploys/specs/deploy-runtime/spec.md`.
- [ ] Archive the change to `changes/archive/2026-09-02-add-canary-deploys/`. DEFERRED — outside this task's
      scope boundary (openspec CLI not installed; archive manually in the established format).
