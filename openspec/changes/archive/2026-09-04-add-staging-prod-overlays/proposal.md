## Why

The platform is **local-only**: `platform-gitops/envs/` holds exactly one environment,
`envs/local/values.yaml`, which carries the per-service image tags (all `local`) that CI bumps and
every ArgoCD Application references via `$values/envs/local/values.yaml`. The just-landed
`harden-service-helm` change gave `charts/service` real probes, resources, and HPA/KEDA autoscaling,
so the chart is now safe to run beyond a kind cluster — but there is nowhere to express the
**per-environment** differences (image tags per env, higher replicas/resources, autoscaling bounds,
real ingress hosts) that a staging or prod target needs. This change adds `envs/staging` and
`envs/prod` overlays and the ArgoCD wiring to target them, so the same chart deploys to three
environments without forking it.

## What Changes

- **platform-gitops (`envs/staging/values.yaml`, `envs/prod/values.yaml`)** — add two new env value
  files mirroring `envs/local/values.yaml`'s shape (`global.registry`, `global.imagePullPolicy`,
  `images: {<service>: <tag>}`), with per-env registries and image tags (staging: e.g. `staging`/
  digest; prod: pinned release tags, `imagePullPolicy: IfNotPresent`) instead of `local`.
- **platform-gitops (`envs/staging/`, `envs/prod/`)** — add per-env, per-service override values
  (resource requests/limits, replica counts, autoscaling min/max + targets, ingress hosts) so an env
  can scale a service up without editing the ArgoCD app inline block. Structure chosen in the ArgoCD
  wiring below (a shared per-env values file layered under the app's inline values).
- **platform-gitops (`argocd/apps/*`)** — parameterise the Applications so they target an env: today
  each app hard-codes `valueFiles: [$values/envs/local/values.yaml]` plus an inline `values:` block.
  Introduce an env dimension (ApplicationSet over `envs/*`, or a per-env app directory) so
  staging/prod apps point at `$values/envs/<env>/values.yaml` and layer the per-env service overrides
  ahead of the inline defaults. Local behaviour is preserved unchanged.
- **platform-gitops** — document the precedence order (chart defaults → env image values → per-env
  service overlay → app inline) so image tags and per-env sizing resolve deterministically.

This EXTENDS the `deploy-runtime` capability (probes/resources/HPA already specified); it adds the
environment dimension on top of the hardened chart. No chart template change is required — overlays
only supply values the chart already understands.

## Non-goals

- **Canary / progressive delivery (Argo Rollouts)** — a separate P2 change (`add-canary-deploys`);
  these overlays deploy plain Deployments/HPA only.
- **Provisioning real staging/prod clusters** — network, DNS, cluster creation, and ArgoCD
  cluster registration are infra-ops, out of scope; this change only adds the GitOps values +
  Application wiring that those clusters would sync.
- **Secrets backend changes** — the Vault/External-Secrets seam (`secrets.enabled`) is reused as-is;
  only per-env **values** (e.g. vault path or ingress host) change, not the mechanism.
- **New services or chart templates** — no `charts/service` template edits; overlays supply values
  the hardened chart already renders.
- **CI pipeline changes** beyond having CI bump the new env image files the same way it bumps
  `envs/local/values.yaml` (noted for the pipeline owner, not implemented here).
