# platform-gitops

Desired-state (GitOps) repo for the marketplace: ArgoCD pulls from here and converges the
cluster (auto-sync, prune, self-heal on every Application). It is the **deployment plane
only**. There is no business code, no service, no proto and no database owned here. It
holds ArgoCD Applications, one generic Helm chart, per-env values and raw platform
manifests. It is not a deployed service; it is read by ArgoCD in the local kind cluster
(`deploy/`), and the staging/prod overlays are not live anywhere (see "Staging and prod").

## Layout

| Path | What it holds |
|---|---|
| `argocd/apps/` | Child ArgoCD Applications, synced by the root app-of-apps. Per-service Apps (`team-*.yaml`), infra/platform Apps, Helm-chart Apps (vault, external-secrets, keda, metrics-server, argo-rollouts), and `appset-services.yaml` (ApplicationSet for staging/prod). |
| `charts/service/` | The **only** Helm chart. Renders Deployment (or Argo Rollout), Service, Ingress, NetworkPolicy, HPA or KEDA ScaledObject, and ServiceAccount + SecretStore + ExternalSecret. The knob surface is `charts/service/values.yaml`. A new service is not a new chart. |
| `envs/local/values.yaml` | `global.registry` plus the `images:` tag map (one key per image, `local`). CI bumps these. |
| `envs/services/<svc>.yaml` | Invariant per-service baseline for the 16 `team-*` services. Used by the ApplicationSet only. |
| `envs/{staging,prod}/` | `values.yaml` (registry + image tags), `services.yaml` (env-wide overlay: probe timing), `services/<svc>.yaml` (per-service overlay). |
| `platform/` | Raw manifests, no Helm: `infra` (redpanda, opensearch, minio, valkey, flipt, modelserve), `postgres`, `backup`, `monitoring` (Prometheus), `recsys`, `search-indexer`, `team-analytics`, `vault-config`. |
| `scripts/check_identity_seed.py` | The only check shipped in this repo. |
| `.claude/skills/scaffold-service/` | Agent skill for adding a service. |

The root app-of-apps (`marketplace-root`, path `argocd/apps`, recursive) is **not in this
repo**: it is `deploy/argocd/root-app.yaml`. Gitea (`gitea.localtest.me`, repo
`ci/platform-gitops`) is created by `deploy/gitea/`; Applications read it in-cluster at
`http://gitea-http.gitea.svc.cluster.local:3000/ci/platform-gitops.git`, branch `main`.

## Deployed apps

Sync waves: 0 infra, 1 operators (vault, external-secrets, keda, argo-rollouts), 2 vault-config
/ prometheus / postgres backup, 4 services.

| Group | Applications |
|---|---|
| Services (wave 4, chart `charts/service`) | 16 Apps: `team-identity`, `team-domain`, `team-search`, `team-engagement`, `team-order`, `team-payment`, `team-chat`, `team-notification`, `team-referral`, `team-verification`, `team-sharing`, `team-audit`, `team-gateway`, `team-frontend`, `team-ai`, `team-promotion` |
| Raw-manifest apps (wave 4) | `team-analytics` (`platform/team-analytics`), `search-indexer`, `platform-recsys` (CronJob) |
| Infra (wave 0) | `infra-shared` (`platform/infra`), `platform-modelserve` (also `platform/infra`), `infra-postgres` (`platform/postgres`), `metrics-server` 3.12.2 |
| Wave 2 | `infra-postgres-backup` (`platform/backup`), `prometheus` (`platform/monitoring`), `vault-config` |
| Operators (wave 1) | `vault` 0.28.1, `external-secrets` 0.10.4, `keda` 2.14.0, `argo-rollouts` 2.37.3 |

## Add a service

Follow `.claude/skills/scaffold-service/SKILL.md`. Minimum edits:

1. `argocd/apps/<svc>.yaml`: Application with two sources (`ref: values` plus `path: charts/service`), inline `values:` (name, image.repo, ports, env, `secrets.enabled`), `valueFiles: $values/envs/local/values.yaml`.
2. `envs/local/values.yaml`: add `images.<svc>: local` (otherwise the chart falls back to `defaultTag: local`).
3. If it needs secrets: append it to `SERVICES` in `platform/vault-config/vault-config.yaml`. If it needs a database: also `SERVICES` in `platform/postgres/postgres.yaml`.
4. For staging/prod (the skill does not cover this): add `envs/services/<svc>.yaml`, an element in the `list` generator of `appset-services.yaml`, and image tags in `envs/staging/values.yaml` and `envs/prod/values.yaml`.
5. Gateway routing: add `UPSTREAM_<X>_ADDR` to `argocd/apps/team-gateway.yaml`.

Then push to Gitea (`make -C deploy gitops-push`). Never `kubectl apply` or `helm upgrade` by hand.

## Env precedence

Local Applications use `$values/envs/local/values.yaml` plus their own inline `values:`. The
ApplicationSet (`marketplace-services`) uses, later wins:

1. `charts/service/values.yaml`
2. `envs/<env>/values.yaml` (registry, image tags)
3. `envs/services/<svc>.yaml`
4. `envs/<env>/services.yaml`
5. `envs/<env>/services/<svc>.yaml` (optional, `ignoreMissingValueFiles`)
6. inline `image.repo` (`<envRegistry>/<svc>`) and `pullPolicy: IfNotPresent`

## Staging and prod

`appset-services.yaml` generates one Application per (cluster x service) for clusters labelled
`envScoped=true`, with `envName` (staging|prod) and `envRegistry` labels. The local kind
cluster is unlabelled, so the ApplicationSet generates **zero** apps locally. Staging and prod
registries are placeholders (`staging-registry.example.com`, `registry.example.com`) and no
real staging/prod cluster is provisioned. Infra and raw-manifest apps are local-only. Prod pins
tags `"1.0.0"`; staging uses `staging`. Design: archived OpenSpec change `add-staging-prod-overlays`.

## Canary (Argo Rollouts)

Opt-in per service with `rollout.enabled` (default false: plain Deployment). When true the chart
renders a canary `Rollout` plus an `AnalysisTemplate` gating promotion on Prometheus RED
metrics (error rate, p95, min RPS); the HPA then targets the Rollout. Enabled only for
`team-gateway` in staging and prod overlays, with different step cadence and thresholds
(prod: 10/25/50/100%, 300s pauses, error rate 0.02). Design: archived change `add-canary-deploys`.

## Secrets and Vault

Services with `secrets.enabled` get a ServiceAccount, a `SecretStore` (Vault kubernetes auth, role = service name) and an
`ExternalSecret` that extracts `<svc>/config` from the `svc` kv-v2 mount into Secret `<svc>-env`.
`secrets.jwtShared` (team-identity only, ADR-0006) also pulls `JWT_PRIVATE_KEY` and `JWT_KID`
from `shared/jwt`. `vault-config` runs a Job that seeds auth, policies and `svc/<name>/config`
for every name in its `SERVICES` list.

Local-only, never a prod practice: Vault runs in **dev mode with root token `root`**
(`argocd/apps/vault.yaml`, in memory, re-seeded on start), the seed script in
`vault-config.yaml` embeds a dev JWT keypair, and `platform/postgres` creates roles with
`<svc>_pass` passwords and an `emptyDir` volume.

## Cluster up and down

The local kind cluster is part of the `deploy/` plane and **should stay stopped** unless you
need it (cluster e2e or smoke): it contends for resources and caused flakes (root `README.md`).

```bash
make -C deploy cluster-up     # kind + registry + ingress + argocd + gitea + app-of-apps
make -C deploy images         # retag local images -> registry, bump tags here
make -C deploy gitops-push    # commit + push this repo to Gitea; ArgoCD syncs
make -C deploy status         # ArgoCD apps + marketplace pods
make -C deploy argocd-ui      # https://localhost:8083
make -C deploy smoke          # platform-e2e against the cluster ingress
make -C deploy cluster-down
```

See `deploy/README.md`. Flow: dev push, CI (`deploy/act`) builds and pushes the image and bumps
the tag in `envs/local/values.yaml`, Gitea, ArgoCD. CI lives in `deploy/act`, not in this repo.

## Check and verify

- `python3 scripts/check_identity_seed.py`: fails if any manifest enables `SEED_ADMIN_ENABLED` or carries a `SEED_ADMIN_PASSWORD` literal. No CI is wired in this repo; run it before pushing.
- Render a service before pushing: `helm template <svc> charts/service -f envs/local/values.yaml --set name=<svc> ...` (per the skill; optional `kubeconform`).
- `FEATURES.yaml`: none in this repo. Verification is the cluster smoke (`make -C deploy smoke`). Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.

## Known gaps

- Local per-service Applications carry **inline values**, while `envs/services/<svc>.yaml` is a copy of them for the ApplicationSet. The two can drift; nothing checks it.
- `infra-shared` and `platform-modelserve` both sync `platform/infra` recursively (two Applications, same path).
- `envs/staging/values.yaml` and `envs/prod/values.yaml` lack tags for `team-referral`, `team-verification`, `team-sharing`, `team-audit`, `team-promotion` (all are in the ApplicationSet list). The chart falls back to `defaultTag: local`.
- Service lists are duplicated by hand in four places: `argocd/apps/`, `appset-services.yaml`, `vault-config.yaml` `SERVICES`, `postgres.yaml` `SERVICES`. The latter two differ (gateway and ai are Vault-only).
- The scaffold skill omits `envs/services/` and the ApplicationSet list.
- `platform-gitops` has no entry in the root `AGENTS.md` repo map, and no helm/kubeconform/ArgoCD diff check is wired.

## Links

Root `AGENTS.md` (rules); `deploy/README.md`; ADR-0006 (JWT issuer key) and ADR-0010 (zero-trust interim, NetworkPolicy) in `platform-core/docs/ADR/`.
